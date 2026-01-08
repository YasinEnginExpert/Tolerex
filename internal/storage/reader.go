// ===================================================================================
// TOLEREX – STORAGE LAYER (READ PATH)
// ===================================================================================
//
// This file implements the low-level, filesystem-backed read operation
// for the Tolerex distributed storage system.
//
// From a systems programming perspective, this package:
//
// - Provides deterministic disk access primitives
// - Encapsulates filesystem path construction
// - Translates OS-level errors into domain-level semantics
// - Acts as the final persistence boundary in the data plane
//
// Design characteristics:
//
// - One message = one file
// - File naming is deterministic: <id>.msg
// - Directory layout is fixed and predictable
// - No caching, buffering, or concurrency abstraction is applied
//
// Error semantics:
//
// - If the file does not exist, a NOT_FOUND error is returned
// - Other I/O errors are propagated verbatim
//
// This function is intentionally minimal and synchronous,
// making behavior explicit and easy to reason about.
//
// ===================================================================================

package storage

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"tolerex/internal/logger"
)

// --- READ MESSAGE ---
// Reads a message from disk based on its numeric identifier.
//
// Parameters:
// - baseDir : base directory assigned to the Member
// - id      : message identifier
//
// Returns:
// - message content as string
// - error if the file does not exist or cannot be read
func ReadMessage(baseDir string, id int) (string, error) {

	// --- FILE PATH CONSTRUCTION ---
	// Constructs the absolute path to the message file:
	//   <baseDir>/messages/<id>.msg
	filename := filepath.Join(
		baseDir,
		"messages",
		strconv.Itoa(id)+".msg",
	)

	// --- FILE READ ---
	data, err := os.ReadFile(filename)
	if err != nil {

		// --- NOT FOUND TRANSLATION ---
		// Explicitly maps os.ErrNotExist to a domain-level NOT_FOUND error.
		if os.IsNotExist(err) {
			logger.Debug(logger.Member, "ReadMessage: file not found id=%d path=%s", id, filename)
			return "", errors.New("NOT_FOUND")
		}

		// --- PROPAGATE OTHER ERRORS ---
		logger.Error(logger.Member, "ReadMessage: read error id=%d path=%s err=%v", id, filename, err)
		return "", err
	}

	// --- SUCCESS PATH ---
	return string(data), nil
}
