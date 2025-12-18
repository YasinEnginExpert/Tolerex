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
	// -------- Logger init --------
	logger.Init()
	logger.SetLevel(logger.INFO)

	log := logger.Member
	logger.Info(log, "Member server starting...")

	/// -------- Params --------
	port := flag.String("port", "5556", "gRPC port")
	flag.Parse()

	leaderAddr := "localhost:5555"
	myAddr := "localhost:" + *port

	// -------- Data dir --------
	dataDir := filepath.Join("internal", "data", "member-"+*port)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		logger.Fatal(log, "DataDir oluşturulamadı: %v", err)
	}
	cleanup := func() {
		os.RemoveAll(dataDir)
	}
	defer cleanup()

	// -------- Signal handling --------
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sig
		logger.Warn(log, "Shutdown signal received, cleaning up")
		cleanup()
		os.Exit(0)
	}()

	// -------- gRPC Server --------
	go func() {
		listener, err := net.Listen("tcp", ":"+*port)
		if err != nil {
			logger.Fatal(log, "Port dinlenemedi: %v", err)
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

		logger.Info(log, "Member running on port %s, dataDir=%s", *port, dataDir)

		if err := grpcServer.Serve(listener); err != nil {
			logger.Fatal(log, "gRPC başlatılamadı: %v", err)
		}
	}()

	// -------- Register to leader --------
	time.Sleep(500 * time.Millisecond)
	registerToLeader(log, leaderAddr, myAddr)

	// -------- Heartbeat --------
	startHeartbeat(log, leaderAddr, myAddr)

	select {} // programı açık tut
}

func registerToLeader(log *log.Logger, leaderAddr, myAddr string) {
	conn, err := grpc.Dial(
		leaderAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)

	if err != nil {
		logger.Fatal(log, "Lider bağlantı hatası: %v", err)
	}
	defer conn.Close()

	client := proto.NewStorageServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = client.RegisterMember(ctx, &proto.MemberInfo{Address: myAddr})
	if err != nil {
		logger.Fatal(log, "Lider kaydı başarısız: %v", err)
	}
	log.Printf("Lider sunucuya başarıyla kaydedildi: %s", myAddr)
}

func startHeartbeat(log *log.Logger, leaderAddr, myAddr string) {
	go func() {
		for {
			conn, err := grpc.Dial(
				leaderAddr,
				grpc.WithTransportCredentials(insecure.NewCredentials()),
			)
			if err != nil {
				logger.Warn(log, "Leader unreachable: %v", err)
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
				logger.Warn(log, "Heartbeat failed: %v", err)
			}

			time.Sleep(5 * time.Second)
		}
	}()
}

// go run cmd/member/main.go -port=5556
