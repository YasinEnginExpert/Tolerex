// ===================================================================================
// TOLEREX – gRPC METRICS INTERCEPTOR
// ===================================================================================
//
// This file implements a gRPC unary server interceptor responsible for
// collecting Prometheus-compatible metrics for RPC traffic.
//
// From an observability and telemetry perspective, this interceptor:
//
// - Counts total gRPC requests by method and status
// - Measures end-to-end request latency
// - Exposes metrics suitable for SLI / SLO analysis
// - Integrates transparently with Prometheus via middleware
//
// The interceptor wraps the actual gRPC handler and records metrics
// after request completion to ensure accurate duration measurement.
//
// Metrics collected here are:
// - grpc_requests_total (Counter)
// - grpc_request_duration_seconds (Histogram)
//
// ===================================================================================

package middleware

import (
	"context"
	"time"

	"tolerex/internal/metrics"

	"google.golang.org/grpc"
)

// --- METRICS INTERCEPTOR ---
// Creates a gRPC UnaryServerInterceptor that records
// request volume and latency metrics for every RPC.
func MetricsInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {

		// --- REQUEST START TIME ---
		// Captures the timestamp before invoking the handler
		// to measure total request processing time.
		start := time.Now()

		// --- DELEGATE TO NEXT HANDLER ---
		// Executes the actual gRPC method (or next interceptor).
		resp, err := handler(ctx, req)

		// --- DURATION CALCULATION ---
		// Measures end-to-end latency in seconds.
		duration := time.Since(start).Seconds()

		// --- STATUS DETERMINATION ---
		// Simplified success/error classification based on handler result.
		status := "success"
		if err != nil {
			status = "error"
		}

		// --- REQUEST COUNTER UPDATE ---
		// Increments the total request counter labeled by:
		// - gRPC method
		// - request status
		metrics.GrpcRequestsTotal.
			WithLabelValues(info.FullMethod, status).
			Inc()

		// --- LATENCY HISTOGRAM UPDATE ---
		// Observes the request duration for the given gRPC method.
		metrics.GrpcRequestDuration.
			WithLabelValues(info.FullMethod).
			Observe(duration)

		return resp, err
	}
}
