// ===================================================================================
// TOLEREX – TCP CLIENT (INTERACTIVE CLI)
// ===================================================================================
//
// This file implements a lightweight interactive TCP client for the Tolerex
// Leader control interface.
//
// From a systems programming perspective, this client:
//
// - Establishes a persistent TCP connection to the Leader's local control port
// - Implements automatic reconnection with retry logic
// - Provides a synchronous, prompt-based command interface
// - Parses and handles multi-line server responses (e.g., DIR output)
// - Separates transport concerns from business logic
//
// This client communicates exclusively over the Leader's localhost-only
// TCP control plane and is intentionally not secured with TLS,
// as it is designed for operator-level local access.
//
// Supported commands include:
// - SET
// - GET
// - DIR
// - PWD
// - QUIT
//
// ===================================================================================

package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

// --- PROMPT DEFINITION ---
// Server-side prompt marker used to detect
// end-of-response boundaries.
const PROMPT = "tolerex>"

// --- CONNECTION HANDSHAKE ---
// Continuously attempts to establish a TCP connection
// to the Leader until successful.
func connect(address string) net.Conn {
	for {
		conn, err := net.Dial("tcp", address)
		if err == nil {
			fmt.Println("Connected to Leader successfully.")
			return conn
		}
		fmt.Println("Unable to connect to Leader, retrying...")
		time.Sleep(2 * time.Second)
	}
}

func main() {

	// --- LEADER CONTROL ADDRESS ---
	// Localhost-only TCP control interface
	address := "localhost:6666"

	// --- INITIAL CONNECTION ---
	conn := connect(address)
	defer conn.Close()

	// --- I/O STREAM SETUP ---
	stdin := bufio.NewReader(os.Stdin)
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	// --- INITIAL BANNER & PROMPT READ ---
	// Reads server greeting until the first prompt appears
	readUntilPrompt(reader)

	for {
		// --- USER INPUT ---
		fmt.Print("> ")
		line, _ := stdin.ReadString('\n')
		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}

		// --- EXIT HANDLING ---
		if line == "exit" || line == "quit" {
			fmt.Println("Exiting client...")
			fmt.Fprintf(writer, "QUIT\r\n")
			writer.Flush()
			return
		}

		// --- SEND COMMAND TO SERVER ---
		_, err := fmt.Fprintf(writer, "%s\r\n", line)
		writer.Flush()
		if err != nil {
			fmt.Println("Connection lost, reconnecting...")
			conn.Close()
			conn = connect(address)
			reader = bufio.NewReader(conn)
			writer = bufio.NewWriter(conn)
			readUntilPrompt(reader)
			continue
		}

		// --- RESPONSE HANDLING ---
		// DIR command returns multi-line output terminated with END
		if strings.HasPrefix(strings.ToUpper(line), "DIR") {
			readDirResponse(reader)
		} else {
			readUntilPrompt(reader)
		}
	}
}

// --- PROMPT-BASED RESPONSE READER ---
// Reads raw bytes until the server prompt is detected.
// Does not rely on newline boundaries.
func readUntilPrompt(reader *bufio.Reader) {
	var buf strings.Builder

	for {
		b, err := reader.ReadByte()
		if err != nil {
			fmt.Println("\nServer connection lost.")
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

// --- DIRECTORY RESPONSE READER ---
// Reads line-by-line output until the END marker is received.
// Used specifically for DIR command responses.
func readDirResponse(reader *bufio.Reader) {
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Failed to read DIR response.")
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
