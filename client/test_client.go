package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	DEFAULT_ADDR     = "localhost:6666"
	DIAL_TIMEOUT     = 3 * time.Second
	RETRY_DELAY      = 2 * time.Second
	BULK_FLUSH_EVERY = 1000

	// Leader prompt (server prints this WITHOUT newline)
	SERVER_PROMPT = "tolerex> "
)

var (
	addr       = flag.String("addr", DEFAULT_ADDR, "Leader TCP address (e.g., localhost:6666)")
	measure    = flag.Bool("measure", false, "Measure RTT per command using a separate TCP connection")
	csv        = flag.Bool("csv", false, "Output measurements as CSV to stdout (Timestamp,Operation,Count,Bytes,RTT_us). All logs/UI go to stderr.")
	flushEvery = flag.Int("flush", 1000, "Flush buffer after every N operations (default 1000). Set to 1 for unbuffered.")
	offset     = flag.Int("offset", 0, "Starting ID offset for BULK operations (default 0)")
	clientID   = flag.String("id", "client", "Client Identifier for CSV output (default 'client')")
)

func connectLoop(address string) net.Conn {
	for {
		dialer := net.Dialer{Timeout: DIAL_TIMEOUT}
		conn, err := dialer.Dial("tcp", address)
		if err == nil {
			if tcp, ok := conn.(*net.TCPConn); ok {
				_ = tcp.SetNoDelay(true) // TCP_NODELAY
			}
			return conn
		}
		time.Sleep(RETRY_DELAY)
	}
}

// Wait for the leader prompt explicitly, to avoid startup sync issues.
// This consumes everything up to and including "tolerex> ".
func drainMeasureStartup(mconn net.Conn, r *bufio.Reader, maxDur time.Duration) error {
	deadline := time.Now().Add(maxDur)
	_ = mconn.SetReadDeadline(deadline)
	defer mconn.SetReadDeadline(time.Time{})

	p := []byte(SERVER_PROMPT)
	window := make([]byte, 0, len(p))

	for {
		b, err := r.ReadByte()
		if err != nil {
			return fmt.Errorf("startup sync failed (waiting for prompt): %w", err)
		}

		if len(window) < len(p) {
			window = append(window, b)
		} else {
			copy(window, window[1:])
			window[len(p)-1] = b
		}

		if len(window) == len(p) && bytes.Equal(window, p) {
			return nil // prompt consumed
		}
	}
}

// Consume the server prompt bytes from the measurement reader if present.
// This is important because the prompt is not newline-terminated.
func consumePrompt(mconn net.Conn, r *bufio.Reader) {
	for i := 0; i < 3; i++ {
		_ = mconn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))

		b, err := r.Peek(len(SERVER_PROMPT))
		if err == nil && string(b) == SERVER_PROMPT {
			_, _ = r.Discard(len(SERVER_PROMPT))
			_ = mconn.SetReadDeadline(time.Time{})
			return
		}

		one, err2 := r.Peek(1)
		if err2 == nil && (one[0] == '\r' || one[0] == '\n' || one[0] == ' ') {
			_, _ = r.Discard(1)
			continue
		}

		_ = mconn.SetReadDeadline(time.Time{})
		return
	}
	_ = mconn.SetReadDeadline(time.Time{})
}

// Skip banner/empty lines; return first "semantic" response line.
func readSemanticLine(mconn net.Conn, r *bufio.Reader, timeout time.Duration) (string, error) {
	_ = mconn.SetReadDeadline(time.Now().Add(timeout))
	defer mconn.SetReadDeadline(time.Time{})

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return "", err
		}
		t := strings.TrimSpace(line)

		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "----") {
			continue
		}
		if strings.Contains(t, "TOLEREX") {
			continue
		}
		if t == "Commands:" || t == "Examples:" {
			continue
		}
		if strings.HasPrefix(t, "SET <") || strings.HasPrefix(t, "<N> ") || strings.HasPrefix(t, "GET <") {
			continue
		}
		if strings.HasPrefix(t, "HELP") || strings.HasPrefix(t, "QUIT") {
			continue
		}

		return t, nil
	}
}

func listDirs(base string) ([]string, error) {
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, err
	}

	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	return dirs, nil
}

func listTxtFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".txt") {
			files = append(files, e.Name())
		}
	}
	return files, nil
}

func selectTestFile() (string, error) {
	const baseDir = "client/test"

	categories, err := listDirs(baseDir)
	if err != nil || len(categories) == 0 {
		return "", fmt.Errorf("no test categories found")
	}

	fmt.Fprintln(os.Stderr, "Select test category:")
	for i, c := range categories {
		fmt.Fprintf(os.Stderr, "  %d) %s\n", i+1, c)
	}

	var catChoice int
	fmt.Fprint(os.Stderr, "Choice: ")
	fmt.Scanln(&catChoice)

	if catChoice < 1 || catChoice > len(categories) {
		return "", fmt.Errorf("invalid category choice")
	}

	category := categories[catChoice-1]
	categoryPath := filepath.Join(baseDir, category)

	files, err := listTxtFiles(categoryPath)
	if err != nil || len(files) == 0 {
		return "", fmt.Errorf("no txt files found in %s", category)
	}

	fmt.Fprintf(os.Stderr, "\nAvailable files in %s:\n", category)
	for i, f := range files {
		fmt.Fprintf(os.Stderr, "  %d) %s\n", i+1, f)
	}

	var fileChoice int
	fmt.Fprint(os.Stderr, "Select file: ")
	fmt.Scanln(&fileChoice)

	if fileChoice < 1 || fileChoice > len(files) {
		return "", fmt.Errorf("invalid file choice")
	}

	return filepath.Join(categoryPath, files[fileChoice-1]), nil
}

func readWholeFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	content := string(data)
	content = strings.ReplaceAll(content, "\r\n", " ")
	content = strings.ReplaceAll(content, "\n", " ")
	content = strings.Join(strings.Fields(content), " ")
	return content, nil
}

// -----------------------------------------------------------------------------
// CSV + HUMAN OUTPUT HELPERS
// -----------------------------------------------------------------------------

type rttStats struct {
	count int64
	sum   time.Duration
	min   time.Duration
	max   time.Duration
}

func (s *rttStats) add(rtt time.Duration) {
	s.count++
	s.sum += rtt
	if rtt < s.min {
		s.min = rtt
	}
	if rtt > s.max {
		s.max = rtt
	}
}

func opFromLine(line string) string {
	f := strings.Fields(line)
	if len(f) == 0 {
		return "CMD"
	}
	return strings.ToUpper(f[0])
}

func emitStat(csvEnabled bool, csvOut *bufio.Writer, op string, count int, bytesSent int64, rtt time.Duration) {
	if csvEnabled && csvOut != nil {
		ts := time.Now().Format(time.RFC3339Nano)
		fmt.Fprintf(csvOut, "%s,%s,%s,%d,%d,%d\n", ts, *clientID, op, count, bytesSent, rtt.Microseconds())
		_ = csvOut.Flush()
		return
	}
	fmt.Fprintf(os.Stderr, "RTT (%s): %d us | count=%d | bytes=%d\n", op, rtt.Microseconds(), count, bytesSent)
}

func main() {
	flag.Parse()

	// CSV mode requires measure, yoksa anlamı kalmıyor (RTT yok).
	if *csv && !*measure {
		fmt.Fprintln(os.Stderr, "ERROR: -csv requires -measure (CSV needs RTT). Run with: -measure -csv")
		os.Exit(2)
	}

	// Scanner buffer
	stdin := bufio.NewScanner(os.Stdin)
	stdin.Buffer(make([]byte, 1024), 10*1024*1024)

	// CSV output (stdout only)
	var csvOut *bufio.Writer
	if *csv {
		csvOut = bufio.NewWriterSize(os.Stdout, 64*1024)
		// Print CSV Header
		fmt.Fprintln(csvOut, "Timestamp,ClientID,Operation,Count,Bytes,RTT_us")
		_ = csvOut.Flush()
	}

	// --------------------------------------------------------------------
	// 1) CLI CONNECTION (interactive)
	// --------------------------------------------------------------------
	cliConn := connectLoop(*addr)
	defer cliConn.Close()

	// stdout temiz kalsın: CSV stdout'ta; CLI output stderr'e gitsin
	cliOut := os.Stdout
	if *csv {
		cliOut = os.Stderr
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(cliOut, cliConn)
	}()

	cliWriter := bufio.NewWriterSize(cliConn, 64*1024)

	// --------------------------------------------------------------------
	// 2) MEASURE CONNECTION (ONLY IF --measure)
	// --------------------------------------------------------------------
	var (
		mconn   net.Conn
		mreader *bufio.Reader
		mwriter *bufio.Writer
	)

	stats := rttStats{min: time.Hour}

	if *measure {
		mconn = connectLoop(*addr)
		defer mconn.Close()
		mreader = bufio.NewReader(mconn)
		mwriter = bufio.NewWriterSize(mconn, 64*1024)

		if err := drainMeasureStartup(mconn, mreader, 5*time.Second); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
	}

	// --------------------------------------------------------------------
	// 3) INPUT LOOP
	// --------------------------------------------------------------------
	for {
		if !stdin.Scan() {
			return
		}
		line := strings.TrimSpace(stdin.Text())
		if line == "" {
			continue
		}

		// ----------------------------------------------------------------
		// BULK FILE MODE
		// Syntax: <N> FILE
		// CSV: tek satır -> BULK_FILE,Count,Bytes,RTT_us
		// ----------------------------------------------------------------
		fields := strings.Fields(line)
		if len(fields) == 2 {
			if bulkCount, err := strconv.Atoi(fields[0]); err == nil && strings.EqualFold(fields[1], "FILE") {

				path, err := selectTestFile()
				if err != nil {
					fmt.Fprintln(os.Stderr, "File selection error:", err)
					continue
				}

				content, err := readWholeFile(path)
				if err != nil {
					fmt.Fprintln(os.Stderr, "File read error:", err)
					continue
				}

				fmt.Fprint(os.Stderr, "Base message ID: ")
				var baseID int
				fmt.Scanln(&baseID)

				startBulk := time.Now()
				var bytesSent int64

				// RTT sample: first + last (sonra average alacağız)
				var sampleRTTs []time.Duration

				for i := 0; i < bulkCount; i++ {
					id := baseID + i
					cmd := fmt.Sprintf("SET %d %s\r\n", id, content)
					bytesSent += int64(len(cmd))

					if _, err := cliWriter.WriteString(cmd); err != nil {
						return
					}

					// measure only first & last
					if *measure && mwriter != nil && mreader != nil && (i == 0 || i == bulkCount-1) {
						startRTT := time.Now()

						if _, err := mwriter.WriteString(cmd); err != nil {
							fmt.Fprintln(os.Stderr, "measure write error:", err)
							continue
						}
						if err := mwriter.Flush(); err != nil {
							fmt.Fprintln(os.Stderr, "measure flush error:", err)
							continue
						}

						_, rerr := readSemanticLine(mconn, mreader, 10*time.Second)
						rtt := time.Since(startRTT)
						consumePrompt(mconn, mreader)

						if rerr == nil {
							stats.add(rtt)
							sampleRTTs = append(sampleRTTs, rtt)
						} else {
							fmt.Fprintln(os.Stderr, "RTT read error:", rerr)
						}
					}

					if (i+1)%BULK_FLUSH_EVERY == 0 {
						if err := cliWriter.Flush(); err != nil {
							return
						}
					}
				}
				_ = cliWriter.Flush()

				totalBulk := time.Since(startBulk)
				avgBulk := totalBulk / time.Duration(bulkCount)

				// Aggregate RTT: sample avg (first+last)/2 or first/only
				aggRTT := time.Duration(0)
				if len(sampleRTTs) == 1 {
					aggRTT = sampleRTTs[0]
				} else if len(sampleRTTs) >= 2 {
					aggRTT = (sampleRTTs[0] + sampleRTTs[len(sampleRTTs)-1]) / 2
				}

				// CSV: tek satır (plan formatına uygun)
				if *measure && *csv && aggRTT > 0 {
					emitStat(true, csvOut, "BULK_FILE", bulkCount, bytesSent, aggRTT)
				}

				// İnsan okunabilir özet (stderr)
				fmt.Fprintf(os.Stderr,
					"BULK FILE DONE: %d SET | total=%d ms | avg=%d us | bytes=%d | payload=%d bytes | sampleRTT=%d us\n",
					bulkCount,
					totalBulk.Milliseconds(),
					avgBulk.Microseconds(),
					bytesSent,
					len(content),
					aggRTT.Microseconds(),
				)

				continue
			}
		}

		// ----------------------------------------------------------------
		// SINGLE FILE MODE
		// Syntax: FILE
		// CSV: FILE_SET,1,Bytes,RTT
		// ----------------------------------------------------------------
		if strings.EqualFold(line, "FILE") {

			path, err := selectTestFile()
			if err != nil {
				fmt.Fprintln(os.Stderr, "File selection error:", err)
				continue
			}

			content, err := readWholeFile(path)
			if err != nil {
				fmt.Fprintln(os.Stderr, "File read error:", err)
				continue
			}

			fmt.Fprint(os.Stderr, "Base message ID: ")
			var msgID int
			fmt.Scanln(&msgID)

			cmd := fmt.Sprintf("SET %d %s\r\n", msgID, content)
			bytesSent := int64(len(cmd))

			if _, err := cliWriter.WriteString(cmd); err != nil {
				fmt.Fprintln(os.Stderr, "CLI write error:", err)
				continue
			}
			if err := cliWriter.Flush(); err != nil {
				fmt.Fprintln(os.Stderr, "CLI flush error:", err)
				continue
			}

			if *measure && mwriter != nil && mreader != nil {
				startRTT := time.Now()

				if _, err := mwriter.WriteString(cmd); err != nil {
					fmt.Fprintln(os.Stderr, "measure write error:", err)
					continue
				}
				if err := mwriter.Flush(); err != nil {
					fmt.Fprintln(os.Stderr, "measure flush error:", err)
					continue
				}

				_, rerr := readSemanticLine(mconn, mreader, 10*time.Second)
				rtt := time.Since(startRTT)
				consumePrompt(mconn, mreader)

				if rerr != nil {
					fmt.Fprintln(os.Stderr, "RTT read error:", rerr)
					continue
				}

				stats.add(rtt)
				emitStat(*csv, csvOut, "FILE_SET", 1, bytesSent, rtt)
			}

			continue
		}

		// ----------------------------------------------------------------
		// QUIT/EXIT
		// ----------------------------------------------------------------
		if strings.EqualFold(line, "EXIT") || strings.EqualFold(line, "QUIT") {
			_, _ = cliWriter.WriteString("QUIT\r\n")
			_ = cliWriter.Flush()

			if *measure && mwriter != nil {
				_, _ = mwriter.WriteString("QUIT\r\n")
				_ = mwriter.Flush()
			}

			if *measure && stats.count > 0 {
				avg := stats.sum / time.Duration(stats.count)
				fmt.Fprintln(os.Stderr, "\n=== RTT SUMMARY ===")
				fmt.Fprintf(os.Stderr, "Samples : %d\n", stats.count)
				fmt.Fprintf(os.Stderr, "Min RTT : %d us\n", stats.min.Microseconds())
				fmt.Fprintf(os.Stderr, "Avg RTT : %d us\n", avg.Microseconds())
				fmt.Fprintf(os.Stderr, "Max RTT : %d us\n", stats.max.Microseconds())
				fmt.Fprintln(os.Stderr, "===================")
			}

			<-done
			return
		}

		// ----------------------------------------------------------------
		// BULK SET MODE (CLI only + OPTIONAL RTT sample)
		// Syntax: <N> SET <message>
		// CSV: BULK_SET,Count,Bytes,RTT(sample avg)
		// ----------------------------------------------------------------
		fields = strings.Fields(line)
		if len(fields) >= 3 {
			if bulkCount, err := strconv.Atoi(fields[0]); err == nil {
				if strings.EqualFold(fields[1], "SET") {

					baseMsg := strings.Join(fields[2:], " ")
					startBulk := time.Now()

					var bytesSent int64
					var sampleRTTs []time.Duration

					for i := 1; i <= bulkCount; i++ {
						// Use offset to prevent collisions between multiple clients
						actualID := *offset + i
						cmd := fmt.Sprintf("SET %d %s_%d\r\n", actualID, baseMsg, actualID)
						bytesSent += int64(len(cmd))

						if _, err := cliWriter.WriteString(cmd); err != nil {
							return
						}

						// measure only first & last (same behavior as BULK FILE)
						if *measure && mwriter != nil && mreader != nil && (i == 1 || i == bulkCount) {
							startRTT := time.Now()

							if _, err := mwriter.WriteString(cmd); err != nil {
								fmt.Fprintln(os.Stderr, "measure write error:", err)
								continue
							}
							if err := mwriter.Flush(); err != nil {
								fmt.Fprintln(os.Stderr, "measure flush error:", err)
								continue
							}

							_, rerr := readSemanticLine(mconn, mreader, 10*time.Second)
							rtt := time.Since(startRTT)
							consumePrompt(mconn, mreader)

							if rerr == nil {
								stats.add(rtt)
								sampleRTTs = append(sampleRTTs, rtt)
							} else {
								fmt.Fprintln(os.Stderr, "RTT read error:", rerr)
							}
						}

						if i%BULK_FLUSH_EVERY == 0 {
							if err := cliWriter.Flush(); err != nil {
								return
							}
						}
					}
					_ = cliWriter.Flush()

					totalBulk := time.Since(startBulk)
					avgBulk := totalBulk / time.Duration(bulkCount)

					aggRTT := time.Duration(0)
					if len(sampleRTTs) == 1 {
						aggRTT = sampleRTTs[0]
					} else if len(sampleRTTs) >= 2 {
						aggRTT = (sampleRTTs[0] + sampleRTTs[len(sampleRTTs)-1]) / 2
					}

					if *measure && *csv && aggRTT > 0 {
						emitStat(true, csvOut, "BULK_SET", bulkCount, bytesSent, aggRTT)
					}

					fmt.Fprintf(os.Stderr, "BULK DONE: %d SET in %d ms (avg %d us) | bytes=%d | sampleRTT=%d us\n",
						bulkCount, totalBulk.Milliseconds(), avgBulk.Microseconds(), bytesSent, aggRTT.Microseconds())

					continue
				}
			}
		}

		// ----------------------------------------------------------------
		// NORMAL COMMAND (CLI unchanged)
		// ----------------------------------------------------------------
		_, err := cliWriter.WriteString(line + "\r\n")
		if err != nil {
			return
		}
		if err := cliWriter.Flush(); err != nil {
			return
		}

		// ----------------------------------------------------------------
		// MEASURE RTT (separate connection)
		// CSV: OP,1,Bytes,RTT
		// ----------------------------------------------------------------
		if *measure && mwriter != nil && mreader != nil && mconn != nil {
			op := opFromLine(line)
			// line + "\r\n" gönderiyoruz => +2 byte varsayımı
			cmdBytes := int64(len(line) + 2)

			startRTT := time.Now()

			_, err := mwriter.WriteString(line + "\r\n")
			if err != nil {
				fmt.Fprintln(os.Stderr, "measure write error:", err)
				continue
			}
			if err := mwriter.Flush(); err != nil {
				fmt.Fprintln(os.Stderr, "measure flush error:", err)
				continue
			}

			_, rerr := readSemanticLine(mconn, mreader, 5*time.Second)
			rtt := time.Since(startRTT)
			consumePrompt(mconn, mreader)

			if rerr != nil {
				fmt.Fprintln(os.Stderr, "RTT timeout/read error:", rerr)
				continue
			}

			stats.add(rtt)
			emitStat(*csv, csvOut, op, 1, cmdBytes, rtt)
		}
	}
}
