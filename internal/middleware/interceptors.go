// ===================================================================================
// TOLEREX – gRPC REQUEST ID INTERCEPTOR
// ===================================================================================
//
// This file implements a gRPC unary server interceptor responsible for
// generating and propagating a unique Request ID for each incoming request.
//
// From a distributed systems and observability perspective, this interceptor:
//
// - Generates a globally unique identifier (UUID v4) per request
// - Injects the Request ID into the request context
// - Enables request correlation across logs, metrics, and goroutines
// - Acts as an early middleware layer in the gRPC interceptor chain
//
// The Request ID is stored in context.Context and can be retrieved by
// downstream handlers, services, and logging utilities.
//
// This interceptor does NOT perform logging itself.
// It only enriches the context with metadata.
//
// ===================================================================================

package middleware

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc"

	"tolerex/internal/logger"
)

// --- REQUEST ID INTERCEPTOR ---
// Creates a gRPC UnaryServerInterceptor that:
//
// - Generates a unique request identifier
// - Stores it in the context under logger.RequestIDKey
// - Passes the enriched context to the next handler in the chain
func RequestIDInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {

		// --- REQUEST ID GENERATION ---
		// A new UUID is generated for every incoming gRPC request.
		reqID := uuid.New().String()

		// --- CONTEXT ENRICHMENT ---
		// The Request ID is injected into the context so that
		// downstream layers (logger, metrics, handlers) can access it.
		ctx = context.WithValue(ctx, logger.RequestIDKey, reqID)

		// --- DELEGATE TO NEXT HANDLER ---
		// Control is passed to the next interceptor or the actual
		// gRPC method implementation.
		return handler(ctx, req)
	}
}
