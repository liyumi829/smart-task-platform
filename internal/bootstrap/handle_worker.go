// internal/bootstrap/handle_worker.go
// handle_worker manager 配置
package bootstrap

import (
	"smart-task-platform/internal/model"
	"time"
)

// HandleWorkerConfig
type HandleWorkerConfig struct {
	RetryExchangeName string `yaml:"retry_exchange_name"` // 重试队列名称
	RetryRoutingKey   string `yaml:"retry_routing_key"`   // 重试队列路由键
	MaxRetries        int    `yaml:"max_retries"`         // 最大重试次数（rabbitmq重试次数）
}

func (c *HandleWorkerConfig) setDefault() {
	if c.RetryExchangeName == "" {
		c.RetryExchangeName = model.DefaultOutboxExchangeName + ".retry"
	}

	if c.RetryRoutingKey == "" {
		c.RetryRoutingKey = model.DefaultOutboxRoutingKey + ".retry"
	}

	if c.MaxRetries <= 0 {
		c.MaxRetries = 3
	}
}

// HandleWorkerManagerConfig  Handle 转发消息工作池管理器配置
type HandleWorkerManagerConfig struct {
	Disabled           bool                   `yaml:"disabled"`         // 是否禁用  Handle 工作器
	WorkerCount        int                    `yaml:"worker_count"`     // 并发工作协程数量
	WorkerIDPrefix     string                 `yaml:"worker_id_prefix"` // 消费者 ID 前缀
	StopTimeout        time.Duration          `yaml:"stop_timeout"`     // 优雅关闭超时时间
	HandleWorkerConfig `yaml:"handle_worker"` // worker 配置
}

func (c *HandleWorkerManagerConfig) setDefault() {
	if c.WorkerCount <= 0 {
		c.WorkerCount = 1
	}

	if c.WorkerIDPrefix == "" {
		c.WorkerIDPrefix = "forwarding-worker"
	}

	if c.StopTimeout <= 0 {
		c.StopTimeout = 10 * time.Second
	}
	c.HandleWorkerConfig.setDefault()
}
