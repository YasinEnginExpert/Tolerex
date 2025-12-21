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
	"tolerex/internal/security"
	"tolerex/internal/server"
	proto "tolerex/proto/gen"

	"google.golang.org/grpc"
)

func main() {
	// --- LOGGER INITIALIZATION ---
	logger.Init()
	logger.SetLevel(logger.INFO)

	memberLog := logger.Member
	logger.Info(memberLog, "Member server starting...")

	// --- FLAGS ---
	port := flag.String("port", "5556", "gRPC port")
	ioMode := flag.String("io", "buffered", "disk IO mode: buffered | unbuffered")
	flag.Parse()

	leaderAddr := "localhost:5555"
	myAddr := "localhost:" + *port

	// --- DATA DIR ---
	dataDir := filepath.Join("internal", "data", "member-"+*port)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		logger.Fatal(memberLog, "Data directory could not be created: %v", err)
	}

	cleanup := func() { _ = os.RemoveAll(dataDir) }
	defer cleanup()

	// --- GRACEFUL SHUTDOWN ---
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sig
		logger.Warn(memberLog, "Shutdown signal received, cleaning up")
		cleanup()
		os.Exit(0)
	}()

	// --- START MEMBER gRPC SERVER ---
	go startMemberGRPC(memberLog, *port, dataDir, *ioMode)

	// --- REGISTER TO LEADER ---
	time.Sleep(500 * time.Millisecond)
	registerToLeader(memberLog, leaderAddr, myAddr)

	// --- HEARTBEAT LOOP ---
	startHeartbeat(memberLog, leaderAddr, myAddr)

	// keep process alive
	select {}
}

func startMemberGRPC(memberLog *log.Logger, port, dataDir, ioMode string) {
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		logger.Fatal(memberLog, "Failed to listen on port %s: %v", port, err)
	}
	logger.Info(memberLog, "Starting gRPC server on port %s", port)

	creds, err := security.NewMTLSServerCreds(
		"config/tls/member.crt",
		"config/tls/member.key",
		"config/tls/ca.crt",
	)
	if err != nil {
		logger.Fatal(memberLog, "Failed to load mTLS credentials: %v", err)
	}

	grpcServer := grpc.NewServer(
		grpc.Creds(creds),
		grpc.ChainUnaryInterceptor(
			middleware.RecoveryInterceptor("member"),
			middleware.RequestIDInterceptor(),
			middleware.LoggingInterceptor("member"),
			middleware.MetricsInterceptor(),
		),
	)

	member := &server.MemberServer{
		DataDir: dataDir,
		IOMode:  ioMode,
	}
	proto.RegisterStorageServiceServer(grpcServer, member)

	logger.Info(memberLog, "Member running on port %s, dataDir=%s io=%s", port, dataDir, ioMode)

	if err := grpcServer.Serve(listener); err != nil {
		logger.Fatal(memberLog, "gRPC server failed: %v", err)
	}
}

// --- LEADER REGISTRATION ROUTINE ---
func registerToLeader(memberLog *log.Logger, leaderAddr, myAddr string) {
	logger.Info(memberLog, "Registering to leader at %s as %s", leaderAddr, myAddr)

	creds, err := security.NewMTLSClientCreds(
		"config/tls/member.crt",
		"config/tls/member.key",
		"config/tls/ca.crt",
		"leader",
	)
	if err != nil {
		logger.Fatal(memberLog, "mTLS client credentials error: %v", err)
	}

	conn, err := grpc.Dial(leaderAddr, grpc.WithTransportCredentials(creds))
	if err != nil {
		logger.Fatal(memberLog, "Failed to connect to leader: %v", err)
	}
	defer conn.Close()

	client := proto.NewStorageServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = client.RegisterMember(ctx, &proto.MemberInfo{Address: myAddr})
	if err != nil {
		logger.Fatal(memberLog, "Leader registration failed: %v", err)
	}

	logger.Info(memberLog, "Successfully registered to leader: %s", myAddr)
}

// --- HEARTBEAT LOOP ---
func startHeartbeat(memberLog *log.Logger, leaderAddr, myAddr string) {
	logger.Info(memberLog, "Heartbeat loop started (interval=5s, leader=%s)", leaderAddr)

	creds, err := security.NewMTLSClientCreds(
		"config/tls/member.crt",
		"config/tls/member.key",
		"config/tls/ca.crt",
		"leader",
	)
	if err != nil {
		logger.Fatal(memberLog, "mTLS client credentials error: %v", err)
	}

	go func() {
		for {
			conn, err := grpc.Dial(leaderAddr, grpc.WithTransportCredentials(creds))
			if err != nil {
				logger.Warn(memberLog, "Leader unreachable: %v", err)
				time.Sleep(5 * time.Second)
				continue
			}

			client := proto.NewStorageServiceClient(conn)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_, err = client.Heartbeat(ctx, &proto.HeartbeatRequest{Address: myAddr})
			cancel()
			conn.Close()

			if err != nil {
				logger.Warn(memberLog, "Heartbeat failed: %v", err)
			}

			time.Sleep(5 * time.Second)
		}
	}()
}

/*
USAGE EXAMPLE:
  go run cmd/member/main.go -port=5556 -io=buffered
*/
