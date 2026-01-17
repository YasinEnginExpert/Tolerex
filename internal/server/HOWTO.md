# Tolerex Server Operations

This directory contains the core cluster coordination logic for the Tolerex system.

## Testing and Quality Assurance

### Unit and Integration Tests
Execute the comprehensive test suite for the server layer:
```bash
go test ./internal/server/... -v
```

### Race Condition Detection
Verify thread safety and concurrency controls:
```bash
go test -race ./internal/server/...
```

## Performance Profiling

### CPU Profiling
The system includes built-in support for CPU profiling during load tests.

1.  Execute the profiling test:
    ```bash
    go test ./internal/server/leader -run TestLeader_CPUProfile
    ```
2.  Analyze results using the Go pprof tool:
    ```bash
    go tool pprof internal/server/leader/leader_cpu.prof
    ```
3.  Open the interactive web interface:
    ```bash
    go tool pprof -http=:8080 internal/server/leader/leader_cpu.prof
    ```

## Modular Structure
- `leader/`: Cluster coordination and state management.
- `member/`: Data persistence and RPC handlers.
- `balancer/`: Algorithmic load distribution strategies.
- `shared/`: Canonical types and models.
