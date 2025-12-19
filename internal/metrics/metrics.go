// ===================================================================================
// TOLEREX – PROMETHEUS METRICS SUBSYSTEM
// ===================================================================================
//
// This file defines the core Prometheus metrics used for observing gRPC behavior
// within the Tolerex distributed system.
//
// From an observability and systems monitoring perspective, this package:
//
// - Exposes high-level gRPC traffic metrics
// - Enables request-level visibility (volume + latency)
// - Integrates seamlessly with Prometheus pull-based scraping
// - Is designed to be used by gRPC middleware/interceptors
//
// Metrics defined here are process-wide and must be registered exactly once
// during application startup.
//
// This package does NOT expose an HTTP endpoint by itself.
// It only defines and registers metric collectors.
// Endpoint exposure is handled by the main application.
//
// ===================================================================================

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// --- TOTAL gRPC REQUEST COUNTER ---
	// Counts the total number of gRPC requests processed by the server.
	//
	// Labels:
	// - method : gRPC method name (e.g. StorageService/Store)
	// - status : request outcome (e.g. OK, ERROR)
	GrpcRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "grpc_requests_total",
			Help: "Total number of gRPC requests",
		},
		[]string{"method", "status"},
	)

	// --- gRPC REQUEST LATENCY HISTOGRAM ---
	// Measures how long gRPC requests take to complete.
	//
	// Buckets:
	// Uses Prometheus default latency buckets suitable
	// for general RPC performance analysis.
	//
	// Labels:
	// - method : gRPC method name
	GrpcRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "grpc_request_duration_seconds",
			Help:    "gRPC request latency",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method"},
	)
)

// --- METRICS REGISTRATION ---
// Registers all metric collectors with the Prometheus
// default registry.
//
// Must be called exactly once at application startup.
func Init() {
	prometheus.MustRegister(
		GrpcRequestsTotal,
		GrpcRequestDuration,
	)
}
