package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type ClusterRuntime struct {
	IOMode    string `json:"io_mode"`
	NodeCount int    `json:"node_count"`
}

// -------------------------
// INPUT HELPERS
// -------------------------

func askInt(prompt string) int {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print(prompt)
		text, _ := reader.ReadString('\n')
		val, err := strconv.Atoi(strings.TrimSpace(text))
		if err == nil && val > 0 {
			return val
		}
		fmt.Println("Invalid number, try again.")
	}
}

func askChoice(prompt string, allowed ...string) string {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print(prompt)
		text := strings.TrimSpace(readLine(reader))
		for _, a := range allowed {
			if text == a {
				return text
			}
		}
		fmt.Println("Invalid choice. Allowed:", strings.Join(allowed, ", "))
	}
}

func readLine(r *bufio.Reader) string {
	s, _ := r.ReadString('\n')
	return strings.TrimSpace(s)
}

func waitEnter(msg string) {
	fmt.Println(msg)
	bufio.NewReader(os.Stdin).ReadString('\n')
}

// -------------------------
// TERMINAL SPAWNER
// -------------------------

func spawnTerminal(title, command string) {
	ps := fmt.Sprintf(
		"$host.UI.RawUI.WindowTitle='%s'; %s",
		title, command,
	)

	cmd := exec.Command(
		"cmd",
		"/c",
		"start",
		"powershell",
		"-NoExit",
		"-Command",
		ps,
	)

	_ = cmd.Start()
}

// -------------------------
// MAIN
// -------------------------

func main() {

	fmt.Println("=== TOLEREX LOCAL CLUSTER LAUNCHER ===")

	memberCount := askInt("Number of MEMBERS: ")
	msgCount := askInt("Messages to send (client count): ")
	ioMode := askChoice("IO mode [buffered | unbuffered]: ", "buffered", "unbuffered")

	const (
		baseDir   = "D:\\Tolerex"
		startPort = 5556
	)

	fmt.Println("\n--- Configuration ---")
	fmt.Println("Client count  : 1")
	fmt.Println("Member count  :", memberCount)
	fmt.Println("Messages      :", msgCount)
	fmt.Println("IO Mode       :", ioMode)
	fmt.Println("---------------------\n")

	runtime := ClusterRuntime{
		IOMode:    ioMode,
		NodeCount: memberCount,
	}

	_ = os.MkdirAll("internal/data", 0755)
	b, _ := json.MarshalIndent(runtime, "", "  ")
	_ = os.WriteFile("internal/data/cluster_runtime.json", b, 0644)

	// -------------------------
	// START LEADER
	// -------------------------
	fmt.Println("Starting Leader...")
	spawnTerminal(
		"LEADER",
		fmt.Sprintf("cd %s; go run ./cmd/leader/main.go", baseDir),
	)

	waitEnter("Press ENTER once the Leader is fully ready...")

	// -------------------------
	// START MEMBERS
	// -------------------------
	fmt.Println("Starting Members...")
	for i := 0; i < memberCount; i++ {
		port := startPort + i
		spawnTerminal(
			fmt.Sprintf("MEMBER-%d", port),
			fmt.Sprintf(
				"cd %s; go run ./cmd/member/main.go -port=%d -io=%s",
				baseDir, port, ioMode,
			),
		)
	}

	waitEnter("Press ENTER once all Members are ready...")

	// -------------------------
	// START SINGLE CLIENT
	// -------------------------
	fmt.Println("Starting Client...")
	spawnTerminal(
		"CLIENT",
		fmt.Sprintf(
			"cd %s; go run ./client/test_client.go -count %d",
			baseDir, msgCount,
		),
	)

	fmt.Println("\nCluster successfully started.")
}
