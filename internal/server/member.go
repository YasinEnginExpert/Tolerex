package server

import (
	"context"

	"tolerex/internal/logger"
	"tolerex/internal/storage"
	pb "tolerex/proto/gen"
)

// -------- Üye sunucu
type MemberServer struct {
	pb.UnimplementedStorageServiceServer
	DataDir string
}

// -------- Store: Liderden gelen mesajı diske yazar
func (s *MemberServer) Store(ctx context.Context, msg *pb.StoredMessage) (*pb.StoreResult, error) {
	log := logger.WithContext(ctx, logger.Member)

	log.Printf("Store request received: msg_id=%d", msg.Id)

	err := storage.WriteMessage(s.DataDir, int(msg.Id), msg.Text)
	if err != nil {
		log.Printf("Disk write failed: %v", err)
		return &pb.StoreResult{Ok: false, Err: err.Error()}, nil
	}

	log.Printf("Message stored successfully: msg_id=%d", msg.Id)
	return &pb.StoreResult{Ok: true}, nil
}

// -------- Retrieve: Diskten mesajı okur
func (s *MemberServer) Retrieve(ctx context.Context, req *pb.MessageID) (*pb.StoredMessage, error) {
	log := logger.WithContext(ctx, logger.Member)

	log.Printf("Retrieve request received: msg_id=%d", req.Id)

	text, err := storage.ReadMessage(s.DataDir, int(req.Id))
	if err != nil {
		log.Printf("Message not found: msg_id=%d, err=%v", req.Id, err)
		return &pb.StoredMessage{}, nil
	}

	log.Printf("Message retrieved successfully: msg_id=%d", req.Id)
	return &pb.StoredMessage{Id: req.Id, Text: text}, nil
}
