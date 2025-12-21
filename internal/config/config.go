// ===================================================================================
// TOLEREX – CONFIGURATION LOADER (TOLERANCE)
// ===================================================================================
//
// This file provides low-level configuration parsing utilities for the Tolerex
// distributed storage system.
//
// Specifically, it is responsible for loading and validating the replication
// tolerance parameter used by the Leader to determine fault tolerance behavior.
//
// From a systems perspective:
//
// - Configuration is read from a plain-text file (tolerance.conf)
// - The format is intentionally simple to avoid external dependencies
// - Validation is performed eagerly at startup to fail fast on misconfiguration
//
// Expected file format:
//
//   TOLERANCE=<integer>
//
// Example:
//
//   TOLERANCE=2
//
// This package performs no caching and no runtime reloading.
// Configuration is assumed to be static for the lifetime of the process.
//
// ===================================================================================

package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// --- TOLERANCE CONFIG READER ---
// Reads and parses the replication tolerance value from the given file path.
// Returns an error if the file format is invalid or the value is not an integer.
func ReadTolerance(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	line := strings.TrimSpace(string(data))

	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid tolerance.conf format, expected TOLERANCE=<positive integer>")
	}

	key := strings.TrimSpace(parts[0])
	val := strings.TrimSpace(parts[1])

	if key != "TOLERANCE" {
		return 0, fmt.Errorf("invalid tolerance.conf format, expected TOLERANCE=<positive integer>")
	}

	tol, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("tolerance must be a valid integer")
	}

	if tol <= 0 {
		return 0, fmt.Errorf("tolerance must be greater than 0")
	}

	return tol, nil
}
