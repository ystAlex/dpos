package utils

import (
	"fmt"
	"time"
)

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
	level  LogLevel
	nodeID string // 新增：记录节点ID
}

// NewLogger 创建新的日志记录器
// 修改：接受节点ID参数，默认使用INFO级别
func NewLogger(nodeID string) *Logger {
	return &Logger{
		level:  INFO, // 默认INFO级别
		nodeID: nodeID,
	}
}

// NewLoggerWithLevel 创建指定级别的日志记录器
func NewLoggerWithLevel(nodeID string, level LogLevel) *Logger {
	return &Logger{
		level:  level,
		nodeID: nodeID,
	}
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
	// 修改：添加节点ID前缀
	fmt.Printf("[%s] [%s] %s: %s\n", timestamp, l.nodeID, level, message)
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

// SetLevel 设置日志级别
func (l *Logger) SetLevel(level LogLevel) {
	l.level = level
}
