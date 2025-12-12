package main

import (
	"fmt"
	"log"
	"net"
	"time"
	"tolerex/internal/server"
	proto "tolerex/proto/gen"

	"google.golang.org/grpc"
)

func main() {
	// --- Sabit portlar ---
	grpcPort := ":5555" // Member → Leader gRPC iletişimi
	tcpPort := ":6666"  // Client → Leader TCP SET/GET

	//-------- Komut satırı argümanları alınır --------
	confPath := "config/tolerance.conf" // Hata toleransı için dosya yolu

	//-------- Baslangıcta uye yok --------
	members := []string{}

	//-------- Lider sunucu başlatılır --------
	leader, err := server.NewLeaderServer(members, confPath)
	leader.DataDir = "data/leader"
	if err != nil {
		log.Fatalf("Lider sunucu baslatilamadi: %v", err)

	}

	leader.StartHeartbeatWatcher()

	// --- gRPC Server ---
	go func() {
		lis, err := net.Listen("tcp", grpcPort)
		if err != nil {
			log.Fatalf("[FATAL] Failed to start gRPC server: %v", err)
		}

		grpcServer := grpc.NewServer()
		proto.RegisterStorageServiceServer(grpcServer, leader)

		log.Printf("[INFO] Leader gRPC server listening on %s\n", grpcPort)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("[FATAL] gRPC server error: %v", err)
		}
	}()

	// --- TCP Server (Client SET/GET) ---
	go func() {
		listener, err := net.Listen("tcp", tcpPort)
		if err != nil {
			log.Fatalf("[FATAL] Failed to start TCP server: %v", err)
		}
		defer listener.Close()

		fmt.Printf("[INFO] Leader TCP server listening on %s\n", tcpPort)

		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Printf("[WARN] Incoming TCP connection failed: %v", err)
				continue
			}
			go leader.HandleClient(conn)
		}
	}()

	// --- Üye istatistiklerini yazdır ---
	for {
		time.Sleep(30 * time.Second)
		leader.PrintMemberStats()
	}
}

// go run cmd/leader/main.go
