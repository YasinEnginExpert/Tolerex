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

	// --- FILE READ ---
	// Reads the entire configuration file into memory.
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	// --- LINE NORMALIZATION ---
	// Trims whitespace and newline characters.
	line := strings.TrimSpace(string(data))

	// --- KEY=VALUE PARSING ---
	parts := strings.Split(line, "=")
	if len(parts) != 2 || strings.ToUpper(parts[0]) != "TOLERANCE" {
		return 0, fmt.Errorf("invalid tolerance.conf format")
	}

	// --- INTEGER CONVERSION ---
	// Converts the tolerance value to integer.
	return strconv.Atoi(parts[1])
}
