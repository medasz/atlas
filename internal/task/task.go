package task

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"

	"atlas/internal/audit"
	"atlas/internal/blacklist"
	"atlas/internal/model"
	"atlas/internal/queue"
	"atlas/internal/ratelimit"
	"atlas/internal/scan"
	"atlas/internal/scope"
	"atlas/internal/store"
)

// Processor 单目标处理逻辑（Issue #4 注入真实探测实现）
type Processor interface {
	Process(ctx context.Context, task model.Task, target, ports string) (map[string]any, error)
}

// noopProcessor 占位实现，仅供调度链路自测
type noopProcessor struct{}

func (noopProcessor) Process(_ context.Context, _ model.Task, target, _ string) (map[string]any, error) {
	return map[string]any{"target": target, "skipped": "no processor"}, nil
}

// TaskMsg 队列传递的任务消息
type TaskMsg struct {
	TaskID string `json:"task_id"`
	Target string `json:"target"`
	Ports  string `json:"ports"`
	Kind   string `json:"kind"`
}

// Service 任务调度：创建、编排、断点续扫、Worker 注册
type Service struct {
	store         *store.Store
	queue         *queue.Queue
	audit         *audit.Auditor
	bl            *blacklist.Service
	rate          *ratelimit.Limiter
	scanProc      Processor
	vulnProc      Processor
	concurrency   int
	defaultPorts  []int
	portChunkSize int
}

// New 构造任务服务
func New(s *store.Store, q *queue.Queue, a *audit.Auditor, bl *blacklist.Service, r *ratelimit.Limiter, concurrency int, defaultPorts []int, portChunkSize int) *Service {
	if portChunkSize <= 0 {
		portChunkSize = 1000
	}
	return &Service{store: s, queue: q, audit: a, bl: bl, rate: r, scanProc: noopProcessor{}, vulnProc: noopProcessor{}, concurrency: concurrency, defaultPorts: defaultPorts, portChunkSize: portChunkSize}
}

// SetProcessor 注入资产扫描处理器（Issue #4）
func (svc *Service) SetProcessor(p Processor) { svc.scanProc = p }

// SetVulnProcessor 注入漏洞检测处理器（Issue #10~#13）
func (svc *Service) SetVulnProcessor(p Processor) { svc.vulnProc = p }

// processorFor 按任务类型选择处理器
func (svc *Service) processorFor(kind string) Processor {
	if kind == model.TaskVuln {
		return svc.vulnProc
	}
	return svc.scanProc
}

// Create 创建任务：展开 scope → 落库子项 → 过滤黑名单 → 分发
func (svc *Service) Create(ctx context.Context, operator, kind string, sc, schedule, rateLimit map[string]any) (string, error) {
	targets, err := scope.Expand(sc)
	if err != nil {
		return "", err
	}
	id := newID()
	type itemSpec struct {
		target   string
		ports    string
		filtered bool
	}
	specs := make([]itemSpec, 0, len(targets))
	for _, t := range targets {
		filtered := false
		if hit, _ := svc.bl.Match(ctx, t); hit {
			filtered = true
		}
		var chunks []string
		if net.ParseIP(t) != nil {
			if plist := svc.portsForScope(sc); len(plist) > 0 {
				chunks = chunkSpec(plist, svc.portChunkSize)
			}
		}
		if len(chunks) == 0 {
			specs = append(specs, itemSpec{t, "", filtered})
		} else {
			for _, c := range chunks {
				specs = append(specs, itemSpec{t, c, filtered})
			}
		}
	}
	task := model.Task{
		ID:        id,
		Kind:      kind,
		Scope:     sc,
		Schedule:  schedule,
		RateLimit: rateLimit,
		Status:    model.TaskRunning,
		Progress:  map[string]int{"total": len(specs), "done": 0},
	}
	if err := svc.store.CreateTask(ctx, task); err != nil {
		return "", err
	}
	for _, sp := range specs {
		st := model.TaskItemPending
		if sp.filtered {
			st = model.TaskItemFiltered
		}
		if err := svc.store.UpsertTaskItem(ctx, model.TaskItem{TaskID: id, Target: sp.target, Ports: sp.ports, Status: st}); err != nil {
			return "", err
		}
	}
	if svc.audit.Enabled() {
		_ = svc.audit.Log(ctx, operator, fmt.Sprintf("task:%s", id), id, "task.create")
	}
	return id, svc.dispatch(ctx, id)
}

// dispatch 分发待处理子项：有 NATS 则发布队列，否则进程内执行
func (svc *Service) dispatch(ctx context.Context, taskID string) error {
	pending := model.TaskItemPending
	items, err := svc.store.ListTaskItems(ctx, taskID, &pending)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		_ = svc.store.UpdateTaskStatus(ctx, taskID, model.TaskDone)
		return nil
	}
	if svc.queue != nil {
		subject := queue.SubjectScan
		if kind, _ := svc.kindOf(ctx, taskID); kind == model.TaskVuln {
			subject = queue.SubjectVuln
		}
		for _, it := range items {
			msg := TaskMsg{TaskID: taskID, Target: it.Target, Ports: it.Ports, Kind: subject}
			if err := svc.queue.Publish(subject, msg); err != nil {
				return err
			}
		}
		return nil
	}
	go svc.runInProcess(context.Background(), taskID)
	return nil
}

func (svc *Service) kindOf(ctx context.Context, taskID string) (string, error) {
	t, err := svc.store.GetTask(ctx, taskID)
	if err != nil {
		return "", err
	}
	return t.Kind, nil
}

// runInProcess 进程内 Worker 池（无 NATS 时单实例执行）
func (svc *Service) runInProcess(ctx context.Context, taskID string) {
	pending := model.TaskItemPending
	items, err := svc.store.ListTaskItems(ctx, taskID, &pending)
	if err != nil {
		return
	}
	task, err := svc.store.GetTask(ctx, taskID)
	if err != nil {
		return
	}
	ch := make(chan model.TaskItem, len(items))
	for _, it := range items {
		ch <- it
	}
	close(ch)

	n := svc.concurrency
	if n < 1 {
		n = 1
	}
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for it := range ch {
				svc.processOne(ctx, task, it.Target, it.Ports)
			}
		}()
	}
	wg.Wait()
}

// RegisterWorker 注册 NATS 队列订阅（多实例负载均衡）
func (svc *Service) RegisterWorker() error {
	if svc.queue == nil {
		return nil
	}
	handler := func(data []byte) {
		var msg TaskMsg
		if err := json.Unmarshal(data, &msg); err != nil {
			return
		}
		task, err := svc.store.GetTask(context.Background(), msg.TaskID)
		if err != nil {
			return
		}
		svc.processOne(context.Background(), task, msg.Target, msg.Ports)
	}
	if _, err := svc.queue.SubscribeQueue(queue.SubjectScan, "atlas-workers", handler); err != nil {
		return err
	}
	if _, err := svc.queue.SubscribeQueue(queue.SubjectVuln, "atlas-workers", handler); err != nil {
		return err
	}
	return nil
}

// processOne 处理单个目标（或端口块）：限速 → 调用处理器 → 落库结果 → 更新进度
func (svc *Service) processOne(ctx context.Context, task model.Task, target, ports string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("task: processOne recovered panic task=%s target=%s ports=%s: %v\n%s", task.ID, target, ports, r, debug.Stack())
		}
	}()
	if err := svc.rate.WaitGlobal(ctx); err != nil {
		return
	}
	proc := svc.processorFor(task.Kind)
	res, err := proc.Process(ctx, task, target, ports)
	if err != nil {
		res = map[string]any{"error": err.Error()}
	}
	_ = svc.store.MarkItemDone(ctx, task.ID, target, ports, res)

	total, done, cerr := svc.store.CountTaskItems(ctx, task.ID)
	if cerr == nil {
		_ = svc.store.UpdateTaskProgress(ctx, task.ID, total, done)
		if total > 0 && done >= total {
			_ = svc.store.UpdateTaskStatus(ctx, task.ID, model.TaskDone)
		}
	}
}

// Resume 断点续扫：对已有任务的 pending 子项重新分发
func (svc *Service) Resume(ctx context.Context, taskID string) error {
	_ = svc.store.UpdateTaskStatus(ctx, taskID, model.TaskRunning)
	return svc.dispatch(ctx, taskID)
}

// portsForScope 解析某任务端口列表：优先用 scope.ports，否则回退默认端口
func (svc *Service) portsForScope(sc map[string]any) []int {
	if v, ok := sc["ports"].(string); ok {
		if ps, err := scan.ParsePortSpec(v); err == nil && len(ps) > 0 {
			return ps
		}
	}
	return svc.defaultPorts
}

// chunkSpec 将端口切片按 size 切块，返回可被 ParsePortSpec 精确还原的规格字符串：
// 连续递增块用 "lo-hi"，非连续块用逗号拼接。
func chunkSpec(ports []int, size int) []string {
	if size <= 0 {
		size = 1
	}
	out := make([]string, 0, (len(ports)+size-1)/size)
	for i := 0; i < len(ports); i += size {
		end := i + size
		if end > len(ports) {
			end = len(ports)
		}
		chunk := ports[i:end]
		if isContiguous(chunk) {
			out = append(out, fmt.Sprintf("%d-%d", chunk[0], chunk[len(chunk)-1]))
		} else {
			parts := make([]string, len(chunk))
			for j, p := range chunk {
				parts[j] = strconv.Itoa(p)
			}
			out = append(out, strings.Join(parts, ","))
		}
	}
	return out
}

// isContiguous 判断切片是否严格递增 1
func isContiguous(chunk []int) bool {
	for i := 1; i < len(chunk); i++ {
		if chunk[i] != chunk[i-1]+1 {
			return false
		}
	}
	return true
}

func newID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}
