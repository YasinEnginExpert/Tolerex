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
func WriteMessage(baseDir string, id int, text string) error {

	// --- DIRECTORY ENSURE ---
	// Ensures that the messages directory exists:
	//   <baseDir>/messages/
	dir := filepath.Join(baseDir, "messages")
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return err
	}

	// --- FILE PATH CONSTRUCTION ---
	// Constructs the full path for the message file:
	//   <baseDir>/messages/<id>.msg
	filename := filepath.Join(
		dir,
		strconv.Itoa(id)+".msg",
	)

	// --- FILE WRITE ---
	// Writes the message content to disk.
	// Existing files with the same ID will be overwritten.
	return os.WriteFile(filename, []byte(text), 0644)
}
