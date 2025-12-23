
package storage

import (
	"os"
	"path/filepath"
	"testing"
)

//
// Test 1: Write -> Read (happy path)
//
func TestWriteAndReadMessage(t *testing.T) {

	// --- TEMP BASE DIR ---
	baseDir := t.TempDir()

	// --- WRITE ---
	err := WriteMessage(baseDir, 1, "hello tolerex", "buffered")
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// --- READ ---
	msg, err := ReadMessage(baseDir, 1)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	// --- VERIFY ---
	if msg != "hello tolerex" {
		t.Errorf("expected 'hello tolerex', got '%s'", msg)
	}
}

//
// Test 2: Read non-existing message -> NOT_FOUND
//
func TestReadMessage_NotFound(t *testing.T) {

	baseDir := t.TempDir()

	_, err := ReadMessage(baseDir, 42)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if err.Error() != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND error, got %v", err)
	}
}

//
// Test 3: Overwrite existing message
//
func TestWriteMessage_Overwrite(t *testing.T) {

	baseDir := t.TempDir()

	// First write
	if err := WriteMessage(baseDir, 5, "old", "unbuffered"); err != nil {
		t.Fatalf("first write failed: %v", err)
	}

	// Overwrite
	if err := WriteMessage(baseDir, 5, "new", "unbuffered"); err != nil {
		t.Fatalf("overwrite failed: %v", err)
	}

	// Read back
	msg, err := ReadMessage(baseDir, 5)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	if msg != "new" {
		t.Errorf("expected 'new', got '%s'", msg)
	}
}

//
// Test 4: Messages directory is created
//
func TestWriteMessage_CreatesDirectory(t *testing.T) {

	baseDir := t.TempDir()

	err := WriteMessage(baseDir, 7, "data", "buffered")
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	dir := filepath.Join(baseDir, "messages")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("messages directory not created")
	}
}
