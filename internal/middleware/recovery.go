package middleware

import (
	"context"
	"runtime/debug"

	"tolerex/internal/logger"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func RecoveryInterceptor(baseLoggerName string) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp interface{}, err error) {

		defer func() {
			if r := recover(); r != nil {
				var base = logger.Leader
				if baseLoggerName == "member" {
					base = logger.Member
				}
				log := logger.WithContext(ctx, base)

				log.Printf("PANIC recovered: %v | method=%s\n%s", r, info.FullMethod, string(debug.Stack()))

				err = status.Errorf(codes.Internal, "internal server error")
			}
		}()

		return handler(ctx, req)
	}
}
