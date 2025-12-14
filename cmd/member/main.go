package main

import (
	"context"
	"flag"
	"log"
	"net"
	"path/filepath"
	"time"

	"tolerex/internal/server"
	proto "tolerex/proto/gen"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	port := flag.String("port", "5556", "gRPC port")

	flag.Parse()

	// --- Sabit adres---
	leaderAddr := "localhost:5555"
	dataDir := filepath.Join("internal", "data", "member-"+*port)
	myAddr := "localhost:" + *port

	//--- gRPC sunucusu ayrı goroutine'de başlatılır
	go func() {
		listener, err := net.Listen("tcp", ":"+*port)
		if err != nil {
			log.Fatalf("Port dinlenemedi: %v", err)
		}

		grpcServer := grpc.NewServer()

		member := &server.MemberServer{
			DataDir: dataDir, //
		}
		proto.RegisterStorageServiceServer(grpcServer, member)

		log.Printf("Member running on port %s, dataDir=%s\n", *port, dataDir)
		if err := grpcServer.Serve(listener); err != nil {
			log.Fatalf("gRPC başlatılamadı: %v", err)
		}
	}()

	//--- Lider sunucuya kayıt---
	time.Sleep(500 * time.Millisecond) // sunucu hazır olana kadar bekle
	registerToLeader(leaderAddr, myAddr)
	startHeartbeat(leaderAddr, myAddr)

	select {} // programı açık tut
}

func registerToLeader(leaderAddr, myAddr string) {
	conn, err := grpc.Dial(
		leaderAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)

	if err != nil {
		log.Fatalf("Lider bağlantı hatası: %v", err)
	}
	defer conn.Close()

	client := proto.NewStorageServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = client.RegisterMember(ctx, &proto.MemberInfo{Address: myAddr})
	if err != nil {
		log.Fatalf("Lider kaydı başarısız: %v", err)
	}
	log.Printf("Lider sunucuya başarıyla kaydedildi: %s", myAddr)
}

func startHeartbeat(leaderAddr, myAddr string) {
	go func() {
		for {
			conn, err := grpc.Dial(
				leaderAddr,
				grpc.WithTransportCredentials(insecure.NewCredentials()),
			)
			if err != nil {
				log.Printf("[HB] Leader unreachable: %v", err)
				time.Sleep(5 * time.Second)
				continue
			}

			client := proto.NewStorageServiceClient(conn)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)

			_, err = client.Heartbeat(ctx, &proto.HeartbeatRequest{
				Address: myAddr,
			})

			cancel()
			conn.Close()

			if err != nil {
				log.Printf("[HB] Heartbeat failed: %v", err)
			}

			time.Sleep(5 * time.Second)
		}
	}()
}

// go run cmd/member/main.go -port=5556
