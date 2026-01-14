# Server Package (`internal/server`) 🧠

This package contains the core business logic of the Tolerex distributed system. It implements the "Control Plane" (Leader) and "Data Plane" (Member) logic.

## Components

### 1. Leader Server (`leader.go`)
The brain of the system.
- **Responsibilities:**
  - Manages cluster membership (Heartbeats).
  - Handles client requests (HaToKuSe protocol).
  - Coordinates replication to Member nodes.
  - Maintains metadata (`leader_state.json`) and connection pools.

### 2. Member Server (`member.go`)
The storage worker.
- **Responsibilities:**
  - Receives `Store` RPCs from the Leader.
  - Persists data to disk (via `internal/storage`).
  - Serves read requests.

### 3. Load Balancer (`balancer.go`)
Decides where to store data.
- **Least Loaded:** Picks members with the absolute lowest message count.
- **Power of Two Choices (P2C):** Picks 2 random members and chooses the better one.

## Testing (`_test.go`)

- **`leader_test.go`**: Verifies that the Leader initializes correctly, loads configuration from disk, and maintains state.
- **`balancer_test.go`**: Mathematically proves that:
  - The `LeastLoaded` strategy correctly sorts and picks the best candidates.
  - The `P2C` strategy returns valid subsets of members.

## Running Tests 🚀

To run the tests for this package (Leader, Member, Balancer), execute the following command from the project root:

```bash
go test -v ./internal/server/...
```
