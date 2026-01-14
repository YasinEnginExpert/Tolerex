# Config Package (`internal/config`) ⚙️

This package handles the loading and validation of system-critical configurations, specifically the Replication Tolerance factor.

## Functionality

### `tolerance.conf` Loader
The system's fault tolerance (N) is defined in a simple key-value file.
- **Format:** `TOLERANCE=N`
- **Validation:**
  - File must exist.
  - N must be a positive integer > 0.
  - Key must be exactly `TOLERANCE`.

## Testing (`config_test.go`)

Tests ensure the system fails fast (refuses to start) if the configuration is invalid.
- **Valid Case:** `TOLERANCE=3` → Returns 3.
- **Invalid Key:** `MY_VAL=3` → Returns Error.
- **Invalid Value:** `TOLERANCE=-1` → Returns Error.

## Running Tests 🚀

To run the tests for this package, execute the following command from the project root:

```bash
go test -v ./internal/config/...
```
