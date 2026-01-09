package main

import (
	"bufio"
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
	addr    = flag.String("addr", DEFAULT_ADDR, "Leader TCP address (e.g., localhost:6666)")
	measure = flag.Bool("measure", false, "Measure RTT per command using a separate TCP connection")
)

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

// Drain any initial banner/prompt bytes from the measurement connection,
// so our first RTT measurement isn't polluted by startup output.
func drainMeasureStartup(mconn net.Conn, r *bufio.Reader, maxDur time.Duration) {
	deadline := time.Now().Add(maxDur)
	_ = mconn.SetReadDeadline(deadline)

	// Read lines while available (banner lines end with \n)
	for {
		_, err := r.ReadString('\n')
		if err != nil {
			break
		}
	}

	// Now try to read any remaining bytes (like "tolerex> " without newline)
	_ = mconn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	buf := make([]byte, 4096)
	_, _ = mconn.Read(buf)

	// Clear deadline
	_ = mconn.SetReadDeadline(time.Time{})
}

// Consume the server prompt bytes from the measurement reader if present.
// This is important because the prompt is not newline-terminated.
func consumePrompt(mconn net.Conn, r *bufio.Reader) {
	// Try a few times quickly to catch prompt bytes
	for i := 0; i < 3; i++ {
		_ = mconn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))

		// Peek prompt length
		b, err := r.Peek(len(SERVER_PROMPT))
		if err == nil && string(b) == SERVER_PROMPT {
			_, _ = r.Discard(len(SERVER_PROMPT))
			_ = mconn.SetReadDeadline(time.Time{})
			return
		}

		// Maybe there are leading \r or \n; eat whitespace and retry
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

		// skip empties and banner-ish lines
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

		// This is a real response line (OK / ERROR / NOT_FOUND / value)
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

	fmt.Println("Select test category:")
	for i, c := range categories {
		fmt.Printf("  %d) %s\n", i+1, c)
	}

	var catChoice int
	fmt.Print("Choice: ")
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

	fmt.Printf("\nAvailable files in %s:\n", category)
	for i, f := range files {
		fmt.Printf("  %d) %s\n", i+1, f)
	}

	var fileChoice int
	fmt.Print("Select file: ")
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

func main() {
	flag.Parse()

	// --------------------------------------------------------------------
	// 1) CLI CONNECTION (UNCHANGED TELNET BEHAVIOR)
	// --------------------------------------------------------------------
	cliConn := connectLoop(*addr)
	defer cliConn.Close()

	
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(os.Stdout, cliConn)
	}()

	cliWriter := bufio.NewWriterSize(cliConn, 64*1024)
	stdin := bufio.NewScanner(os.Stdin)

	// --------------------------------------------------------------------
	// 2) MEASURE CONNECTION (ONLY IF --measure)
	// --------------------------------------------------------------------
	var (
		mconn   net.Conn
		mreader *bufio.Reader
		mwriter *bufio.Writer
	)

	var (
		rttCount int64
		rttSum   time.Duration
		rttMin   = time.Hour
		rttMax   time.Duration
	)

	if *measure {
		mconn = connectLoop(*addr)
		defer mconn.Close()
		mreader = bufio.NewReader(mconn)
		mwriter = bufio.NewWriterSize(mconn, 64*1024)

		// Drain banner/prompt so RTT starts clean
		drainMeasureStartup(mconn, mreader, 700*time.Millisecond)
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
		// ----------------------------------------------------------------
		fields := strings.Fields(line)
		if len(fields) == 2 {
			if bulkCount, err := strconv.Atoi(fields[0]); err == nil &&
				strings.EqualFold(fields[1], "FILE") {

				// 1) dosya seçtir
				path, err := selectTestFile()
				if err != nil {
					fmt.Fprintln(os.Stderr, "File selection error:", err)
					continue
				}

				// 2) dosyayı oku (tek satır)
				content, err := readWholeFile(path)
				if err != nil {
					fmt.Fprintln(os.Stderr, "File read error:", err)
					continue
				}

				// 3) base message id al
				fmt.Print("Base message ID: ")
				var baseID int
				fmt.Scanln(&baseID)

				start := time.Now()

				// 4) BULK FILE SET
				for i := 0; i < bulkCount; i++ {
					id := baseID + i
					cmd := fmt.Sprintf("SET %d %s\r\n", id, content)

					if _, err := cliWriter.WriteString(cmd); err != nil {
						return
					}

					if (i+1)%BULK_FLUSH_EVERY == 0 {
						if err := cliWriter.Flush(); err != nil {
							return
						}
					}
				}
				_ = cliWriter.Flush()

				total := time.Since(start)
				avg := total / time.Duration(bulkCount)

				fmt.Fprintf(
					os.Stderr,
					"BULK FILE DONE: %d SET | total=%d ms | avg=%d us | payload=%d bytes\n",
					bulkCount,
					total.Milliseconds(),
					avg.Microseconds(),
					len(content),
				)

				continue
			}
		}

		if strings.EqualFold(line, "FILE") {

			// 1) dosya seçtir
			path, err := selectTestFile()
			if err != nil {
				fmt.Fprintln(os.Stderr, "File selection error:", err)
				continue
			}

			// 2) dosyanın tamamını oku
			content, err := readWholeFile(path)
			if err != nil {
				fmt.Fprintln(os.Stderr, "File read error:", err)
				continue
			}

			// 3) message id al
			fmt.Print("Base message ID: ")
			var msgID int
			fmt.Scanln(&msgID)

			cmd := fmt.Sprintf("SET %d %s\r\n", msgID, content)

			// 4) CLI bağlantısından gönder (normal davranış)
			if _, err := cliWriter.WriteString(cmd); err != nil {
				fmt.Fprintln(os.Stderr, "CLI write error:", err)
				continue
			}
			if err := cliWriter.Flush(); err != nil {
				fmt.Fprintln(os.Stderr, "CLI flush error:", err)
				continue
			}

			// 5) RTT ölçümü (measure açıksa)
			if *measure && mwriter != nil && mreader != nil {

				start := time.Now()

				if _, err := mwriter.WriteString(cmd); err != nil {
					fmt.Fprintln(os.Stderr, "measure write error:", err)
					continue
				}
				if err := mwriter.Flush(); err != nil {
					fmt.Fprintln(os.Stderr, "measure flush error:", err)
					continue
				}

				_, rerr := readSemanticLine(mconn, mreader, 10*time.Second)
				rtt := time.Since(start)

				consumePrompt(mconn, mreader)

				if rerr != nil {
					fmt.Fprintln(os.Stderr, "RTT read error:", rerr)
					continue
				}

				rttCount++
				rttSum += rtt
				if rtt < rttMin {
					rttMin = rtt
				}
				if rtt > rttMax {
					rttMax = rtt
				}

				fmt.Fprintf(os.Stderr, "RTT (file): %d us\n", rtt.Microseconds())
			}

			continue
		}

		// QUIT/EXIT
		if strings.EqualFold(line, "EXIT") || strings.EqualFold(line, "QUIT") {
			_, _ = cliWriter.WriteString("QUIT\r\n")
			_ = cliWriter.Flush()

			if *measure && mwriter != nil {
				_, _ = mwriter.WriteString("QUIT\r\n")
				_ = mwriter.Flush()
			}

			if *measure && rttCount > 0 {
				avg := rttSum / time.Duration(rttCount)
				fmt.Fprintln(os.Stderr, "\n=== RTT SUMMARY ===")
				fmt.Fprintf(os.Stderr, "Samples : %d\n", rttCount)
				fmt.Fprintf(os.Stderr, "Min RTT : %d us\n", rttMin.Microseconds())
				fmt.Fprintf(os.Stderr, "Avg RTT : %d us\n", avg.Microseconds())
				fmt.Fprintf(os.Stderr, "Max RTT : %d us\n", rttMax.Microseconds())
				fmt.Fprintln(os.Stderr, "===================")
			}

			<-done
			return
		}

		// ----------------------------------------------------------------
		// BULK MODE (same as before for CLI)
		// Syntax: <N> SET <message>
		// ----------------------------------------------------------------
		fields = strings.Fields(line)
		if len(fields) >= 3 {
			if bulkCount, err := strconv.Atoi(fields[0]); err == nil {
				if strings.EqualFold(fields[1], "SET") {

					baseMsg := strings.Join(fields[2:], " ")
					start := time.Now()

					for i := 1; i <= bulkCount; i++ {
						cmd := fmt.Sprintf("SET %d %s_%d\r\n", i, baseMsg, i)
						if _, err := cliWriter.WriteString(cmd); err != nil {
							return
						}
						if i%BULK_FLUSH_EVERY == 0 {
							if err := cliWriter.Flush(); err != nil {
								return
							}
						}
					}
					_ = cliWriter.Flush()

					total := time.Since(start)
					avg := total / time.Duration(bulkCount)

					// Print bulk stats to stderr (don’t disturb CLI)
					fmt.Fprintf(os.Stderr, "BULK DONE: %d SET in %d ms (avg %d us)\n",
						bulkCount, total.Milliseconds(), avg.Microseconds())

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
		// ----------------------------------------------------------------
		if *measure && mwriter != nil && mreader != nil && mconn != nil {
			start := time.Now()

			_, err := mwriter.WriteString(line + "\r\n")
			if err != nil {
				fmt.Fprintln(os.Stderr, "measure write error:", err)
				continue
			}
			if err := mwriter.Flush(); err != nil {
				fmt.Fprintln(os.Stderr, "measure flush error:", err)
				continue
			}

			// Read the semantic response line (OK/ERROR/NOT_FOUND/value)
			_, rerr := readSemanticLine(mconn, mreader, 5*time.Second)
			rtt := time.Since(start)

			// Consume the prompt bytes so next command doesn't see old prompt
			consumePrompt(mconn, mreader)

			if rerr != nil {
				fmt.Fprintln(os.Stderr, "RTT timeout/read error:", rerr)
				continue
			}

			rttCount++
			rttSum += rtt
			if rtt < rttMin {
				rttMin = rtt
			}
			if rtt > rttMax {
				rttMax = rtt
			}

			// Print RTT to stderr so prompt/output ordering stays intact
			fmt.Fprintf(os.Stderr, "RTT: %d us\n", rtt.Microseconds())
		}
	}
}
