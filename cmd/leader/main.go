// ===================================================================================
// TOLEREX – LEADER NODE BOOTSTRAP
// ===================================================================================
//
// This file represents the infrastructure-level bootstrapper of the Leader node
// in the Tolerex distributed, fault-tolerant storage system.
//
// From a systems perspective, this file is responsible for orchestrating the
// lifecycle of all runtime components but deliberately avoids implementing
// any business or domain logic.
//
// Architecturally, this entry point:
//
// - Initializes the global logging subsystem with rotation and severity control
// - Loads system-level configuration such as replication tolerance
// - Constructs the LeaderServer instance (core coordination authority)
// - Boots a secure gRPC server protected with mutual TLS (mTLS)
// - Exposes a localhost-only TCP control plane for operator commands
// - Publishes Prometheus-compatible metrics for observability
// - Launches background goroutines for heartbeat supervision
// - Periodically renders real-time cluster state for operational insight
//
// This file acts as the **composition root** of the Leader process.
// All domain behavior is delegated to internal packages (server, middleware,
// security, logger), enforcing clean separation of concerns.
//
// ===================================================================================

package main

import (
	"net"
	"net/http"
	"path/filepath"
	"time"

	"tolerex/internal/logger"
	"tolerex/internal/middleware"
	"tolerex/internal/security"
	"tolerex/internal/server"
	proto "tolerex/proto/gen"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
)

const (
	grpcPort    = ":5555"
	tcpPort     = ":6666"
	metricsPort = ":9090"
)

func main() {

	// --- gRPC SERVER HANDLE ---
	// Holds a reference to the gRPC server instance so that
	// it can be managed or extended later if required.

	// --- LOGGER INITIALIZATION ---
	// Initializes the global logging subsystem:
	// - Log rotation (lumberjack)
	// - Leader-specific logger instance
	// - Runtime-adjustable log level
	logger.Init()
	logger.SetLevel(logger.INFO)

	log := logger.Leader
	logger.Info(log, "Leader server starting...")

	// --- NETWORK PORT CONFIGURATION ---
	// grpcPort:
	//   Secure gRPC endpoint for Leader ↔ Member communication (mTLS)
	//
	// tcpPort:
	//   Local-only TCP control interface for interactive commands

	// --- CONFIGURATION LOADING ---
	// tolerance.conf defines the replication tolerance factor (N-fault tolerance)
	// Initial member list is empty; members self-register dynamically.
	confPath := filepath.Join("config", "tolerance.conf")

	members := []string{}

	// --- LEADER SERVER CONSTRUCTION ---
	// Creates the LeaderServer instance which:
	// - Loads replication tolerance
	// - Initializes in-memory cluster state
	// - Prepares secure client credentials for member communication
	leader, err := server.NewLeaderServer(members, confPath)
	if err != nil {
		logger.Fatal(log, "Leader initialization failed: %v", err)
	}

	// --- HEARTBEAT SUPERVISOR ---
	// Starts a background watcher that:
	// - Tracks periodic heartbeats from members
	// - Detects failures via timeout
	// - Marks unreachable members as inactive
	leader.StartHeartbeatWatcher()

	// --- gRPC SERVER (mTLS-PROTECTED DATA PLANE) ---
	// Responsible for:
	// - Member registration
	// - Heartbeat processing
	// - Distributed Store / Retrieve RPCs
	go func() {
		lis, err := net.Listen("tcp", grpcPort)
		if err != nil {
			logger.Fatal(log, "Failed to listen on gRPC port %s: %v", grpcPort, err)
		}

		// --- mTLS CREDENTIAL LOADING ---
		// Loads:
		// - Leader certificate & private key
		// - Trusted CA certificate
		creds, err := security.NewMTLSServerCreds(
			"config/tls/leader.crt",
			"config/tls/leader.key",
			"config/tls/ca.crt",
		)
		if err != nil {
			logger.Fatal(log, "Failed to load mTLS credentials: %v", err)
		}

		// --- gRPC SERVER CREATION ---
		// Server is configured with:
		// - Mutual TLS transport security
		// - Panic recovery interceptor
		// - Request ID propagation
		// - Structured logging
		// - Prometheus metrics instrumentation
		grpcServer := grpc.NewServer(
			grpc.Creds(creds),
			grpc.ChainUnaryInterceptor(
				middleware.RecoveryInterceptor("leader"),
				middleware.RequestIDInterceptor(),
				middleware.LoggingInterceptor("leader"),
				middleware.MetricsInterceptor(),
			),
		)

		// --- SERVICE REGISTRATION ---
		// Binds LeaderServer implementation to gRPC service definition
		proto.RegisterStorageServiceServer(grpcServer, leader)

		logger.Info(log, "Leader gRPC server listening on %s", grpcPort)

		if err := grpcServer.Serve(lis); err != nil {
			logger.Fatal(log, "gRPC server error: %v", err)
		}
	}()

	// --- TCP COMMAND INTERFACE (CONTROL PLANE) ---
	// Localhost-only TCP server that:
	// - Accepts operator commands
	// - Supports SET / GET commands
	// - Is intentionally isolated from the network
	go func() {
		listener, err := net.Listen("tcp", "127.0.0.1"+tcpPort)
		if err != nil {
			logger.Fatal(log, "Failed to listen on TCP port %s: %v", tcpPort, err)
		}
		defer listener.Close()

		logger.Info(log, "Leader TCP server listening on %s", tcpPort)

		for {
			conn, err := listener.Accept()
			if err != nil {
				logger.Warn(log, "Incoming TCP connection failed: %v", err)
				continue
			}
			go leader.HandleClient(conn)
		}
	}()

	// --- PROMETHEUS METRICS ENDPOINT ---
	// Exposes runtime and application metrics at:
	// http://localhost:9090/metrics
	go func() {
		logger.Info(log, "Metrics server listening on :9090")
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())

		if err := http.ListenAndServe(metricsPort, mux); err != nil {
			logger.Warn(log, "Metrics server stopped: %v", err)
		}
	}()

	// --- LIVE CLUSTER STATE DISPLAY ---
	// Periodically refreshes terminal output with:
	// - Member addresses
	// - Liveness state
	// - Last heartbeat timestamp
	// - Stored message counts
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		leader.PrintMemberStats()
	}
}
