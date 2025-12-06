package main

import (
	"flag"
	"log"
	"net"
	"tolerex/internal/server"
	proto "tolerex/proto/gen"

	"google.golang.org/grpc"
)

func main() {
	port := flag.String("port", "5556", "gRPC port")
	flag.Parse()

	listener, err := net.Listen("tcp", ":"+*port)
	if err != nil {
		log.Fatalf("Port dinlenemedi: %v", err)
	}

	grpcServer := grpc.NewServer()
	proto.RegisterStorageServiceServer(grpcServer, &server.MemberServer{})

	log.Printf("Üye sunucu çalışıyor :%s\n", *port)
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("gRPC başlatılamadı: %v", err)
	}
}
