package server

import (
	"context"
	"log"
	"tolerex/internal/storage"
	pb "tolerex/proto/gen"
)

type MemberServer struct {
	pb.UnimplementedStorageServiceServer
}

func (s *MemberServer) Store(ctx context.Context, msg *pb.StoredMessage) (*pb.StoreResult, error) {
	err := storage.WriteMessage(int(msg.Id), msg.Text)
	if err != nil {
		log.Printf("Disk yazımı başarısız: %v\n", err)
		return &pb.StoreResult{Ok: false, Err: err.Error()}, nil
	}
	log.Printf("Mesaj kaydedildi: id=%d", msg.Id)
	return &pb.StoreResult{Ok: true}, nil
}

func (s *MemberServer) Retrieve(ctx context.Context, req *pb.MessageID) (*pb.StoredMessage, error) {
	text, err := storage.ReadMessage(int(req.Id))
	if err != nil {
		log.Printf("Mesaj okunamadı: %v\n", err)
		return nil, err
	}
	return &pb.StoredMessage{Id: req.Id, Text: text}, nil
}
