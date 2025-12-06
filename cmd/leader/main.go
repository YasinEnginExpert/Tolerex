package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"time"
	"tolerex/internal/server"
)

func main() {

	//---TCP portu dinlenmeye baslar---
	port := flag.String("port", "6666", "Lider TCP portu")
	confPath := flag.String("conf", "config/tolerance.conf", "Tolerance config dosyasi")
	memberCount := flag.Int("count", 4, "Üye sayısı")
	flag.Parse()

	//Uye adresleri yukle
	members := generateMemberPorts(5555, *memberCount)

	// Lider sunucuyu başlat
	leader, err := server.NewLeaderServer(members, *confPath)

	// Üye durumlarını periyodik bastır
	go func() {
		for {
			time.Sleep(15 * time.Second)
			leader.PrintMemberStats()
		}
	}()

	//---TCP sunucusu aç---
	address := ":" + *port
	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatalf("Lider TCP başlatılamadı: %v", err)
	}
	defer listener.Close()
	fmt.Printf("Lider %s portunda dinliyor...\n", address)

	//---İstemci bağlantılarını dinle---
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Bağlantı hatası: %v", err)
			continue
		}
		go leader.HandleClient(conn) // Her bağlantı ayrı goroutine
	}
}

func generateMemberPorts(startPort, count int) []string {
	var members []string
	for i := 0; i < count; i++ {
		members = append(members, fmt.Sprintf("localhost:%d", startPort+i))
	}
	return members
}
