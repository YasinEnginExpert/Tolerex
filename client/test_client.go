package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
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
	addr  = flag.String("addr", DEFAULT_ADDR, "Leader TCP address (e.g., localhost:6666)")
	count = flag.Int("count", 0, "Bulk SET count (0 = normal interactive)")
)

// ================= METRICS =================

type ClientResult struct {
	Mode      string `json:"io_mode"`
	NodeCount int    `json:"node_count"`
	Messages  int    `json:"messages"`
	TotalMs   int64  `json:"total_ms"`
	AvgUs     int64  `json:"avg_us"`
}

type ClusterRuntime struct {
	IOMode    string `json:"io_mode"`
	NodeCount int    `json:"node_count"`
}

func readClusterRuntime() ClusterRuntime {
	var rt ClusterRuntime

	path := "internal/data/cluster_runtime.json"

	data, err := os.ReadFile(path)
	if err != nil {
		return rt
	}

	_ = json.Unmarshal(data, &rt)

	_ = os.Remove(path)

	return rt
}

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

// ================= RESULT STORAGE =================

func appendResultToFile(r ClientResult) {
	_ = os.MkdirAll("results", 0755)

	var results []ClientResult

	if data, err := os.ReadFile(RESULT_FILE); err == nil && len(data) > 0 {
		_ = json.Unmarshal(data, &results)
	}

	results = append(results, r)

	if b, err := json.MarshalIndent(results, "", "  "); err == nil {
		_ = os.WriteFile(RESULT_FILE, b, 0644)
	}
}

// ================= MAIN =================

func main() {
	bulkDone := false
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

		upper := strings.ToUpper(line)

		// EXIT
		if upper == "EXIT" || upper == "QUIT" {
			_, _ = writer.WriteString("QUIT\r\n")
			_ = writer.Flush()
			<-done
			return
		}

		// BULK SET MODE (MEASURE + JSON, NO EXTRA TERMINAL OUTPUT)
		if *count > 0 && !bulkDone && strings.HasPrefix(upper, "SET ") {
			parts := strings.SplitN(line, " ", 3)
			if len(parts) < 3 {
				// Orijinal davranış: kullanıcı uyarısı vardı
				fmt.Println("Invalid SET format. Use: SET <id> <message>")
				continue
			}

			baseMsg := parts[2]

			start := time.Now()

			for i := 1; i <= *count; i++ {
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

			if err := writer.Flush(); err != nil {
				return
			}

			total := time.Since(start)
			avg := total / time.Duration(*count)

			rt := readClusterRuntime()

			appendResultToFile(ClientResult{
				Mode:      rt.IOMode,
				NodeCount: rt.NodeCount,
				Messages:  *count,
				TotalMs:   total.Milliseconds(),
				AvgUs:     avg.Microseconds(),
			})
			

			continue
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
