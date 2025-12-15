package main

import (
	"context"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
	"tolerex/internal/logger"
	"tolerex/internal/middleware"
	"tolerex/internal/server"

	proto "tolerex/proto/gen"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	//--- Loglama Islemi ---
	logger.Init()
	logger.Member.Println("Member server starting...")

	//--- Parametre Tahsisi ---
	port := flag.String("port", "5556", "gRPC port")
	flag.Parse()

	// --- Sabit adres---
	leaderAddr := "localhost:5555"

	myAddr := "localhost:" + *port

	//--- islem bitince sil---
	dataDir := filepath.Join("internal", "data", "member-"+*port)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatalf("DataDir oluşturulamadı: %v", err)
	}
	cleanup := func() {
		os.RemoveAll(dataDir)
	}
	defer cleanup()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sig
		cleanup()
		os.Exit(0)
	}()

	//--- gRPC sunucusu ayrı goroutine'de başlatılır
	go func() {
		listener, err := net.Listen("tcp", ":"+*port)
		if err != nil {
			log.Fatalf("Port dinlenemedi: %v", err)
		}

		grpcServer := grpc.NewServer(
			grpc.ChainUnaryInterceptor(
				middleware.RecoveryInterceptor("member"),
				middleware.RequestIDInterceptor(),
				middleware.LoggingInterceptor("member"),
				middleware.MetricsInterceptor(),
			),
		)

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
