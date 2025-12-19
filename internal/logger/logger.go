// ===================================================================================
// TOLEREX – CENTRALIZED LOGGING SUBSYSTEM
// ===================================================================================
//
// This file implements the centralized logging infrastructure for the Tolerex
// distributed system.
//
// From a systems and observability perspective, this logger:
//
// - Provides role-based loggers (Leader / Member)
// - Supports log level filtering at runtime
// - Uses structured, prefix-based logging instead of external frameworks
// - Integrates log rotation via lumberjack to prevent unbounded disk growth
// - Propagates request-scoped metadata (Request ID) via context
//
// Design principles:
//
// - Minimal dependencies (standard log + lumberjack)
// - Fail-fast behavior on fatal errors
// - Shared log sink for distributed components
// - Deterministic, human-readable log format
//
// This package intentionally avoids complex logging frameworks (zap, zerolog)
// to keep behavior explicit and predictable in a systems programming context.
//
// ===================================================================================

package logger

import (
	"context"
	"log"
	"os"

	"gopkg.in/natefinch/lumberjack.v2"
)

// --- CONTEXT KEY TYPE ---
// Strongly-typed context key to avoid collisions.
type ctxKey string

// --- REQUEST ID CONTEXT KEY ---
// Used to propagate request-scoped identifiers
// across goroutines and service boundaries.
const RequestIDKey ctxKey = "request_id"

// --- ROLE-BASED LOGGER INSTANCES ---
// Leader  → coordination and control-plane logs
// Member  → storage and data-plane logs
var (
	Leader *log.Logger
	Member *log.Logger
)

// --- LOG LEVEL DEFINITION ---
type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
	FATAL
)

// --- CURRENT LOG LEVEL ---
// Default log level is INFO.
// Messages below this level are suppressed.
var currentLevel = INFO

// --- LOG LEVEL FILTER ---
func enabled(level Level) bool {
	return level >= currentLevel
}

// --- SET GLOBAL LOG LEVEL ---
func SetLevel(level Level) {
	currentLevel = level
}

// --- LOGGER INITIALIZATION ---
// Initializes the logging backend with log rotation.
//
// Rotation policy:
// - Max file size: 10 MB
// - Max backups  : 5 files
// - Max age      : 14 days
// - Compression  : enabled (.gz)
func Init() {
	writer := &lumberjack.Logger{
		Filename:   "logs/tolerex.log",
		MaxSize:    10,    // Rotate after 10 MB
		MaxBackups: 5,     // Keep at most 5 old log files
		MaxAge:     14,    // Remove logs older than 14 days
		Compress:   true, // Compress rotated logs
	}

	Leader = log.New(
		writer,
		"[LEADER] ",
		log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile,
	)

	Member = log.New(
		writer,
		"[MEMBER] ",
		log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile,
	)
}

// --- CONTEXT-AWARE LOGGER ---
// Returns a derived logger enriched with request-scoped metadata.
// If a Request ID is present in the context, it is injected
// into the log prefix.
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

// --- DEBUG LOG ---
func Debug(log *log.Logger, format string, v ...any) {
	if enabled(DEBUG) {
		log.Printf("[DEBUG] "+format, v...)
	}
}

// --- INFO LOG ---
func Info(log *log.Logger, format string, v ...any) {
	if enabled(INFO) {
		log.Printf("[INFO] "+format, v...)
	}
}

// --- WARNING LOG ---
func Warn(log *log.Logger, format string, v ...any) {
	if enabled(WARN) {
		log.Printf("[WARN] "+format, v...)
	}
}

// --- ERROR LOG ---
func Error(log *log.Logger, format string, v ...any) {
	if enabled(ERROR) {
		log.Printf("[ERROR] "+format, v...)
	}
}

// --- FATAL LOG ---
// Logs the message and immediately terminates the process.
// Used for unrecoverable system-level failures.
func Fatal(log *log.Logger, format string, v ...any) {
	log.Printf("[FATAL] "+format, v...)
	os.Exit(1)
}
