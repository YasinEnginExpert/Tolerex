package server

import (
	"context"
	"log"
	"tolerex/internal/storage"
	pb "tolerex/proto/gen" // gRPC için otomatik üretilen protobuf interface'i
)

// ------ Üye sunucu tipi, gRPC interface'ini gövdesiz şekilde içerir
type MemberServer struct {
	pb.UnimplementedStorageServiceServer
	DataDir string
}

// ------ Store metodu: Liderden gelen mesajı diske yazar
func (s *MemberServer) Store(ctx context.Context, msg *pb.StoredMessage) (*pb.StoreResult, error) {
	err := storage.WriteMessage(s.DataDir, int(msg.Id), msg.Text) // Disk yazımı
	if err != nil {
		log.Printf("Disk yazımı başarısız: %v\n", err)
		return &pb.StoreResult{Ok: false, Err: err.Error()}, nil // Hata varsa geri dön
	}
	log.Printf("Mesaj kaydedildi: id=%d", msg.Id)
	return &pb.StoreResult{Ok: true}, nil // Başarıyla kaydedildiyse OK döner
}

// ------ Retrieve metodu: Diskten istenen ID'li mesajı okur
func (s *MemberServer) Retrieve(ctx context.Context, req *pb.MessageID) (*pb.StoredMessage, error) {
	text, err := storage.ReadMessage(s.DataDir, int(req.Id)) // ID'ye göre diskteki mesajı oku
	if err != nil {
		log.Printf("Mesaj okunamadı: %v\n", err)
		return nil, err // Okuma hatası varsa hata döner
	}
	return &pb.StoredMessage{Id: req.Id, Text: text}, nil // Mesaj başarıyla okunduysa döner
}
