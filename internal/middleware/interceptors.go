package middleware

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc"

	"tolerex/internal/logger"
)

func RequestIDInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {

		reqID := uuid.New().String()
		ctx = context.WithValue(ctx, logger.RequestIDKey, reqID)

		return handler(ctx, req) // Is bir sonaki katmana devredilir.
	}
}
