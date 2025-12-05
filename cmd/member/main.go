package main

import (
	"log"
	"net"
	"tolerex/internal/server"
	proto "tolerex/proto/gen"

	"google.golang.org/grpc"
)

func main() {

	//---TCP portu dinlenmeye baslar---
	listener, err := net.Listen("tcp", "5556")
	if err != nil {
		log.Fatalf("Port dinlenemedi: %v", err)
	}

	//---Bos bir gRPC suncuusu olusturulur---
	grpcServer := grpc.NewServer()

	//---StorageService'si register edilir
	proto.RegisterStorageServiceServer(grpcServer, &server.MemberServer{})

	//---Server baslatilir
	log.Println("Uye sunucu calisiyor :5556")
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("gRPC baslatilamadi: %v", err)
	}
}
