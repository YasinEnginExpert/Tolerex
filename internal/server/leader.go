package server

import (
	"context"
	"log"
	"math/rand"
	"time"

	"tolerex/internal/config"
	"tolerex/internal/storage"
	pb "tolerex/proto/gen"

	"google.golang.org/grpc"
)

type LeaderServer struct {
	pb.UnimplementedStorageServiceServer
	Members   []string
	Tolerance int
	MsgMap    map[int][]string
}

func NewLeaderServer(members []string, toleranceFile string) (*LeaderServer, error) {
	tol, err := config.ReadTolerance(toleranceFile)
	if err != nil {
		return nil, err
	}

	return &LeaderServer{
		Members:   members,
		Tolerance: tol,
		MsgMap:    make(map[int][]string),
	}, nil
}

func (s *LeaderServer) Store(ctx context.Context, msg *pb.StoredMessage) (*pb.StoreResult, error) {
	// 1. Lider kendi diskine yazıyor
	err := storage.WriteMessage(int(msg.Id), msg.Text)
	if err != nil {
		return &pb.StoreResult{Ok: false, Err: "Lider diske yazamadı"}, nil
	}

	// 2. Rastgele tolerance kadar üye seç
	rand.Seed(time.Now().UnixNano())
	shuffled := append([]string{}, s.Members...)
	rand.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
	selected := shuffled[:s.Tolerance]

	var successful []string

	// 3. Üyelere Store çağrısı yap
	for _, addr := range selected {
		conn, err := grpc.Dial(addr, grpc.WithInsecure())
		if err != nil {
			log.Printf("Bağlantı hatası (%s): %v", addr, err)
			continue
		}
		defer conn.Close()

		client := pb.NewStorageServiceClient(conn)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		res, err := client.Store(ctx, msg)
		if err == nil && res.Ok {
			successful = append(successful, addr)
		} else {
			log.Printf("Üyeye gönderilemedi (%s): %v", addr, err)
		}
	}

	// 4. Hepsi başarılı mı?
	if len(successful) == s.Tolerance {
		s.MsgMap[int(msg.Id)] = successful
		return &pb.StoreResult{Ok: true}, nil
	}

	return &pb.StoreResult{Ok: false, Err: "Üyelere tam yazılamadı"}, nil
}
