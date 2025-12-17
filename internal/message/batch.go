package message

import (
	"sync"
	"time"
)

// BatchProcessor 批处理器
type BatchProcessor struct {
	mu          sync.Mutex
	batch       []interface{}
	batchSize   int
	batchWindow time.Duration
	processor   func([]interface{})
	ticker      *time.Ticker
	done        chan struct{}
}

// NewBatchProcessor 创建批处理器
func NewBatchProcessor(batchSize int, batchWindow time.Duration, processor func([]interface{})) *BatchProcessor {
	return &BatchProcessor{
		batch:       make([]interface{}, 0, batchSize),
		batchSize:   batchSize,
		batchWindow: batchWindow,
		processor:   processor,
		done:        make(chan struct{}),
	}
}

// Start 启动批处理器
func (b *BatchProcessor) Start() {
	b.ticker = time.NewTicker(b.batchWindow)
	go b.run()
}

// Add 添加消息到批处理
func (b *BatchProcessor) Add(msg interface{}) {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	b.batch = append(b.batch, msg)
	
	if len(b.batch) >= b.batchSize {
		b.flush()
	}
}

// flush 刷新批处理
func (b *BatchProcessor) flush() {
	if len(b.batch) == 0 {
		return
	}
	
	batch := make([]interface{}, len(b.batch))
	copy(batch, b.batch)
	b.batch = b.batch[:0]
	
	go b.processor(batch)
}

// run 运行批处理循环
func (b *BatchProcessor) run() {
	for {
		select {
		case <-b.ticker.C:
			b.mu.Lock()
			b.flush()
			b.mu.Unlock()
		case <-b.done:
			b.mu.Lock()
			b.flush()
			b.mu.Unlock()
			return
		}
	}
}

// Stop 停止批处理器
func (b *BatchProcessor) Stop() {
	close(b.done)
	if b.ticker != nil {
		b.ticker.Stop()
	}
}

