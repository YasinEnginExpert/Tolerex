// ===================================================================================
// TOLEREX – MEMBER NODE BOOTSTRAP
// ===================================================================================
//
// This file is the infrastructure-level bootstrapper of a Member node
// in the Tolerex distributed, fault-tolerant storage system.
//
// From a distributed systems perspective, a Member node acts as a
// stateful storage worker responsible for persisting data replicas
// assigned by the Leader.
//
// Architecturally, this entry point:
//
// - Initializes the logging subsystem for Member context
// - Parses runtime parameters to allow multi-instance deployment
// - Creates and manages an isolated on-disk storage directory
// - Boots a secure gRPC server protected via mutual TLS (mTLS)
// - Registers itself dynamically to the Leader node
// - Periodically emits heartbeat signals to indicate liveness
// - Handles OS-level shutdown signals for graceful termination
//
// This file contains **no domain or storage logic**.
// All business responsibilities are delegated to server.MemberServer.
//
// The design enforces:
// - Clear separation between infrastructure and business logic
// - Secure-by-default communication
// - Fault detection via heartbeat-based supervision
//
// ===================================================================================

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
	// Initializes the logging infrastructure for the Member node.
	// Logs share the global backend but are tagged with [MEMBER].
	logger.Init()
	logger.SetLevel(logger.INFO)

	log := logger.Member
	logger.Info(log, "Member server starting...")

	// --- RUNTIME PARAMETER PARSING ---
	// The gRPC port is provided via command-line flag.
	// This enables running multiple Member instances on the same host.
	port := flag.String("port", "5556", "gRPC port")
	flag.Parse()

	leaderAddr := "localhost:5555"
	myAddr := "localhost:" + *port

	// --- DATA DIRECTORY INITIALIZATION ---
	// Each Member persists its data in an isolated directory:
	//   internal/data/member-<port>
	//
	// This directory is treated as ephemeral and removed on shutdown.
	dataDir := filepath.Join("internal", "data", "member-"+*port)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		logger.Fatal(log, "Data directory could not be created: %v", err)
	}

	cleanup := func() {
		os.RemoveAll(dataDir)
	}
	defer cleanup()

	// --- SIGNAL HANDLING (GRACEFUL SHUTDOWN) ---
	// Listens for OS interrupt and termination signals.
	// Ensures storage cleanup before process termination.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sig
		logger.Warn(log, "Shutdown signal received, cleaning up")
		cleanup()
		os.Exit(0)
	}()

	// --- gRPC SERVER (mTLS-PROTECTED DATA PLANE) ---
	// Starts the Member-side gRPC server responsible for:
	// - Store RPC: persisting messages to disk
	// - Retrieve RPC: reading messages from disk
	go func() {
		listener, err := net.Listen("tcp", ":"+*port)
		if err != nil {
			logger.Fatal(log, "Failed to listen on port %s: %v", *port, err)
		}

		logger.Info(log, "Starting gRPC server on port %s", *port)

		// --- mTLS CREDENTIAL LOADING ---
		// Loads Member certificate, private key, and trusted CA.
		creds, err := security.NewMTLSServerCreds(
			"config/tls/member.crt",
			"config/tls/member.key",
			"config/tls/ca.crt",
		)
		if err != nil {
			logger.Fatal(log, "Failed to load mTLS credentials: %v", err)
		}

		// --- gRPC SERVER CREATION ---
		// Configured with:
		// - Mutual TLS authentication
		// - Panic recovery
		// - Request ID propagation
		// - Structured logging
		// - Prometheus metrics instrumentation
		grpcServer := grpc.NewServer(
			grpc.Creds(creds),
			grpc.ChainUnaryInterceptor(
				middleware.RecoveryInterceptor("member"),
				middleware.RequestIDInterceptor(),
				middleware.LoggingInterceptor("member"),
				middleware.MetricsInterceptor(),
			),
		)

		// --- SERVICE REGISTRATION ---
		// Registers the MemberServer which implements
		// Store and Retrieve operations.
		member := &server.MemberServer{
			DataDir: dataDir,
		}
		proto.RegisterStorageServiceServer(grpcServer, member)

		logger.Info(log, "Member running on port %s, dataDir=%s", *port, dataDir)

		if err := grpcServer.Serve(listener); err != nil {
			logger.Fatal(log, "gRPC server failed: %v", err)
		}
	}()

	// --- LEADER REGISTRATION ---
	// After startup, the Member securely registers itself
	// to the Leader using an mTLS client connection.
	time.Sleep(500 * time.Millisecond)
	registerToLeader(log, leaderAddr, myAddr)

	// --- HEARTBEAT EMISSION LOOP ---
	// Periodically sends heartbeat messages to the Leader
	// to indicate liveness and availability.
	startHeartbeat(log, leaderAddr, myAddr)

	// --- MAIN GOROUTINE BLOCK ---
	// All lifecycle management is handled via goroutines.
	// This select{} prevents main() from exiting.
	select {}
}

// --- LEADER REGISTRATION ROUTINE ---
// Establishes a secure mTLS connection to the Leader
// and registers this Member’s network address.
func registerToLeader(log *log.Logger, leaderAddr, myAddr string) {
	logger.Info(log, "Registering to leader at %s as %s", leaderAddr, myAddr)

	creds, err := security.NewMTLSClientCreds(
		"config/tls/member.crt",
		"config/tls/member.key",
		"config/tls/ca.crt",
		"leader",
	)
	if err != nil {
		logger.Fatal(log, "mTLS client credentials error: %v", err)
	}

	conn, err := grpc.Dial(
		leaderAddr,
		grpc.WithTransportCredentials(creds),
	)
	if err != nil {
		logger.Fatal(log, "Failed to connect to leader: %v", err)
	}
	defer conn.Close()

	client := proto.NewStorageServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = client.RegisterMember(ctx, &proto.MemberInfo{Address: myAddr})
	if err != nil {
		logger.Fatal(log, "Leader registration failed: %v", err)
	}

	logger.Info(log, "Successfully registered to leader: %s", myAddr)
}

// --- HEARTBEAT LOOP ---
// Sends heartbeat messages to the Leader at fixed intervals.
// Automatically retries if the Leader becomes unreachable.
func startHeartbeat(log *log.Logger, leaderAddr, myAddr string) {
	logger.Info(log, "Heartbeat loop started (interval=5s, leader=%s)", leaderAddr)

	creds, err := security.NewMTLSClientCreds(
		"config/tls/member.crt",
		"config/tls/member.key",
		"config/tls/ca.crt",
		"leader",
	)
	if err != nil {
		logger.Fatal(log, "mTLS client credentials error: %v", err)
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
