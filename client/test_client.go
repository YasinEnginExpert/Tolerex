package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
)

func main() {
	// Liderin TCP adresi (gerekirse değiştir)
	address := "localhost:6666"

	conn, err := net.Dial("tcp", address)
	if err != nil {
		fmt.Printf("Bağlantı hatası: %v\n", err)
		return
	}
	defer conn.Close()

	reader := bufio.NewReader(os.Stdin)
	fmt.Println("Mesaj istemcisine hoş geldiniz (SET <id> <msg> / GET <id>)")

	for {
		fmt.Print("> ")
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)

		if line == "exit" {
			break
		}

		// Komutu sunucuya gönder
		_, err := fmt.Fprintf(conn, "%s\n", line)
		if err != nil {
			fmt.Printf("Gönderim hatası: %v\n", err)
			continue
		}

		// Sunucudan yanıt oku
		response, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			fmt.Printf("Yanıt okunamadı: %v\n", err)
			continue
		}
		fmt.Printf("Sunucu: %s", response)
	}
}
