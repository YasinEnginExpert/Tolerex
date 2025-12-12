package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
)

func main() {
	//---Liderin portu---
	address := "localhost:6666"

	conn, err := net.Dial("tcp", address)
	if err != nil {
		fmt.Printf("Bağlanti hatasi: %v\n", err)
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

		//---Komutu sunucuya gönder---
		_, err := fmt.Fprintf(conn, "%s\n", line)
		if err != nil {
			fmt.Printf("Gönderim hatasi: %v\n", err)
			continue
		}

		//---Sunucudan yanıt oku---
		response, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			fmt.Printf("Yanit okunamadi: %v\n", err)
			continue
		}
		fmt.Printf("Sunucu: %s", response)
	}
}
