// ===================================================================================
// TOLEREX – STORAGE LAYER (WRITE PATH)
// ===================================================================================
//
// This file implements the low-level, filesystem-backed write operation
// for the Tolerex distributed storage system.
//
// From a systems programming perspective, this function:
//
// - Persists message content to disk using a deterministic file layout
// - Ensures the target directory exists before writing
// - Relies on the operating system for buffering and durability semantics
// - Performs a synchronous write using os.WriteFile
//
// Design characteristics:
//
// - One message = one file
// - File naming convention: <id>.msg
// - Directory layout: <baseDir>/messages/
// - No in-memory buffering or caching
//
// Durability semantics:
//
// - Data is written using os.WriteFile
// - fsync is NOT explicitly invoked
// - Atomicity is dependent on the underlying filesystem behavior
//
// This implementation is intentionally minimal and explicit,
// making it easy to reason about correctness and failure modes.
//
// ===================================================================================

package storage

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
)

// --- WRITE MESSAGE ---
// Writes a message to disk using its numeric identifier as filename.
//
// Parameters:
// - baseDir : base directory assigned to the Member
// - id      : message identifier
// - text    : message payload
//
// Returns:
// - error if directory creation or file write fails
// --- WRITE MESSAGE (DISPATCHER) ---
func WriteMessage(baseDir string, id int, text string, mode string) error {

	// Ensure messages directory exists
	dir := filepath.Join(baseDir, "messages")
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return err
	}

	switch mode {
	case "unbuffered":
		return writeUnbuffered(dir, id, text)
	default:
		return writeBuffered(dir, id, text)
	}
}

func writeBuffered(dir string, id int, text string) error {
	filename := filepath.Join(dir, strconv.Itoa(id)+".msg")

	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	if _, err := w.WriteString(text); err != nil {
		return err
	}
	return w.Flush()
}

func writeUnbuffered(dir string, id int, text string) error {
	filename := filepath.Join(dir, strconv.Itoa(id)+".msg")
	return os.WriteFile(filename, []byte(text), 0644)
}
