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
// TOLEREX – LOCAL CLUSTER LAUNCHER
// ===================================================================================
//
// This program is a local orchestration utility for the TOLEREX distributed system.
// Its purpose is to bootstrap a full local cluster environment consisting of:
//
//   - 1 Leader node
//   - N Member nodes
//   - 1 Client process
//
// Each component is launched in its own dedicated terminal window to provide
// operational visibility and isolated logging during development and testing.
//
// IMPORTANT:
// - This launcher is NOT part of the distributed system itself.
// - It performs no networking, coordination, or health checking.
// - It exists purely as a developer convenience tool.
//
// ===================================================================================

// ===================================================================================
// INPUT HELPERS
// ===================================================================================

// askInt prompts the user for a positive integer value.
// It blocks until a valid (>0) integer is provided.
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

// askChoice prompts the user to select one value from a predefined set.
// It enforces strict validation to prevent invalid configuration states.
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

// readLine reads a single line from stdin and trims whitespace.
func readLine(r *bufio.Reader) string {
	line, _ := r.ReadString('\n')
	return strings.TrimSpace(line)
}

// waitEnter pauses execution until the user presses ENTER.
// This is used as a manual synchronization barrier between startup phases.
func waitEnter(message string) {
	fmt.Println(message)
	bufio.NewReader(os.Stdin).ReadString('\n')
}

// ===================================================================================
// TERMINAL PROCESS SPAWNER (WINDOWS)
// ===================================================================================

// spawnTerminal launches a new PowerShell window and executes the provided command.
// Each process runs in its own terminal for clear separation of logs and lifecycle.
func spawnTerminal(title, command string) {
	// PowerShell command:
	// - Set window title for easy identification
	// - Execute the given command
	psCommand := fmt.Sprintf(
		"$host.UI.RawUI.WindowTitle='%s'; %s",
		title,
		command,
	)

	// Windows-specific terminal spawning
	cmd := exec.Command(
		"cmd",
		"/c",
		"start",
		"powershell",
		"-NoExit",
		"-Command",
		psCommand,
	)

	// Start process asynchronously
	_ = cmd.Start()
}

// ===================================================================================
// MAIN
// ===================================================================================

func main() {

	fmt.Println("=== TOLEREX LOCAL CLUSTER LAUNCHER ===")

	// -------------------------------------------------------------------------------
	// Runtime configuration input
	// -------------------------------------------------------------------------------

	memberCount := askInt("Number of MEMBERS: ")
	ioMode := askChoice(
		"IO mode [buffered | unbuffered]: ",
		"buffered",
		"unbuffered",
	)

	// Base directory and port allocation strategy
	const (
		baseDir   = "D:\\Tolerex"
		startPort = 5556
	)

	// -------------------------------------------------------------------------------
	// Configuration summary
	// -------------------------------------------------------------------------------

	fmt.Println("\n--- Configuration Summary ---")
	fmt.Println("Client count  : 1")
	fmt.Println("Member count  :", memberCount)
	fmt.Println("IO Mode       :", ioMode)
	fmt.Println("-----------------------------")

	// -------------------------------------------------------------------------------
	// Start Leader node
	// -------------------------------------------------------------------------------

	fmt.Println("Starting Leader...")

	spawnTerminal(
		"LEADER",
		fmt.Sprintf("cd %s; go run ./cmd/leader/main.go", baseDir),
	)

	// Manual readiness confirmation
	waitEnter("Press ENTER once the Leader is fully ready...")

	// -------------------------------------------------------------------------------
	// Start Member nodes
	// -------------------------------------------------------------------------------

	fmt.Println("Starting Members...")

	for i := 0; i < memberCount; i++ {
		port := startPort + i

		spawnTerminal(
			fmt.Sprintf("MEMBER-%d", port),
			fmt.Sprintf(
				"cd %s; go run ./cmd/member/main.go -port=%d -io=%s",
				baseDir,
				port,
				ioMode,
			),
		)
	}

	// Manual readiness confirmation
	waitEnter("Press ENTER once all Members are ready...")

	// -------------------------------------------------------------------------------
	// Start Client
	// -------------------------------------------------------------------------------

	fmt.Println("Starting Client...")

	spawnTerminal(
		"CLIENT",
		fmt.Sprintf("cd %s; go run ./client/test_client.go", baseDir),
	)

	fmt.Println("\nCluster successfully started.")
}
