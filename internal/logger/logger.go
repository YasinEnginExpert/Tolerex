package logger

import (
	"context"
	"log"

	"gopkg.in/natefinch/lumberjack.v2"
)

type ctxKey string

const RequestIDKey ctxKey = "request_id"

var (
	Leader *log.Logger
	Member *log.Logger
)

type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
	FATAL
)

var currentLevel = INFO

func enabled(level Level) bool {
	return level >= currentLevel
}

func SetLevel(level Level) {
	currentLevel = level
}


func Init() {
	writer := &lumberjack.Logger{
		Filename:   "logs/tolerex.log",
		MaxSize:    10,   //Dosya 10 MB olunca yeni dosyaya geçer
		MaxBackups: 5,    // En fazla 5 eski log dosyası tutar
		MaxAge:     14,   // 14 günden eski logları siler
		Compress:   true, // Eski dosyaları .gz sıkıştırır
	}

	Leader = log.New(writer, "[LEADER] ", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile)
	Member = log.New(writer, "[MEMBER] ", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile)
}

func WithContext(ctx context.Context, base *log.Logger) *log.Logger {
	if ctx == nil {
		return base
	}

	if reqID, ok := ctx.Value(RequestIDKey).(string); ok {
		return log.New(
			base.Writer(),
			base.Prefix()+"[REQ:"+reqID+"] ",
			base.Flags(),
		)
	}

	return base
}

func Debug(log *log.Logger, format string, v ...any) {
	if enabled(DEBUG) {
		log.Printf("[DEBUG] "+format, v...)
	}
}

func Info(log *log.Logger, format string, v ...any) {
	if enabled(INFO) {
		log.Printf("[INFO] "+format, v...)
	}
}

func Warn(log *log.Logger, format string, v ...any) {
	if enabled(WARN) {
		log.Printf("[WARN] "+format, v...)
	}
}

func Error(log *log.Logger, format string, v ...any) {
	if enabled(ERROR) {
		log.Printf("[ERROR] "+format, v...)
	}
}

func Fatal(log *log.Logger, format string, v ...any) {
	log.Printf("[FATAL] "+format, v...)
	panic("fatal error occurred")
}
