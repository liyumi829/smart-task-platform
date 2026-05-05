// Package bootstrap
// 初始化全局日志器
// 全局日志器的配置文件信息：日志器的运行模式、日志的存储路径、文件名称
package bootstrap

import (
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var Logger *zap.Logger // 全局日志器

// LoggerConfig 日志配置
type LoggerConfig struct {
	// 运行模式 debug / prod
	Mode string `yaml:"mode"`

	// 日志存储路径
	LogPath string `yaml:"log_path"`

	// 日志文件名称
	LogName string `yaml:"log_name"`

	// 日志文件最大大小（MB）
	MaxSize int `yaml:"max_size"`

	// 最大保留日志文件数
	MaxBackups int `yaml:"max_backups"`

	// 最大保留天数
	MaxAge int `yaml:"max_age"`
}

// setDefault 设置配置默认值
func (c *LoggerConfig) setDefault() {
	if c.Mode == "" {
		c.Mode = "debug"
	}
	if c.LogName == "" {
		c.LogName = "log"
	}
	if c.LogPath == "" {
		c.LogPath = "./logs"
	}
	if c.MaxSize <= 0 {
		c.MaxSize = 128
	}
	if c.MaxBackups <= 0 {
		c.MaxBackups = 30
	}
	if c.MaxAge <= 0 {
		c.MaxAge = 7
	}
}

// initLogger 初始化全局日志器
func InitLogger(c *LoggerConfig) {
	c.setDefault() // 设置默认值，避免有非法参数传入

	var level zapcore.Level    // 日志等级
	if c.Mode == ModeRelease { // 如果项目是生产环境
		level = zap.InfoLevel // 只打印Info以上的信息
	} else { // 默认运行模式debug
		level = zap.DebugLevel // 答应调试以上的信息
	}
	// 自定义日志格式
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "Logger",
		CallerKey:      "caller",                      // 哪一个文件哪一行
		MessageKey:     "msg",                         // 日志内容
		StacktraceKey:  "stack",                       // 调用栈
		LineEnding:     zapcore.DefaultLineEnding,     // 默认换行符
		EncodeLevel:    zapcore.CapitalLevelEncoder,   // 日志级别的显示（无颜色）
		EncodeTime:     zapcore.ISO8601TimeEncoder,    // 时间格式：2026-03-30T16:23:10+08:00
		EncodeDuration: zapcore.StringDurationEncoder, // 耗时格式字符串
		EncodeCaller:   zapcore.ShortCallerEncoder,    // 短路径
	}
	// encoder := zapcore.NewJSONEncoder(encoderConfig) // 创建编码器 -- 在下面生产环境 Json、开发环境 Console
	var core zapcore.Core      // zap日志库的核心接口
	if c.Mode == ModeRelease { // 生产环境级别
		logDir := c.LogPath        // 写入的目录路径
		_ = os.Mkdir(logDir, 0755) // 创建目录
		lumberLogger := &lumberjack.Logger{
			Filename:   filepath.Join(logDir, c.LogName+".log"), // 日志文件
			MaxSize:    c.MaxSize,                               // 单个文件不超过128MB
			MaxBackups: c.MaxBackups,                            //最多保存30个日志文件
			MaxAge:     c.MaxAge,                                // 最多保存七天
			Compress:   true,                                    // 压缩旧日志为gz
			LocalTime:  true,                                    // 使用本地时间
		}
		fileEncoder := zapcore.NewJSONEncoder(encoderConfig)        // 创建Json编码器
		fileWriteSyncer := zapcore.AddSync(lumberLogger)            // 绑定lumber日志写入器到zap中
		core = zapcore.NewCore(fileEncoder, fileWriteSyncer, level) // 创建核心
	} else { // 开发环境级别
		consoleEncoder := zapcore.NewConsoleEncoder(encoderConfig) // 创建显示器编码器
		consoleWriteSyncer := zapcore.AddSync(os.Stdout)           // 绑定标准输出日志写入器到zap
		core = zapcore.NewCore(consoleEncoder, consoleWriteSyncer, level)
	}
	Logger = zap.New(core,
		zap.AddCaller(),                   // 显示行号
		zap.AddStacktrace(zap.ErrorLevel)) // Error以上显示调用堆栈
	// 全局logger创建完成
	zap.ReplaceGlobals(Logger) // 替换全局日志器，可使用zap.L()调用
}
