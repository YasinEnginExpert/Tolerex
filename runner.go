package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
)

func waitEnter(msg string) {
	fmt.Println(msg)
	bufio.NewReader(os.Stdin).ReadString('\n')
}

func newTerminal(title, command string) {
	// PowerShell
	psCmd := fmt.Sprintf(
		"$host.UI.RawUI.WindowTitle='%s'; "+
			"$size = New-Object Management.Automation.Host.Size(20,20); "+
			"$host.UI.RawUI.WindowSize = $size; "+
			"$host.UI.RawUI.BufferSize = $size; "+
			"%s",
		title, command,
	)

	cmd := exec.Command(
		"cmd",
		"/c",
		"start",
		"powershell",
		"-NoExit",
		"-Command",
		psCmd,
	)

	err := cmd.Start()
	if err != nil {
		fmt.Println("Terminal açılamadı:", err)
	}
}

func main() {
	n := flag.Int("n", 1, "number of clients and members")
	flag.Parse()

	base := "D:\\Tolerex"
	startPort := 5556

	// Leader
	fmt.Println(" Leader terminali açılıyor...")
	newTerminal(
		"LEADER",
		fmt.Sprintf("cd %s; go run ./cmd/leader/main.go", base),
	)

	// SADECE LEADER İÇİN ONAY
	waitEnter("Leader hazırsa ENTER'a bas")

	// Client'lar
	fmt.Println(" Client'lar başlatılıyor...")
	for i := 1; i <= *n; i++ {
		newTerminal(
			fmt.Sprintf("CLIENT-%d", i),
			fmt.Sprintf("cd %s; go run ./client/test_client.go", base),
		)
	}

	// Member'lar
	fmt.Println("Member'lar başlatılıyor...")
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

	fmt.Println("🎉 Tüm terminaller başarıyla açıldı")
}
