package server

import (
	"context"
	"time"

	"tolerex/internal/logger"
	"tolerex/internal/storage"
	pb "tolerex/proto/gen"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

func callerFromContext(ctx context.Context) string {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return "unknown"
	}

	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok || len(tlsInfo.State.PeerCertificates) == 0 {
		return "unknown"
	}

	return tlsInfo.State.PeerCertificates[0].Subject.CommonName
}

// -------- Üye sunucu
type MemberServer struct {
	pb.UnimplementedStorageServiceServer
	DataDir string
}

// -------- Store: Liderden gelen mesajı diske yazar
func (s *MemberServer) Store(ctx context.Context, msg *pb.StoredMessage) (*pb.StoreResult, error) {
	start := time.Now()
	log := logger.WithContext(ctx, logger.Member)

	caller := callerFromContext(ctx)

	logger.Info(
		log,
		"Store start | caller=%s | msg_id=%d",
		caller,
		msg.Id,
	)

	err := storage.WriteMessage(s.DataDir, int(msg.Id), msg.Text)
	if err != nil {
		logger.Error(
			log,
			"Store failed | caller=%s | msg_id=%d | err=%v",
			caller,
			msg.Id,
			err,
		)
		return &pb.StoreResult{Ok: false, Err: err.Error()}, nil
	}

	logger.Info(
		log,
		"Store success | caller=%s | msg_id=%d | duration=%s",
		caller,
		msg.Id,
		time.Since(start),
	)

	return &pb.StoreResult{Ok: true}, nil
}

// -------- Retrieve: Diskten mesajı okur
func (s *MemberServer) Retrieve(ctx context.Context, req *pb.MessageID) (*pb.StoredMessage, error) {
	start := time.Now()
	log := logger.WithContext(ctx, logger.Member)

	caller := callerFromContext(ctx)

	logger.Info(
		log,
		"Retrieve start | caller=%s | msg_id=%d",
		caller,
		req.Id,
	)

	text, err := storage.ReadMessage(s.DataDir, int(req.Id))
	if err != nil {
		logger.Warn(
			log,
			"Retrieve miss | caller=%s | msg_id=%d | err=%v",
			caller,
			req.Id,
			err,
		)
		return &pb.StoredMessage{}, nil
	}

	logger.Info(
		log,
		"Retrieve success | caller=%s | msg_id=%d | duration=%s",
		caller,
		req.Id,
		time.Since(start),
	)

	return &pb.StoredMessage{Id: req.Id, Text: text}, nil
}
