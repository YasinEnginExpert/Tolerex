package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
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

	memberCount := askInt("Number of MEMBERS: ")
	ioMode := askChoice(
		"IO mode [buffered | unbuffered]: ",
		"buffered",
		"unbuffered",
	)

	const (
		baseDir         = "D:\\Tolerex"
		startGrpcPort   = 5556
		startMetricPort = 9092
	)

	fmt.Println("\n--- Configuration Summary ---")
	fmt.Println("Client count  : 1")
	fmt.Println("Member count  :", memberCount)
	fmt.Println("IO Mode       :", ioMode)
	fmt.Println("-----------------------------")

	// -------------------------------------------------------------------------------
	// Start Leader node (LOCAL)
	// -------------------------------------------------------------------------------

	fmt.Println("Starting Leader...")

	// Local senaryoda leader zaten localhost:5555 dinliyor.
	// İstersen Leader'a da env basabilirsin ama şart değil.
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

		// ✅ En kritik fix:
		// LEADER_ADDR kesinlikle localhost:5555 olmalı (Docker'daki leader:5555 değil)
		// MEMBER_ADDR da leader'a kendini doğru advertise etmeli
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
	// Start Client (LOCAL)
	// -------------------------------------------------------------------------------

	fmt.Println("Starting Client...")

	spawnTerminal(
		"CLIENT",
		fmt.Sprintf(
			"cd %s; "+
				"$env:LEADER_ADDR='localhost:5555'; "+
				"go run ./client/test_client.go",
			baseDir,
		),
	)

	fmt.Println("\nCluster successfully started.")
}
