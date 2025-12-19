// ===================================================================================
// TOLEREX – gRPC PANIC RECOVERY INTERCEPTOR
// ===================================================================================
//
// This file implements a gRPC unary server interceptor responsible for
// isolating and recovering from panics occurring during request handling.
//
// From a systems reliability and fault-tolerance perspective, this interceptor:
//
// - Prevents process-wide crashes caused by panics
// - Contains failures at the request boundary
// - Logs detailed panic diagnostics including stack traces
// - Converts unrecoverable panics into well-defined gRPC errors
// - Preserves service availability under unexpected runtime failures
//
// This interceptor represents the last-resort safety net in the gRPC
// interceptor chain and must be placed as the outermost interceptor
// to ensure complete coverage.
//
// ===================================================================================

package middleware

import (
	"context"
	"runtime/debug"

	"tolerex/internal/logger"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// --- RECOVERY INTERCEPTOR ---
// Creates a gRPC UnaryServerInterceptor that:
//
// - Wraps request execution in a deferred recover block
// - Captures panics raised by downstream handlers or interceptors
// - Logs panic details with stack trace
// - Translates the panic into a gRPC INTERNAL error
func RecoveryInterceptor(baseLoggerName string) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp interface{}, err error) {

		// --- PANIC RECOVERY DEFER ---
		// Ensures that any panic during request handling
		// is recovered and does not crash the entire process.
		defer func() {
			if r := recover(); r != nil {

				// --- BASE LOGGER SELECTION ---
				// Chooses the appropriate logger based on node role.
				var base = logger.Leader
				if baseLoggerName == "member" {
					base = logger.Member
				}

				// --- CONTEXT-AWARE LOGGER ---
				// Enriches log output with request-scoped metadata.
				log := logger.WithContext(ctx, base)

				// --- PANIC LOGGING ---
				// Logs panic value, gRPC method, and full stack trace
				// for post-mortem analysis.
				log.Printf(
					"PANIC recovered: %v | method=%s\n%s",
					r,
					info.FullMethod,
					string(debug.Stack()),
				)

				// --- ERROR TRANSLATION ---
				// Converts the panic into a standardized gRPC error
				// to prevent leaking internal details to the client.
				err = status.Errorf(codes.Internal, "internal server error")
			}
		}()

		// --- DELEGATE TO NEXT HANDLER ---
		// Executes the next interceptor or the actual gRPC method.
		return handler(ctx, req)
	}
}
