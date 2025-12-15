package main

import (
	"net"
	"net/http"
	"time"

	"tolerex/internal/logger"
	"tolerex/internal/middleware"
	"tolerex/internal/server"
	proto "tolerex/proto/gen"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
)

func main() {
	// -------- Logger init --------
	logger.Init()
	logger.Leader.Println("Leader server starting...")

	// -------- Ports --------
	grpcPort := ":5555"
	tcpPort := ":6666"

	// -------- Config --------
	confPath := "D:/Tolerex/config/tolerance.conf"
	members := []string{}

	// -------- Leader init --------
	leader, err := server.NewLeaderServer(members, confPath)
	if err != nil {
		logger.Leader.Fatalf("Leader init failed: %v", err)
	}

	leader.StartHeartbeatWatcher()

	// -------- gRPC Server --------
	go func() {
		lis, err := net.Listen("tcp", grpcPort)
		if err != nil {
			logger.Leader.Fatalf("Failed to listen on gRPC port %s: %v", grpcPort, err)
		}

		grpcServer := grpc.NewServer(
			grpc.ChainUnaryInterceptor(
				middleware.RecoveryInterceptor("leader"),
				middleware.RequestIDInterceptor(),
				middleware.LoggingInterceptor("leader"),
				middleware.MetricsInterceptor(),
			),
		)
		proto.RegisterStorageServiceServer(grpcServer, leader)

		logger.Leader.Printf("Leader gRPC server listening on %s", grpcPort)

		if err := grpcServer.Serve(lis); err != nil {
			logger.Leader.Fatalf("gRPC server error: %v", err)
		}
	}()

	// -------- TCP Server --------
	go func() {
		listener, err := net.Listen("tcp", tcpPort)
		if err != nil {
			logger.Leader.Fatalf("Failed to listen on TCP port %s: %v", tcpPort, err)
		}
		defer listener.Close()

		logger.Leader.Printf("Leader TCP server listening on %s", tcpPort)

		for {
			conn, err := listener.Accept()
			if err != nil {
				logger.Leader.Printf("Incoming TCP connection failed: %v", err)
				continue
			}
			go leader.HandleClient(conn)
		}
	}()

	//--- Prometheus ----(http://localhost:9090/metrics)
	go func() {
		logger.Leader.Println("Metrics server listening on :9090")
		http.Handle("/metrics", promhttp.Handler())
		http.ListenAndServe(":9090", nil)
	}()

	// -------- Periodic stats --------
	for {
		time.Sleep(30 * time.Second)
		leader.PrintMemberStats()
	}
}
