// ===================================================================================
// TOLEREX – CONFIGURATION LOADER (REPLICATION TOLERANCE)
// ===================================================================================
//
// This package provides low-level configuration parsing utilities for the
// Tolerex distributed, fault-tolerant storage system.
//
// Its sole responsibility is loading and validating the **replication tolerance**
// parameter, which is a critical system-wide setting used by the Leader node to
// determine fault tolerance behavior.
//
// From a systems-design perspective:
//
//   - Configuration is read from a plain-text file (tolerance.conf)
//   - The format is intentionally minimal and dependency-free
//   - Validation is performed eagerly at startup (fail-fast philosophy)
//   - No caching or runtime reloading is performed
//
// Configuration is assumed to be **static for the lifetime of the process**.
// Any misconfiguration should immediately prevent the system from starting.
//
// -------------------------------------------------------------------------------
// EXPECTED FILE FORMAT
// -------------------------------------------------------------------------------
//
//   TOLERANCE=<positive integer>
//
// Example:
//
//   TOLERANCE=2
//
// This indicates that the system should tolerate up to N failures by replicating
// each message to N distinct members.
//
// ===================================================================================

package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"tolerex/internal/logger"
)

// ===================================================================================
// READ TOLERANCE CONFIGURATION
// ===================================================================================
//
// ReadTolerance loads and parses the replication tolerance value from the given
// file path.
//
// Validation rules:
//   - File must be readable
//   - Format must be exactly: TOLERANCE=<integer>
//   - Key must be "TOLERANCE"
//   - Value must be a positive integer (> 0)
//
// Returns:
//   - The parsed tolerance value on success
//   - An error on any format, parsing, or validation failure
//
// This function performs no normalization, caching, or fallback behavior.
// Any error is considered fatal to system startup.

func ReadTolerance(path string) (int, error) {

	// ---------------------------------------------------------------------------
	// READ FILE CONTENT
	// ---------------------------------------------------------------------------
	//
	// The entire file is read at once. The expected file size is minimal
	// (single line), so this is safe and efficient.

	data, err := os.ReadFile(path)
	if err != nil {
		logger.Error(logger.Member, "ReadTolerance: failed to read file path=%s err=%v", path, err)
		return 0, err
	}

	// ---------------------------------------------------------------------------
	// PARSE LINE
	// ---------------------------------------------------------------------------
	//
	// Whitespace is trimmed to tolerate trailing newlines or spaces.

	line := strings.TrimSpace(string(data))

	// Split on the first '=' only.
	// This prevents malformed inputs with multiple '=' characters.
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		logger.Error(logger.Member, "ReadTolerance: invalid format line=%s", line)
		return 0, fmt.Errorf(
			"invalid tolerance.conf format, expected TOLERANCE=<positive integer>",
		)
	}

	key := strings.TrimSpace(parts[0])
	val := strings.TrimSpace(parts[1])

	// ---------------------------------------------------------------------------
	// VALIDATE KEY
	// ---------------------------------------------------------------------------
	//
	// The key must be exactly "TOLERANCE".
	// No aliases or lowercase variants are accepted by design.

	if key != "TOLERANCE" {
		logger.Error(logger.Member, "ReadTolerance: invalid key=%s", key)
		return 0, fmt.Errorf(
			"invalid tolerance.conf format, expected TOLERANCE=<positive integer>",
		)
	}

	// ---------------------------------------------------------------------------
	// PARSE INTEGER VALUE
	// ---------------------------------------------------------------------------

	tol, err := strconv.Atoi(val)
	if err != nil {
		logger.Error(logger.Member, "ReadTolerance: failed to parse tolerance value err=%v", err)
		return 0, fmt.Errorf("tolerance must be a valid integer")
	}

	// ---------------------------------------------------------------------------
	// SEMANTIC VALIDATION
	// ---------------------------------------------------------------------------
	//
	// A tolerance value <= 0 is meaningless in a replication context.
	// Enforcing this here prevents undefined behavior later in the system.

	if tol <= 0 {
		logger.Error(logger.Member, "ReadTolerance: invalid tolerance value=%d", tol)
		return 0, fmt.Errorf("tolerance must be greater than 0")
	}

	return tol, nil
}
