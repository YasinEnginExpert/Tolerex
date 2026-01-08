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

// ===================================================================================
// TOLEREX – TCP CLIENT / BENCHMARK DRIVER
// ===================================================================================
//
// This program is a lightweight TCP client used to interact with the
// Tolerex Leader's **local TCP control plane**.
//
// It behaves like a minimal telnet-style client with two main purposes:
//
//   1) Interactive usage
//      - Manually send SET / GET commands
//      - Observe Leader responses in real time
//
//   2) Benchmark / load testing
//      - Send large batches of SET commands
//      - Measure total and average latency
//
// The client intentionally avoids any application-level protocol logic.
// It simply streams text commands over TCP and prints server output verbatim.
//
// ===================================================================================

// ===================================================================================
// CONFIGURATION CONSTANTS
// ===================================================================================
//
// DEFAULT_ADDR
//   Default TCP address of the Leader control plane.
//
// DIAL_TIMEOUT
//   Maximum time allowed for a single TCP connection attempt.
//
// RETRY_DELAY
//   Backoff delay between connection retry attempts.
//
// BULK_FLUSH_EVERY
//   Number of commands written before forcing a flush when sending bulk SETs.
//   This reduces syscall overhead while preventing excessive buffering.
//
// RESULT_FILE
//   Reserved for future use (e.g., structured benchmark output persistence).

const (
	DEFAULT_ADDR     = "localhost:6666"
	DIAL_TIMEOUT     = 3 * time.Second
	RETRY_DELAY      = 2 * time.Second
	BULK_FLUSH_EVERY = 1000
	RESULT_FILE      = "results/measured_values.json"
)

// ===================================================================================
// COMMAND-LINE FLAGS
// ===================================================================================
//
// addr
//   Specifies the Leader TCP control-plane address.

var (
	addr = flag.String("addr", DEFAULT_ADDR, "Leader TCP address (e.g., localhost:6666)")
)

// ===================================================================================
// TCP CONNECTION HANDLING
// ===================================================================================
//
// connectLoop continuously attempts to establish a TCP connection to the Leader.
// It blocks until a connection is successful.
//
// Behavior:
//   - Uses a bounded dial timeout
//   - Retries indefinitely with a fixed delay
//   - Disables Nagle's algorithm (TCP_NODELAY) for lower latency

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

// ===================================================================================
// MAIN
// ===================================================================================

func main() {
	flag.Parse()

	// -------------------------------------------------------------------------------
	// CONNECT TO LEADER
	// -------------------------------------------------------------------------------
	//
	// Establishes a persistent TCP connection to the Leader's control plane.
	// This call blocks until the Leader becomes reachable.

	conn := connectLoop(*addr)
	defer conn.Close()

	// -------------------------------------------------------------------------------
	// SERVER → STDOUT PIPE
	// -------------------------------------------------------------------------------
	//
	// Continuously streams all server output directly to stdout.
	// This creates pure telnet-like behavior: everything the server writes
	// becomes immediately visible to the user.

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(os.Stdout, conn)
	}()

	// -------------------------------------------------------------------------------
	// STDIN → SERVER PIPE
	// -------------------------------------------------------------------------------
	//
	// Reads user input line-by-line and forwards commands to the Leader.
	// Supports both interactive commands and bulk SET operations.

	stdin := bufio.NewScanner(os.Stdin)
	writer := bufio.NewWriterSize(conn, 64*1024) // 64 KB buffer

	for {
		if !stdin.Scan() {
			return
		}

		line := strings.TrimSpace(stdin.Text())
		if line == "" {
			continue
		}

		// -----------------------------------------------------------------------
		// TERMINATION COMMANDS
		// -----------------------------------------------------------------------
		//
		// EXIT / QUIT:
		//   - Gracefully notify the server
		//   - Wait for server-side response
		//   - Terminate client

		if strings.EqualFold(line, "EXIT") || strings.EqualFold(line, "QUIT") {
			_, _ = writer.WriteString("QUIT\r\n")
			_ = writer.Flush()
			<-done
			return
		}

		// -----------------------------------------------------------------------
		// BULK SET MODE
		// -----------------------------------------------------------------------
		//
		// Syntax:
		//   <N> SET <message>
		//
		// Example:
		//   1000 SET hello
		//
		// This sends N sequential SET commands:
		//   SET 1 hello_1
		//   SET 2 hello_2
		//   ...
		//
		// Timing is measured to calculate total and average latency.

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

						// Periodic flush to balance throughput vs latency
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

		// -----------------------------------------------------------------------
		// NORMAL COMMAND MODE
		// -----------------------------------------------------------------------
		//
		// Any command not matching the bulk syntax is forwarded as-is
		// to the Leader control plane.

		if _, err := writer.WriteString(line + "\r\n"); err != nil {
			return
		}
		if err := writer.Flush(); err != nil {
			return
		}
	}

}

