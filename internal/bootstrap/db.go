// Package bootstrap
// 实现 Mysql 数据库的配置信息以及连接
package bootstrap

import (
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
	if mode == "debug" {
		cfg = &gorm.Config{
			Logger: logger.Default.LogMode(logger.Info), // 打印 SQL
		}
	} else {
		// 自定义 GORM 日志（输出到 stdout，生产环境用）
		newLogger := logger.New(
			log.New(os.Stdout, "\r\n", log.LstdFlags), // 输出到标准输出
			logger.Config{
				SlowThreshold:             200 * time.Millisecond, // 慢查询阈值
				LogLevel:                  logger.Warn,            // 只打印 Warn/Error
				IgnoreRecordNotFoundError: true,                   // 忽略记录不存在错误
				Colorful:                  false,                  // 生产环境关闭颜色
			},
		)
		cfg = &gorm.Config{
			Logger: newLogger, // 使用自定义日志器
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
	if c.MaxOpenConns <= 0 {
		c.MaxOpenConns = 100
	}
	if c.MaxIdleConns <= 0 {
		c.MaxIdleConns = 20
	}
	if c.ConnMaxLifetime <= 0 {
		c.ConnMaxLifetime = 30
	}
	if c.ConnMaxIdleTime <= 0 {
		c.ConnMaxIdleTime = 10
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
