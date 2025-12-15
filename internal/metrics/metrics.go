package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	//--- Toplam gRPC çağrı sayısı ---
	GrpcRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "grpc_requests_total",
			Help: "Total number of gRPC requests",
		},
		[]string{"method", "status"},
	)
	//--- gRPC çağrılarının ne kadar sürdüğünü ---
	GrpcRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "grpc_request_duration_seconds",
			Help:    "gRPC request latency",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method"},
	)
)

func Init() {
	prometheus.MustRegister(
		GrpcRequestsTotal,
		GrpcRequestDuration,
	)
}
