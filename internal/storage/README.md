# Storage Package (`internal/storage`) 💾

This package acts as the "Storage Engine" for Tolerex, handling all low-level disk I/O operations.

## Components

### 1. Writer (`writer.go`)
Persists messages to disk using a unique ID (e.g., `100.msg`).
- **Buffered IO:** Uses `bufio` for high performance (High Throughput).
- **Unbuffered IO:** Uses `os.WriteFile` for guarantees (High Durability).

### 2. Reader (`reader.go`)
Retrieves messages from disk.
- Direct filesystem access.
- Returns specific `NOT_FOUND` errors for missing messages.

## Testing (`storage_test.go`)

Tests verify the integrity of the filesystem operations.
- **Write-Read Cycle:** Writes a message and reads it back to confirm content match.
- **Overwrite:** Ensures updates to existing IDs work correctly.
- **Not Found:** Verifies correct error handling for missing files.

## Running Tests 🚀

To run the tests for this package, execute the following command from the project root:

```bash
go test -v ./internal/storage/...
```
