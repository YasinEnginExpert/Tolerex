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

func Init() {
	writer := &lumberjack.Logger{
		Filename:   "logs/tolerex.log",
		MaxSize:    10, //Dosya 10 MB olunca yeni dosyaya geçer
		MaxBackups: 5, // En fazla 5 eski log dosyası tutar
		MaxAge:     14, // 14 günden eski logları siler
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
