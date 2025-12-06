package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func ReadTolerance(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	line := strings.TrimSpace(string(data))
	parts := strings.Split(line, "=")
	if len(parts) != 2 || strings.ToUpper(parts[0]) != "TOLERANCE" {
		return 0, fmt.Errorf("tolerance.conf formati hatali")
	}

	return strconv.Atoi(parts[1])
}
