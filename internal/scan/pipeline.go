package scan

import (
	"context"
	"time"

	"atlas/internal/model"
)

// openPortEvent 纯端口发现阶段抛出的轻量级开放端口信号
type openPortEvent struct {
	IP     string
	Port   int
	Banner string
	IsV6   bool
}

// startPipeline 启动异步 HTTP/指纹探测 Worker 池与 Bulk 写库协程
func (sc *Scanner) startPipeline(ctx context.Context) {
	pipelineCtx, cancel := context.WithCancel(ctx)
	sc.cancel = cancel
	sc.enrichChan = make(chan openPortEvent, 10000)
	sc.writerChan = make(chan model.Asset, 10000)

	// 启动 50 个高并发 HTTP/指纹探测 Worker
	for i := 0; i < 50; i++ {
		go sc.enrichWorker(pipelineCtx)
	}

	// 启动 1 个批量写入器
	go sc.batchWriter(pipelineCtx)
}

func (sc *Scanner) enrichWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-sc.enrichChan:
			if !ok {
				return
			}
			portAsset := model.Asset{
				IP:       evt.IP,
				Port:     evt.Port,
				Proto:    "tcp",
				State:    "open",
				Service:  guessService(evt.Port, evt.Banner),
				Banner:   evt.Banner,
				Host:     evt.IP,
				IsIPv6:   evt.IsV6,
				LastSeen: time.Now(),
			}
			sc.httpEnrich(evt.IP, evt.Port, evt.Banner, &portAsset)
			select {
			case sc.writerChan <- portAsset:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (sc *Scanner) batchWriter(ctx context.Context) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	batch := make([]model.Asset, 0, 50)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		for _, a := range batch {
			sc.upsert(ctx, a)
		}
		batch = batch[:0]
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case a, ok := <-sc.writerChan:
			if !ok {
				flush()
				return
			}
			batch = append(batch, a)
			if len(batch) >= 50 {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}
