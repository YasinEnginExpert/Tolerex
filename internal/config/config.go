package config

import (
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
	return strconv.Atoi(line)
}
