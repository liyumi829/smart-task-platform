// internal/bootstrap/outbox_worker.go
// outbox_worker manager 配置

package bootstrap

import (
	"smart-task-platform/internal/pkg/utils"
	"time"
)

// OutboxWorkerConfig
type OutboxWorkerConfig struct {
	PollInterval          time.Duration `yaml:"poll_interval"`            // 轮询间隔：每次查询完，等待多久再进行下一次查询
	BatchSize             int           `yaml:"batch_size"`               // 每次批量拉取多少条消息
	ProcessingTimeout     time.Duration `yaml:"processing_timeout"`       // 单批消息处理超时时间
	RetryBackoff          time.Duration `yaml:"retry_backoff"`            // 处理失败后的重试退避时间
	ResetTimeoutOnStartup *bool         `yaml:"reset_timeout_on_startup"` // 启动时是否重置超时的未处理消息
}

func (c *OutboxWorkerConfig) setDefault() {
	if c.PollInterval <= 0 {
		c.PollInterval = 3 * time.Second
	}

	if c.BatchSize <= 0 {
		c.BatchSize = 100
	}

	if c.ProcessingTimeout <= 0 {
		c.ProcessingTimeout = 10 * time.Minute
	}

	if c.RetryBackoff <= 0 {
		c.RetryBackoff = 30 * time.Second
	}

	if c.ResetTimeoutOnStartup == nil {
		c.ResetTimeoutOnStartup = utils.SafeGetPtr(true)
	}
}

// OutboxWorkerManagerConfig Outbox 事务消息工作池管理器配置
// 负责：定时扫描 Outbox 表、批量发布、失败重试、优雅关闭
type OutboxWorkerManagerConfig struct {
	Disabled           bool                   `yaml:"disabled"`         // 是否禁用 Outbox 工作器
	WorkerCount        int                    `yaml:"worker_count"`     // 并发工作协程数量
	WorkerIDPrefix     string                 `yaml:"worker_id_prefix"` // 消费者 ID 前缀
	StopTimeout        time.Duration          `yaml:"stop_timeout"`     // 优雅关闭超时时间
	OutboxWorkerConfig `yaml:"outbox_worker"` // worker 配置
}

// setDefault 规范化 Manager 配置
func (c *OutboxWorkerManagerConfig) setDefault() {
	if c.WorkerCount <= 0 {
		c.WorkerCount = 1
	}

	if c.WorkerIDPrefix == "" {
		c.WorkerIDPrefix = "outbox-worker"
	}

	if c.StopTimeout <= 0 {
		c.StopTimeout = 10 * time.Second
	}
	c.OutboxWorkerConfig.setDefault()
}
