package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

const PROMPT = "tolerex>"

func connect(address string) net.Conn {
	for {
		conn, err := net.Dial("tcp", address)
		if err == nil {
			fmt.Println("Leader bağlantısı kuruldu.")
			return conn
		}
		fmt.Println("Leader'a bağlanılamadı, tekrar deneniyor...")
		time.Sleep(2 * time.Second)
	}
}

func main() {
	address := "localhost:6666"

	conn := connect(address)
	defer conn.Close()

	stdin := bufio.NewReader(os.Stdin)
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	// ---- Banner + ilk prompt'u oku ----
	readUntilPrompt(reader)

	for {
		fmt.Print("> ")
		line, _ := stdin.ReadString('\n')
		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}

		if line == "exit" || line == "quit" {
			fmt.Println("Çıkış yapılıyor...")
			fmt.Fprintf(writer, "QUIT\r\n")
			writer.Flush()
			return
		}

		// ---- Komut gönder ----
		_, err := fmt.Fprintf(writer, "%s\r\n", line)
		writer.Flush()
		if err != nil {
			fmt.Println("Bağlantı koptu, tekrar bağlanılıyor...")
			conn.Close()
			conn = connect(address)
			reader = bufio.NewReader(conn)
			writer = bufio.NewWriter(conn)
			readUntilPrompt(reader)
			continue
		}

		// ---- Cevabı oku ----
		if strings.HasPrefix(strings.ToUpper(line), "DIR") {
			readDirResponse(reader)
		} else {
			readUntilPrompt(reader)
		}
	}
}

//
// -------- Prompt görünene kadar okur (newline gerekmez) --------
//
func readUntilPrompt(reader *bufio.Reader) {
	var buf strings.Builder

	for {
		b, err := reader.ReadByte()
		if err != nil {
			fmt.Println("\nSunucu bağlantısı koptu.")
			os.Exit(1)
		}

		buf.WriteByte(b)
		if strings.Contains(buf.String(), PROMPT) {
			output := buf.String()
			output = strings.ReplaceAll(output, PROMPT, "")
			output = strings.Trim(output, "\r\n")
			if output != "" {
				fmt.Println(output)
			}
			return
		}
	}
}

//
// -------- DIR için END'e kadar okur --------
//
func readDirResponse(reader *bufio.Reader) {
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("DIR okunamadı.")
			return
		}

		line = strings.TrimRight(line, "\r\n")

		if line == "END" {
			readUntilPrompt(reader)
			return
		}

		fmt.Println(line)
	}
}
