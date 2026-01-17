# Tolerex Storage Engine Operations

This directory contains the persistent storage layer for the Tolerex system, managing disk I/O and data integrity.

## Testing

Execute the storage layer tests:
```bash
go test ./internal/storage -v
```

## Benchmarking

### Standard Benchmark
Measure basic throughput and memory allocations:
```bash
go test ./internal/storage -bench=Write -benchtime=5s -benchmem
```

### Ultra-Detailed Professional Analysis
Runs the full suite with 1800s (30m) benchtime and generates all profiles.

```powershell
go test ./internal/storage -bench=Write -benchtime=1800s -benchmem -v -count=1 -timeout=60m -cpuprofile=cpu.pprof -memprofile=mem.pprof -blockprofile=block.pprof -mutexprofile=mutex.pprof -trace=trace.out | Tee-Object -FilePath "benchmark_detailed_report.txt"
```

**Output Files:**
- `benchmark_full_results.json`: Machine-readable results (throughput, memory, timings).
- `cpu.pprof`: Analysis of function execution time.
- `mem.pprof`: Identification of memory leak or high allocation areas.
- `block.pprof`: Goroutine wait states (very useful for storage).
- `mutex.pprof`: Lock contention (useful for parallel writers).
- `trace.out`: Microsecond-precision execution timeline.

**How to analyze:**
1. **Visual CPU Profile:** `go tool pprof -http=:8080 cpu.pprof`
2. **Visual Trace:** `go tool trace trace.out`
3. **Compare Matrix:** Open `benchmark_full_results.json` to see how buffer size affects throughput.

## I/O Modes
- **Buffered:** Utilizes userspace caching for high-throughput stream writing.
- **Unbuffered:** Employs direct system calls for critical data durability guarantees.
