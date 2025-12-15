package middleware

import (
	"context"
	"time"

	"tolerex/internal/metrics"

	"google.golang.org/grpc"
)

func MetricsInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {

		start := time.Now()

		resp, err := handler(ctx, req)

		duration := time.Since(start).Seconds()

		status := "success"
		if err != nil {
			status = "error"
		}

		metrics.GrpcRequestsTotal.
			WithLabelValues(info.FullMethod, status).
			Inc()

		metrics.GrpcRequestDuration.
			WithLabelValues(info.FullMethod).
			Observe(duration)

		return resp, err
	}
}
