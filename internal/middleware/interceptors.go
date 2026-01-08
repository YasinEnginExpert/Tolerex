// ===================================================================================
// TOLEREX – gRPC REQUEST ID INTERCEPTOR
// ===================================================================================
//
// This file implements a gRPC unary server interceptor responsible for
// generating and propagating a unique Request ID for every incoming request.
//
// From a distributed systems and observability perspective, this interceptor:
//
//   - Generates a globally unique identifier (UUID v4) per request
//   - Injects the Request ID into the request context
//   - Enables end-to-end request correlation across:
//       * logs
//       * metrics
//       * goroutines
//   - Operates as an early middleware layer in the gRPC interceptor chain
//
// The Request ID is stored in context.Context and can be retrieved by any
// downstream component using logger.RequestIDKey.
//
// IMPORTANT:
//   - This interceptor does NOT perform logging itself
//   - Its sole responsibility is context enrichment
//
// This strict separation keeps middleware predictable and composable.
//
// ===================================================================================

package middleware

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc"

	"tolerex/internal/logger"
)

// ===================================================================================
// REQUEST ID INTERCEPTOR
// ===================================================================================
//
// RequestIDInterceptor returns a grpc.UnaryServerInterceptor that:
//
//   - Generates a new Request ID for each incoming gRPC request
//   - Stores the ID in the request context
//   - Passes the enriched context to the next interceptor or handler
//
// The interceptor is expected to be placed early in the interceptor chain
// so that all subsequent layers can rely on the presence of a Request ID.
// This enables consistent request tracing across the entire gRPC call lifecycle.
// The generated Request ID is a UUID version 4, providing sufficient
// randomness and uniqueness guarantees for distributed systems.
// 

func RequestIDInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {

		// -----------------------------------------------------------------------
		// REQUEST ID GENERATION
		// -----------------------------------------------------------------------
		//
		// A new UUID (version 4) is generated for every incoming request.
		// UUID v4 provides sufficient randomness and uniqueness guarantees
		// for request-level tracing in distributed systems.

		reqID := uuid.NewString()

		// -----------------------------------------------------------------------
		// CONTEXT ENRICHMENT
		// -----------------------------------------------------------------------
		//
		// The generated Request ID is injected into the context using a
		// strongly-typed context key defined in the logger package.
		//
		// This allows:
		//   - loggers to automatically include the Request ID
		//   - metrics to correlate with specific requests
		//   - downstream goroutines to inherit tracing metadata

		ctx = context.WithValue(ctx, logger.RequestIDKey, reqID)

		// -----------------------------------------------------------------------
		// DELEGATION
		// -----------------------------------------------------------------------
		//
		// Control is passed to the next interceptor in the chain or to the
		// actual gRPC method implementation if this is the last interceptor.
		
		return handler(ctx, req)
	}
}
