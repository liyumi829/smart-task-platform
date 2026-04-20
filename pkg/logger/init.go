// Package logger
// 日志初始化封装：zap + lumberjack 日志滚动切割
// 提供全局全局Logger实例，支持控制台输出+文件输出+JSON结构化日志
package logger

import (
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var Logger *zap.Logger // 全局日志器

// InitLogger 外部使用初始化
func InitLogger(mode string, logPath string, name string, instanceID uint64) {
	initLogger(LoggerConfig{
		StorageName: name,
		Mode:        mode,
		StoragePath: logPath,
	})
}

// initLogger 初始化全局日志器
func initLogger(config LoggerConfig) {
	var level zapcore.Level    // 日志等级
	if config.Mode == "prod" { // 如果项目是生产环境
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
	if config.Mode == "prod" { // 生产环境级别
		logDir := config.StoragePath                                  // 写入的目录路径
		_ = os.Mkdir(filepath.Join(logDir, config.StorageName), 0755) // 创建目录
		lumberLogger := &lumberjack.Logger{
			Filename:   filepath.Join(logDir, config.StorageName, ".log"), // 日志文件
			MaxSize:    128,                                               // 单个文件不超过128MB
			MaxBackups: 30,                                                //最多保存30个日志文件
			MaxAge:     7,                                                 // 最多保存七天
			Compress:   true,                                              // 压缩旧日志为gz
			LocalTime:  true,                                              // 使用本地时间
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
