# Containerlab User Guide for Tolerex

This manual explains how to run the Tolerex Distributed System using **Containerlab** (`clab`). This approach is ideal for visualizing network topology and simulating a production-grade environment with core network elements (like a Nokia Switch).

## 1. Prerequisites

Before you begin, ensure you have the following installed on your Linux machine (or WSL2 on Windows):

1.  **Docker**: Container runtime.
2.  **Containerlab**: The orchestration tool.
    ```bash
    # Install Containerlab
    bash -c "$(curl -sL https://get.containerlab.dev)"
    ```

## 2. Build the Tolerex Image

Containerlab needs a local Docker image for the custom Tolerex nodes (Leader & Members).

1.  Navigate to the project root:
    ```bash
    cd /path/to/Tolerex
    ```
2.  Build the unified image:
    ```bash
    docker build -t tolerex-node:latest .
    ```

## 3. Deploy the Lab

The topology is defined in `deploy/tolerex.clab.yml`.

1.  Navigate to the deploy directory:
    ```bash
    cd deploy
    ```
2.  Run the deploy command:
    ```bash
    sudo containerlab deploy --topo tolerex.clab.yml
    ```

**What happens next?**
- Containerlab starts all nodes (Nokia Switch, Leader, Members, Prometheus, Grafana).
- It wires up the virtual ethernet links defined in the topology.
- It displays a summary table with IP addresses and ports.

## 4. Accessing the Lab

Once deployed, the services are accessible via localhost ports:

*   **Grafana Dashboard:** `http://localhost:3000`
*   **Prometheus:** `http://localhost:9091`
*   **Leader TCP (HaToKuSe):** `localhost:6666`

### Running a Client
You can run the client from your host machine (outside the lab) connecting to the exposed port:

```bash
# Connect to the Leader via the lab's exposed port
go run client/test_client.go -addr localhost:6666
```

## 5. Visualizing the Topology

One of the best features of Containerlab is the topology graph.

1.  When the lab is running, Containerlab hosts a web UI at `https://topology.containerlab.dev` (or you can run `clab graph`).
2.  However, for a local view, inspect the `clab-tolerex-lab` directory created by the tool.

## 6. Cleanup

To stop and remove the lab:

```bash
sudo containerlab destroy --topo tolerex.clab.yml
```

---

## Topology Overview

```
       [ Client ] 
           | (Host Access)
           v
    +--------------+
    | Nokia Switch |
    +--------------+
      |         |
      v         v
 [ Grafana ] [ LEADER ]
                |
        +-------+-------+
        |       |       |
    [Mem-1]  [Mem-2]  [Mem-3]
```

## Troubleshooting

*   **Image not found:** Ensure you ran `docker build -t tolerex-node:latest .` locally.
*   **Port conflicts:** Check if ports 6666, 3000, or 9090 are already in use on your host.
