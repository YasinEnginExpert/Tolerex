package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// ================= CONFIG =================

const (
	DEFAULT_ADDR     = "localhost:6666"
	DIAL_TIMEOUT     = 3 * time.Second
	RETRY_DELAY      = 2 * time.Second
	BULK_FLUSH_EVERY = 100
	RESULT_FILE      = "results/measured_values.json"
)

// ================= FLAGS =================

var (
	addr = flag.String("addr", DEFAULT_ADDR, "Leader TCP address (e.g., localhost:6666)")
)

// ================= CONNECTION =================

func connectLoop(address string) net.Conn {
	for {
		dialer := net.Dialer{Timeout: DIAL_TIMEOUT}
		conn, err := dialer.Dial("tcp", address)
		if err == nil {
			if tcp, ok := conn.(*net.TCPConn); ok {
				_ = tcp.SetNoDelay(true)
			}
			return conn
		}
		time.Sleep(RETRY_DELAY)
	}
}

// ================= MAIN =================

func main() {
	flag.Parse()

	conn := connectLoop(*addr)
	defer conn.Close()

	// SERVER -> STDOUT (pure telnet behavior)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(os.Stdout, conn)
	}()

	// STDIN -> SERVER
	stdin := bufio.NewScanner(os.Stdin)
	writer := bufio.NewWriter(conn)

	for {
		if !stdin.Scan() {
			return
		}

		line := strings.TrimSpace(stdin.Text())
		if line == "" {
			continue
		}

		if strings.EqualFold(line, "EXIT") || strings.EqualFold(line, "QUIT") {
			_, _ = writer.WriteString("QUIT\r\n")
			_ = writer.Flush()
			<-done
			return
		}

		fields := strings.Fields(line)
		if len(fields) >= 3 {
			if bulkCount, err := strconv.Atoi(fields[0]); err == nil {
				if strings.EqualFold(fields[1], "SET") {

					baseMsg := strings.Join(fields[2:], " ")
					start := time.Now()

					for i := 1; i <= bulkCount; i++ {
						cmd := fmt.Sprintf("SET %d %s_%d\r\n", i, baseMsg, i)
						if _, err := writer.WriteString(cmd); err != nil {
							return
						}

						if i%BULK_FLUSH_EVERY == 0 {
							if err := writer.Flush(); err != nil {
								return
							}
						}
					}

					_ = writer.Flush()

					total := time.Since(start)
					avg := total / time.Duration(bulkCount)

					fmt.Printf(
						"BULK DONE: %d SET in %d ms (avg %d us)\n",
						bulkCount,
						total.Milliseconds(),
						avg.Microseconds(),
					)
					continue
				}
			}
		}

		// NORMAL COMMAND
		if _, err := writer.WriteString(line + "\r\n"); err != nil {
			return
		}
		if err := writer.Flush(); err != nil {
			return
		}
	}
}
