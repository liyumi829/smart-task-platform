// Package bootstrap
// 实现 Mysql 数据库的配置信息以及连接
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// MySQLConfig MySQL 数据库配置
type MySQLConfig struct {
	// 数据库主机地址
	Host string `yaml:"host"`

	// 数据库端口
	Port int `yaml:"port"`

	// 数据库登录用户名
	User string `yaml:"user"`

	// 数据库登录密码
	Password string `yaml:"password"`

	// 数据库名称
	Database string `yaml:"database"`

	// 数据库字符集
	Charset string `yaml:"charset"`

	// 是否解析时间类型
	ParseTime bool `yaml:"parse_time"`

	// 时区设置
	Loc string `yaml:"loc"`

	// 最大打开连接数
	MaxOpenConns int `yaml:"max_open_conns"`

	// 最大空闲连接数
	MaxIdleConns int `yaml:"max_idle_conns"`

	// 连接最大生命周期（秒）
	ConnMaxLifetime int `yaml:"conn_max_lifetime"`

	// 连接最大空闲时间（秒）
	ConnMaxIdleTime int `yaml:"conn_max_idle_time"`
}

// InitDB 初始化连接数据库
func InitDB(mode string, c *MySQLConfig) *gorm.DB {
	c.setDefault() // 合并用户配置

	// 日志模式进行数据库配置
	var cfg *gorm.Config // 配置信息
	if mode == ModeDev {
		cfg = &gorm.Config{
			Logger: logger.Default.LogMode(logger.Info), // 打印 SQL
		}
	} else {
		// 自定义 GORM 日志（输出到 stdout，生产环境用）
		newLogger := logger.New(
			log.New(os.Stdout, "", log.LstdFlags),
			logger.Config{
				SlowThreshold:             100 * time.Millisecond,
				LogLevel:                  logger.Warn,
				IgnoreRecordNotFoundError: true,
				Colorful:                  false,
			},
		)

		// 这里包装一下，过滤 context canceled
		newLogger = IgnoreCanceledLogger{
			Interface: newLogger,
		}

		cfg = &gorm.Config{
			Logger: newLogger,
		}
	}

	// 1. 打开数据库连接
	dsn := c.mySQLDSN()
	db, err := gorm.Open(mysql.Open(dsn), cfg)
	if err != nil {
		zap.L().Fatal("connect database failed", zap.Error(err))
	}

	// 2. 获取原生的数据库连接
	sqlDB, err := db.DB()
	if err != nil {
		zap.L().Fatal("get sql.DB instance failed", zap.Error(err))
	}

	// 3. 设置数据库连接的配置
	sqlDB.SetMaxOpenConns(c.MaxOpenConns)                                    // 最大打开连接数（根据MySQL配置调整，一般 100~200）
	sqlDB.SetMaxIdleConns(c.MaxIdleConns)                                    // 最大空闲连接数
	sqlDB.SetConnMaxLifetime(time.Duration(c.ConnMaxLifetime) * time.Second) // 连接最大生命周期（避免长连接失效）
	sqlDB.SetConnMaxIdleTime(time.Duration(c.ConnMaxIdleTime) * time.Second) // 连接最大空闲时间
	// 启动时打印确认配置是否生效
	log.Printf("mysql connection pool configured, max_open_conns:%d, max_idle_conns:%d, conn_max_lifetime_seconds:%d, conn_max_idle_time_seconds:%d",
		c.MaxOpenConns,
		c.MaxIdleConns,
		c.ConnMaxLifetime,
		c.ConnMaxIdleTime)

	zap.L().Info("database connected successfully",
		zap.String("dns", dsn),
	)

	return db
}

// setDefault 设置 MySQL 配置默认值
func (c *MySQLConfig) setDefault() {
	if c.Host == "" {
		c.Host = "127.0.0.1"
	}
	if c.Port <= 0 {
		c.Port = 3306
	}
	if c.User == "" {
		c.User = "root"
	}
	// 密码默认空
	if c.Database == "" {
		c.Database = "smart_task_platform"
	}
	if c.Charset == "" {
		c.Charset = "utf8mb4"
	}
	// ParseTime 默认为 true，不需要判断
	if c.Loc == "" {
		c.Loc = "Local"
	}
	// 4 核 4G 单机部署时，不建议默认开到 100，避免 MySQL 并发查询过多导致 Threads_running 波动过大
	if c.MaxOpenConns <= 0 {
		c.MaxOpenConns = 32
	}

	// 空闲连接不应过多，建议为 MaxOpenConns 的 1/4 ~ 1/2
	if c.MaxIdleConns <= 0 {
		c.MaxIdleConns = 16
	}

	// MaxIdleConns 不能大于 MaxOpenConns
	if c.MaxIdleConns > c.MaxOpenConns {
		c.MaxIdleConns = c.MaxOpenConns
	}

	// 连接最大生命周期不要太短，避免压测期间频繁重建连接
	if c.ConnMaxLifetime <= 0 {
		c.ConnMaxLifetime = 300
	}

	// 空闲连接保留时间适中，减少连接频繁创建和销毁
	if c.ConnMaxIdleTime <= 0 {
		c.ConnMaxIdleTime = 60
	}
}

// MySQLDSN 生成 MySQL DSN
func (c *MySQLConfig) mySQLDSN() string {
	return fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=%t&loc=%s",
		c.User,
		c.Password,
		c.Host,
		c.Port,
		c.Database,
		c.Charset,
		c.ParseTime,
		c.Loc,
	)
}

// IgnoreCanceledLogger 包装 GORM Logger，用于忽略 context canceled 日志
type IgnoreCanceledLogger struct {
	logger.Interface
}

// LogMode 设置日志级别
func (l IgnoreCanceledLogger) LogMode(level logger.LogLevel) logger.Interface {
	return IgnoreCanceledLogger{
		Interface: l.Interface.LogMode(level),
	}
}

// Trace 拦截 GORM SQL 日志
func (l IgnoreCanceledLogger) Trace(
	ctx context.Context,
	begin time.Time,
	fc func() (sql string, rowsAffected int64),
	err error,
) {
	// 压测时客户端主动断开会产生 context canceled，不作为服务端错误打印
	if errors.Is(err, context.Canceled) {
		return
	}

	// context deadline exceeded 可以保留，因为它通常代表服务端超时或 SQL 太慢
	l.Interface.Trace(ctx, begin, fc, err)
}
