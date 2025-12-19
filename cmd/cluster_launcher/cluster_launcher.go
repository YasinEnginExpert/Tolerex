// ===================================================================================
// TOLEREX – LOCAL PROCESS ORCHESTRATOR (HOW IT WORKS EMBEDDED)
// ===================================================================================
//
// PURPOSE
// -------
// This program is a local orchestration tool used to simulate a distributed
// Tolerex cluster on a single Windows machine.
//
// It programmatically opens multiple PowerShell terminals and starts:
//
//   1) One Leader node
//   2) N Member nodes (each on a different port)
//   3) N Client instances
//
// The goal is NOT production deployment, but:
// - Development
// - Manual testing
// - Demonstration of distributed behavior
//
// HOW IT WORKS (HIGH LEVEL)
// -------------------------
// 1) Parse command-line arguments (number of nodes)
// 2) Spawn a terminal for the Leader process
// 3) Wait for user confirmation (Leader must be ready)
// 4) Spawn terminals for Clients
// 5) Spawn terminals for Members with incremental ports
//
// Each process runs in its OWN terminal window so logs and output
// are visually isolated.
//
// ===================================================================================

package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
)

// ===================================================================================
// STEP 1 – SYNCHRONIZATION BARRIER
// ===================================================================================
//
// WHY THIS EXISTS:
// ----------------
// The Leader must be fully started BEFORE Members and Clients connect to it.
// Instead of complex health checks, we use a simple manual barrier.
//
// HOW IT WORKS:
// -------------
// - Print a message
// - Block execution until user presses ENTER
// - Execution continues only after confirmation
//
func waitEnter(msg string) {
	fmt.Println(msg)
	bufio.NewReader(os.Stdin).ReadString('\n')
}

// ===================================================================================
// STEP 2 – TERMINAL SPAWNER
// ===================================================================================
//
// WHAT THIS FUNCTION DOES:
// ------------------------
// Opens a NEW PowerShell window and executes a command inside it.
//
// HOW IT WORKS INTERNALLY:
// ------------------------
// - Uses cmd.exe because it can spawn detached windows
// - Uses "start" to open a new terminal window
// - Uses PowerShell to:
//     * Set window title
//     * Fix window size
//     * Execute the given Go command
//     * Keep the terminal open (-NoExit)
//
// WHY NOT exec.Command("go", "run", ...):
// --------------------------------------
// Because we WANT separate terminals, not background processes.
//
func newTerminal(title, command string) {

	// --- PowerShell command script ---
	// 1) Set window title
	// 2) Fix window size and buffer size
	// 3) Execute the provided command
	psCmd := fmt.Sprintf(
		"$host.UI.RawUI.WindowTitle='%s'; "+
			"$size = New-Object Management.Automation.Host.Size(20,20); "+
			"$host.UI.RawUI.WindowSize = $size; "+
			"$host.UI.RawUI.BufferSize = $size; "+
			"%s",
		title, command,
	)

	// --- Windows process invocation ---
	// cmd /c start powershell -NoExit -Command <psCmd>
	cmd := exec.Command(
		"cmd",
		"/c",
		"start",
		"powershell",
		"-NoExit",
		"-Command",
		psCmd,
	)

	// --- Fire-and-forget ---
	// We do NOT wait for the process; each terminal lives independently
	err := cmd.Start()
	if err != nil {
		fmt.Println("Failed to open terminal:", err)
	}
}

// ===================================================================================
// STEP 3 – MAIN ORCHESTRATION LOGIC
// ===================================================================================
func main() {

	// =========================================================================
	// STEP 3.1 – PARSE RUNTIME PARAMETERS
	// =========================================================================
	//
	// -n defines:
	//   * number of Member nodes
	//   * number of Client instances
	//
	// Example:
	//   go run launcher.go -n 3
	//
	// This will start:
	//   1 Leader
	//   3 Members
	//   3 Clients
	//
	n := flag.Int("n", 1, "number of clients and members")
	flag.Parse()

	// =========================================================================
	// STEP 3.2 – PROJECT CONFIGURATION
	// =========================================================================
	//
	// base:
	//   Absolute path to the Tolerex project directory
	//
	// startPort:
	//   First port used by Member nodes
	//   Subsequent Members increment this port
	//
	base := "D:\\Tolerex"
	startPort := 5556

	// =========================================================================
	// STEP 3.3 – START LEADER NODE
	// =========================================================================
	//
	// WHY FIRST:
	// -----------
	// Members and Clients depend on the Leader.
	//
	// WHAT HAPPENS:
	// -------------
	// - A new terminal window opens
	// - Leader gRPC + TCP servers start
	//
	fmt.Println("Opening Leader terminal...")
	newTerminal(
		"LEADER",
		fmt.Sprintf("cd %s; go run ./cmd/leader/main.go", base),
	)

	// =========================================================================
	// STEP 3.4 – WAIT FOR LEADER READINESS
	// =========================================================================
	//
	// WHY THIS IS MANUAL:
	// -------------------
	// - Simple
	// - Explicit
	// - No race conditions
	//
	waitEnter("Press ENTER once the Leader is ready")

	// =========================================================================
	// STEP 3.5 – START CLIENTS
	// =========================================================================
	//
	// Clients connect to the Leader's TCP interface.
	// Each client gets its own terminal window.
	//
	fmt.Println("Starting Clients...")
	for i := 1; i <= *n; i++ {
		newTerminal(
			fmt.Sprintf("CLIENT-%d", i),
			fmt.Sprintf("cd %s; go run ./client/test_client.go", base),
		)
	}

	// =========================================================================
	// STEP 3.6 – START MEMBERS
	// =========================================================================
	//
	// Each Member:
	// - Runs a gRPC server
	// - Uses a unique port
	// - Registers itself to the Leader
	//
	fmt.Println("Starting Members...")
	for i := 0; i < *n; i++ {
		port := startPort + i

		newTerminal(
			fmt.Sprintf("MEMBER-%d", port),
			fmt.Sprintf(
				"cd %s; go run ./cmd/member/main.go -port=%d",
				base, port,
			),
		)
	}

	// =========================================================================
	// STEP 3.7 – DONE
	// =========================================================================
	fmt.Println("All terminals started successfully")
}
