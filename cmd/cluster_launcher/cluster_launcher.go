package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ===================================================================================
// INPUT HELPERS
// ===================================================================================

func askInt(prompt string) int {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print(prompt)
		text, _ := reader.ReadString('\n')
		value, err := strconv.Atoi(strings.TrimSpace(text))

		if err == nil && value > 0 {
			return value
		}
		fmt.Println("Invalid number, please enter a positive integer.")
	}
}

func askChoice(prompt string, allowed ...string) string {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print(prompt)
		input := strings.TrimSpace(readLine(reader))

		for _, option := range allowed {
			if input == option {
				return input
			}
		}

		fmt.Println("Invalid choice. Allowed values:", strings.Join(allowed, ", "))
	}
}

func readLine(r *bufio.Reader) string {
	line, _ := r.ReadString('\n')
	return strings.TrimSpace(line)
}

func waitEnter(message string) {
	fmt.Println(message)
	bufio.NewReader(os.Stdin).ReadString('\n')
}

// ===================================================================================
// TERMINAL PROCESS SPAWNER (WINDOWS)
// ===================================================================================

func spawnTerminal(title, command string) {
	psCommand := fmt.Sprintf(
		"$host.UI.RawUI.WindowTitle='%s'; %s",
		title,
		command,
	)

	cmd := exec.Command(
		"cmd",
		"/c",
		"start",
		"powershell",
		"-NoExit",
		"-Command",
		psCommand,
	)

	_ = cmd.Start()
}

// ===================================================================================
// MAIN
// ===================================================================================

func main() {
	fmt.Println("=== TOLEREX LOCAL CLUSTER LAUNCHER ===")

	clientCount := askInt("Number of CLIENTS: ")
	memberCount := askInt("Number of MEMBERS: ")
	ioMode := askChoice(
		"IO mode [buffered | unbuffered]: ",
		"buffered",
		"unbuffered",
	)

	measureMode := askChoice(
		"Enable RTT measurement for clients? [yes | no]: ",
		"yes",
		"no",
	)

	// CSV Option for clients
	csvMode := "no"
	if measureMode == "yes" {
		csvMode = askChoice(
			"Enable CSV output for RTT? [yes | no]: ",
			"yes",
			"no",
		)
	}

	const (
		baseDir         = "D:\\Tolerex"
		startGrpcPort   = 5556
		startMetricPort = 9092
		leaderTCPAddr   = "localhost:6666"
	)

	// Ensure measured directory exists
	measuredDir := filepath.Join(baseDir, "measured")
	_ = os.MkdirAll(measuredDir, 0755)

	fmt.Println("\n--- Configuration Summary ---")
	fmt.Println("Client count  :", clientCount)
	fmt.Println("Member count  :", memberCount)
	fmt.Println("IO Mode       :", ioMode)
	fmt.Println("RTT Measure   :", measureMode)
	fmt.Println("CSV Output    :", csvMode)
	fmt.Println("TCP Addr      :", leaderTCPAddr)
	fmt.Println("-----------------------------")

	// -------------------------------------------------------------------------------
	// Start Leader node (LOCAL)
	// -------------------------------------------------------------------------------
	fmt.Println("Starting Leader...")

	spawnTerminal(
		"LEADER",
		fmt.Sprintf(
			"cd %s; "+
				"$env:LEADER_GRPC_PORT='5555'; "+
				"$env:LEADER_METRICS_PORT='9090'; "+
				"go run ./cmd/leader/main.go",
			baseDir,
		),
	)

	waitEnter("Press ENTER once the Leader is fully ready...")

	// -------------------------------------------------------------------------------
	// Start Member nodes (LOCAL)
	// -------------------------------------------------------------------------------
	fmt.Println("Starting Members...")

	for i := 0; i < memberCount; i++ {
		grpcPort := startGrpcPort + i
		metricsPort := startMetricPort + i

		spawnTerminal(
			fmt.Sprintf("MEMBER-%d", grpcPort),
			fmt.Sprintf(
				"cd %s; "+
					"$env:LEADER_ADDR='localhost:5555'; "+
					"$env:MEMBER_ADDR='localhost:%d'; "+
					"go run ./cmd/member/main.go -port=%d -metrics=%d -io=%s",
				baseDir,
				grpcPort,
				grpcPort,
				metricsPort,
				ioMode,
			),
		)
	}

	waitEnter("Press ENTER once all Members are ready...")

	// -------------------------------------------------------------------------------
	// Start Client (LOCAL) - UPDATED for compatibility
	// -------------------------------------------------------------------------------
	fmt.Println("Starting Clients...")

	for i := 0; i < clientCount; i++ {
		clientID := fmt.Sprintf("client-%d", i+1)
		offset := i * 100_000 // Each client gets 100k distinct ID range

		title := fmt.Sprintf("CLIENT-%d", i+1)

		// Base command with address
		// Added -id and -offset flags here to fix compatibility/collision issues
		clientCmd := fmt.Sprintf("go run ./client/test_client.go -addr %s -id %s -offset %d", leaderTCPAddr, clientID, offset)

		if measureMode == "yes" {
			clientCmd += " -measure"
			title += " [MEASURE]"
		}
		if csvMode == "yes" {
			clientCmd += " -csv"
			title += " [CSV]"

			// Redirect CSV output to file
			csvFile := fmt.Sprintf(".\\measured\\%s.csv", clientID)
			clientCmd += fmt.Sprintf(" > %s", csvFile)
		}

		spawnTerminal(
			title,
			fmt.Sprintf(
				"cd %s; %s",
				baseDir,
				clientCmd,
			),
		)

		time.Sleep(200 * time.Millisecond)
	}

	fmt.Println("\nCluster successfully started.")
	if csvMode == "yes" {
		fmt.Println("CSV files are under:", measuredDir)
	}
}
