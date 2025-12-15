package middleware

import (
	"context"
	"time"

	"tolerex/internal/logger"

	"google.golang.org/grpc"
)

func LoggingInterceptor(baseLoggerName string) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {

		start := time.Now()

		var base = logger.Leader
		if baseLoggerName == "member" {
			base = logger.Member
		}
		log := logger.WithContext(ctx, base)

		log.Printf("gRPC started: %s", info.FullMethod)

		resp, err := handler(ctx, req)

		dur := time.Since(start)

		if err != nil {
			log.Printf("gRPC failed: %s | duration=%v | err=%v", info.FullMethod, dur, err)
		} else {
			log.Printf("gRPC finished: %s | duration=%v", info.FullMethod, dur)
		}

		return resp, err
	}
}
