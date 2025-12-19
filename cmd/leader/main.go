package main

import (
	"net"
	"net/http"
	"time"

	"tolerex/internal/logger"
	"tolerex/internal/middleware"
	"tolerex/internal/server"
	proto "tolerex/proto/gen"

	"tolerex/internal/security"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
)

/*
	ENTRY POINT – LEADER NODE

	This file is the main entry point of the Leader node in the Tolerex
	distributed storage system.

	Responsibilities of this file:
	- Initialize logging infrastructure
	- Load configuration and create the Leader server
	- Start secured gRPC server (mTLS enabled)
	- Start internal TCP command interface
	- Expose Prometheus-compatible metrics endpoint
	- Periodically print live member statistics

	Business logic is intentionally NOT implemented here.
	This file only orchestrates infrastructure and lifecycle components.
*/
func main() {
	var grpcServer *grpc.Server

	/*
		LOGGER INITIALIZATION

		Initializes the global logging infrastructure with:
		- Log rotation (lumberjack)
		- Leader-specific logger instance
		- Global log level configuration

		Logging is initialized once at startup and shared across the application.
	*/
	logger.Init()
	logger.SetLevel(logger.INFO)

	log := logger.Leader
	logger.Info(log, "Leader server starting...")

	/*
		NETWORK PORT CONFIGURATION

		- grpcPort: secure gRPC endpoint used for Leader <-> Member communication
		- tcpPort : local TCP interface used for interactive client commands

		TCP interface is bound to localhost only for security reasons.
	*/
	grpcPort := ":5555"
	tcpPort := ":6666"

	/*
		CONFIGURATION

		- tolerance.conf defines the replication tolerance level
		- initial member list is empty (members self-register at runtime)
	*/
	confPath := "D:/Tolerex/config/tolerance.conf"
	members := []string{}

	/*
		LEADER SERVER INITIALIZATION

		Creates the LeaderServer instance:
		- Loads replication tolerance
		- Initializes in-memory state
		- Prepares mTLS client credentials for Member communication
	*/
	leader, err := server.NewLeaderServer(members, confPath)
	if err != nil {
		logger.Fatal(log, "Leader initialization failed: %v", err)
	}

	/*
		HEARTBEAT WATCHER

		Starts a background goroutine that:
		- Periodically checks member liveness
		- Marks members as down if heartbeat timeout is exceeded
	*/
	leader.StartHeartbeatWatcher()

	/*
		gRPC SERVER (mTLS PROTECTED)

		Starts the secure gRPC server used for:
		- Member registration
		- Heartbeats
		- Store / Retrieve RPC calls

		The server is protected using mutual TLS authentication.
	*/
	go func() {
		lis, err := net.Listen("tcp", grpcPort)
		if err != nil {
			logger.Fatal(log, "Failed to listen on gRPC port %s: %v", grpcPort, err)
		}

		/*
			Load server-side mTLS credentials:
			- Leader certificate and private key
			- Trusted CA certificate
		*/
		creds, err := security.NewMTLSServerCreds(
			"config/tls/leader.crt",
			"config/tls/leader.key",
			"config/tls/ca.crt",
		)
		if err != nil {
			logger.Fatal(log, "Failed to load mTLS credentials: %v", err)
		}

		/*
			gRPC server is created with:
			- mTLS transport credentials
			- Recovery interceptor (panic protection)
			- Request ID interceptor (traceability)
			- Logging interceptor (observability)
			- Metrics interceptor (Prometheus)
		*/
		grpcServer = grpc.NewServer(
			grpc.Creds(creds),
			grpc.ChainUnaryInterceptor(
				middleware.RecoveryInterceptor("leader"),
				middleware.RequestIDInterceptor(),
				middleware.LoggingInterceptor("leader"),
				middleware.MetricsInterceptor(),
			),
		)

		/*
			Register LeaderServer implementation
			to the generated gRPC service interface.
		*/
		proto.RegisterStorageServiceServer(grpcServer, leader)

		logger.Info(log, "Leader gRPC server listening on %s", grpcPort)

		if err := grpcServer.Serve(lis); err != nil {
			logger.Fatal(log, "gRPC server error: %v", err)
		}
	}()

	/*
		TCP COMMAND SERVER (LOCAL ONLY)

		Starts a TCP server bound to 127.0.0.1 that provides
		an interactive command interface for clients.

		This interface supports commands like:
		- SET
		- GET
		- PWD
		- DIR

		It is intentionally NOT exposed to the network.
	*/
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

	/*
		PROMETHEUS METRICS ENDPOINT

		Exposes application and runtime metrics at:
		http://localhost:9090/metrics

		Metrics include:
		- Go runtime stats
		- Goroutine count
		- Memory usage
		- Custom gRPC metrics
	*/
	go func() {
		logger.Info(log, "Metrics server listening on :9090")
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())

		if err := http.ListenAndServe(":9090", mux); err != nil {
			logger.Warn(log, "Metrics server stopped: %v", err)
		}
	}()

	/*
		LIVE MEMBER STATUS DISPLAY

		Periodically clears the terminal and prints
		current member states including:
		- Address
		- Alive status
		- Last heartbeat time
		- Message count

		This is intended for operator visibility,
		not for persistent logging.
	*/
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		leader.PrintMemberStats()
	}
}
