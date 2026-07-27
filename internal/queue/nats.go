package queue

import (
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
)

// Queue NATS 封装，仅用于跨实例任务分发
type Queue struct {
	conn *nats.Conn
}

// New 连接 NATS。
// 注意：nats.Connect 默认开启后台异步重连，即便 server 不存在也可能返回一个“看似成功”、
// 实际未建立连接的 Conn 对象，导致上层误判为已连接，任务被发往无人消费的队列而永远 pending。
// 因此这里在 Connect 之后主动确认 IsConnected()，未真正连上则视为不可用，
// 让上层（main）正确 fallback 到进程内执行。
func New(url string) (*Queue, error) {
	nc, err := nats.Connect(url)
	if err != nil {
		return nil, fmt.Errorf("connect nats: %w", err)
	}
	if !nc.IsConnected() {
		nc.Close()
		return nil, fmt.Errorf("nats server not reachable at %s", url)
	}
	return &Queue{conn: nc}, nil
}

// Publish 发布 JSON 消息到主题
func (q *Queue) Publish(subject string, msg any) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return q.conn.Publish(subject, b)
}

// SubscribeQueue 以队列组订阅（多实例负载均衡，每条消息仅一个实例消费）
func (q *Queue) SubscribeQueue(subject, queueGroup string, handler func(data []byte)) (*nats.Subscription, error) {
	return q.conn.QueueSubscribe(subject, queueGroup, func(m *nats.Msg) {
		handler(m.Data)
		if m.Reply != "" {
			m.Respond(nil)
		}
	})
}

// Close 关闭连接
func (q *Queue) Close() { q.conn.Close() }

// Subjects 任务主题常量
const (
	SubjectScan = "atlas.scan"
	SubjectVuln = "atlas.vuln"
)
