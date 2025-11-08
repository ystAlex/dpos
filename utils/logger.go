package utils

import (
	"fmt"
	"time"
)

// logger.go
// 日志工具
// 提供分级日志输出功能

// LogLevel 日志级别
type LogLevel int

const (
	ERROR LogLevel = 0
	WARN  LogLevel = 1
	INFO  LogLevel = 2
	DEBUG LogLevel = 3
)

// Logger 日志记录器
type Logger struct {
	level LogLevel
}

// NewLogger 创建新的日志记录器
func NewLogger(level LogLevel) *Logger {
	return &Logger{level: level}
}

// Error 错误日志
func (l *Logger) Error(format string, args ...interface{}) {
	if l.level >= ERROR {
		l.log("ERROR", format, args...)
	}
}

// Warn 警告日志
func (l *Logger) Warn(format string, args ...interface{}) {
	if l.level >= WARN {
		l.log("WARN", format, args...)
	}
}

// Info 信息日志
func (l *Logger) Info(format string, args ...interface{}) {
	if l.level >= INFO {
		l.log("INFO", format, args...)
	}
}

// Debug 调试日志
func (l *Logger) Debug(format string, args ...interface{}) {
	if l.level >= DEBUG {
		l.log("DEBUG", format, args...)
	}
}

// log 内部日志函数
func (l *Logger) log(level, format string, args ...interface{}) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf(format, args...)
	fmt.Printf("[%s] %s: %s\n", timestamp, level, message)
}

// LogSection 输出章节分隔
func (l *Logger) LogSection(title string) {
	if l.level >= INFO {
		fmt.Println("\n" + "=================================================")
		fmt.Printf("  %s\n", title)
		fmt.Println("=================================================" + "\n")
	}
}

// LogSubsection 输出小节分隔
func (l *Logger) LogSubsection(title string) {
	if l.level >= INFO {
		fmt.Printf("\n--- %s ---\n\n", title)
	}
}
