// ===================================================================================
// TOLEREX – gRPC LOGGING INTERCEPTOR
// ===================================================================================
//
// This file implements a gRPC unary server interceptor responsible for
// structured request lifecycle logging.
//
// From an observability and systems monitoring perspective, this interceptor:
//
// - Logs the start and completion of every gRPC request
// - Measures and reports request execution latency
// - Correlates logs using context-propagated Request IDs
// - Differentiates log output based on node role (Leader / Member)
// - Provides consistent, centralized visibility into RPC execution
//
// This interceptor sits in the gRPC middleware chain and wraps
// the actual request handler, making it ideal for:
//
// - Latency analysis
// - Error diagnosis
// - Request tracing
// - Production debugging
//
// ===================================================================================

package middleware

import (
	"context"
	"time"

	"tolerex/internal/logger"

	"google.golang.org/grpc"
)

// --- LOGGING INTERCEPTOR ---
// Creates a gRPC UnaryServerInterceptor that logs the full lifecycle
// of each incoming gRPC request.
func LoggingInterceptor(baseLoggerName string) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {

		// --- REQUEST START TIME ---
		// Captures the timestamp before delegating to the handler
		// to measure total request execution duration.
		start := time.Now()

		// --- BASE LOGGER SELECTION ---
		// Chooses the appropriate base logger depending on node role.
		var base = logger.Leader
		if baseLoggerName == "member" {
			base = logger.Member
		}

		// --- CONTEXT-AWARE LOGGER ---
		// Enriches the logger with request-scoped metadata
		// such as Request ID (if present in context).
		log := logger.WithContext(ctx, base)

		// --- REQUEST START LOG ---
		log.Printf("gRPC started: %s", info.FullMethod)

		// --- DELEGATE TO NEXT HANDLER ---
		// Control is passed to the next interceptor or the actual
		// gRPC method implementation.
		resp, err := handler(ctx, req)

		// --- DURATION CALCULATION ---
		// Measures total execution time of the gRPC request.
		dur := time.Since(start)

		// --- REQUEST COMPLETION LOG ---
		if err != nil {
			log.Printf(
				"gRPC failed: %s | duration=%v | err=%v",
				info.FullMethod,
				dur,
				err,
			)
		} else {
			log.Printf(
				"gRPC finished: %s | duration=%v",
				info.FullMethod,
				dur,
			)
		}

		return resp, err
	}
}
