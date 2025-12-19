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

/*
ENTRY POINT – MEMBER NODE

This file is the main entry point of a Member node in the Tolerex
distributed storage system.

Responsibilities of this file:
- Initialize logging infrastructure
- Parse runtime parameters (port configuration)
- Create and manage the Member data directory
- Start a secure gRPC server (mTLS enabled)
- Register the Member to the Leader node
- Periodically send heartbeat messages to the Leader
- Handle graceful shutdown and resource cleanup

This file contains NO business logic.
All storage logic is delegated to server.MemberServer.
*/
func main() {

	/*
		LOGGER INITIALIZATION

		Initializes the logging system for the Member node.
		Logs are shared with the Leader but prefixed with [MEMBER].
	*/
	logger.Init()
	logger.SetLevel(logger.INFO)

	log := logger.Member
	logger.Info(log, "Member server starting...")

	/*
		RUNTIME PARAMETERS

		- port: gRPC listening port for this Member instance
		Allows multiple Members to run on the same machine.
	*/
	port := flag.String("port", "5556", "gRPC port")
	flag.Parse()

	leaderAddr := "localhost:5555"
	myAddr := "localhost:" + *port

	/*
		DATA DIRECTORY INITIALIZATION

		Each Member stores its data in an isolated directory:
		internal/data/member-<port>

		The directory is removed on graceful shutdown.
	*/
	dataDir := filepath.Join("internal", "data", "member-"+*port)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		logger.Fatal(log, "DataDir oluşturulamadı: %v", err)
	}

	cleanup := func() {
		os.RemoveAll(dataDir)
	}
	defer cleanup()

	/*
		SIGNAL HANDLING (GRACEFUL SHUTDOWN)

		Listens for OS interrupt signals (Ctrl+C, SIGTERM).
		Ensures data directory cleanup before process exit.
	*/
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sig
		logger.Warn(log, "Shutdown signal received, cleaning up")
		cleanup()
		os.Exit(0)
	}()

	/*
		gRPC SERVER (mTLS PROTECTED)

		Starts the Member gRPC server responsible for:
		- Store RPC (write message to disk)
		- Retrieve RPC (read message from disk)

		The server only accepts mutually authenticated TLS connections.
	*/
	go func() {
		listener, err := net.Listen("tcp", ":"+*port)
		if err != nil {
			logger.Fatal(log, "Port dinlenemedi: %v", err)
		}

		logger.Info(log, "Starting gRPC server on port %s", *port)

		/*
			Load Member-side mTLS credentials:
			- Member certificate and private key
			- Trusted CA certificate
		*/
		creds, err := security.NewMTLSServerCreds(
			"config/tls/member.crt",
			"config/tls/member.key",
			"config/tls/ca.crt",
		)
		if err != nil {
			logger.Fatal(log, "mTLS credentials yüklenemedi: %v", err)
		}

		/*
			gRPC server middleware stack:
			- Recovery interceptor (panic protection)
			- Request ID interceptor (traceability)
			- Logging interceptor (observability)
			- Metrics interceptor (Prometheus)
		*/
		grpcServer := grpc.NewServer(
			grpc.Creds(creds),
			grpc.ChainUnaryInterceptor(
				middleware.RecoveryInterceptor("member"),
				middleware.RequestIDInterceptor(),
				middleware.LoggingInterceptor("member"),
				middleware.MetricsInterceptor(),
			),
		)

		/*
			Register MemberServer implementation
			that handles Store and Retrieve operations.
		*/
		member := &server.MemberServer{
			DataDir: dataDir,
		}
		proto.RegisterStorageServiceServer(grpcServer, member)

		logger.Info(log, "Member running on port %s, dataDir=%s", *port, dataDir)

		if err := grpcServer.Serve(listener); err != nil {
			logger.Fatal(log, "gRPC başlatılamadı: %v", err)
		}
	}()

	/*
		LEADER REGISTRATION

		After startup, the Member registers itself to the Leader
		using a secure mTLS client connection.
	*/
	time.Sleep(500 * time.Millisecond)
	registerToLeader(log, leaderAddr, myAddr)

	/*
		HEARTBEAT LOOP

		Starts a background goroutine that periodically sends
		heartbeat messages to the Leader to signal liveness.
	*/
	startHeartbeat(log, leaderAddr, myAddr)

	/*
		BLOCK MAIN GOROUTINE

		The application lifecycle is fully managed by goroutines.
		This select{} keeps the main goroutine alive indefinitely.
	*/
	select {}
}

/*
REGISTER TO LEADER

Establishes a secure mTLS connection to the Leader and
registers this Member’s network address.
*/
func registerToLeader(log *log.Logger, leaderAddr, myAddr string) {
	logger.Info(log, "Registering to leader at %s as %s", leaderAddr, myAddr)

	creds, err := security.NewMTLSClientCreds(
		"config/tls/member.crt",
		"config/tls/member.key",
		"config/tls/ca.crt",
		"leader",
	)
	if err != nil {
		logger.Fatal(log, "mTLS client creds error: %v", err)
	}

	conn, err := grpc.Dial(
		leaderAddr,
		grpc.WithTransportCredentials(creds),
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

	logger.Info(log, "Successfully registered to leader: %s", myAddr)
}

/*
HEARTBEAT LOOP

Sends heartbeat messages to the Leader at fixed intervals.
If the Leader becomes unreachable, retries indefinitely.
*/
func startHeartbeat(log *log.Logger, leaderAddr, myAddr string) {
	logger.Info(log, "Heartbeat loop started (interval=5s, leader=%s)", leaderAddr)

	creds, err := security.NewMTLSClientCreds(
		"config/tls/member.crt",
		"config/tls/member.key",
		"config/tls/ca.crt",
		"leader",
	)
	if err != nil {
		logger.Fatal(log, "mTLS creds error: %v", err)
	}

	go func() {
		for {
			conn, err := grpc.Dial(
				leaderAddr,
				grpc.WithTransportCredentials(creds),
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

/*
	USAGE EXAMPLE:

	go run cmd/member/main.go -port=5556
*/
