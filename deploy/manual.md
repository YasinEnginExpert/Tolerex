# Tolerex Monitoring Setup

This guide details the infrastructure for monitoring a Tolerex cluster using Prometheus and Grafana.

## Prerequisites
- Docker and Docker Compose installed and running.

## Monitoring Configuration

### 1. Leader Node
- Start the Leader server.
- Default metrics endpoint: `http://localhost:9090/metrics`

### 2. Member Nodes
Each member must be assigned a unique Metrics port to avoid conflicts.

- **Member 1:** `go run cmd/member/main.go -port 5556 -metrics 9092`
- **Member 2:** `go run cmd/member/main.go -port 5557 -metrics 9093`
- **Member 3:** `go run cmd/member/main.go -port 5558 -metrics 9094`

### 3. Monitoring Stack
Navigate to the `deploy` directory and start the stack:
```bash
docker-compose up -d
```

### 4. Prometheus Target Discovery
Update `deploy/prometheus.yml` to include your member endpoints. Use `host.docker.internal` for connections from the container to the host machine on Windows/macOS.

```yaml
- targets: 
  - 'host.docker.internal:9092'
  - 'host.docker.internal:9093'
  - 'host.docker.internal:9094'
```

Restart Prometheus to apply configuration changes:
```bash
docker-compose restart prometheus
```

## Grafana Visualization

1.  **Access:** Open `http://localhost:3000` in your browser.
2.  **Credentials:** Default is `admin` / `admin`.
3.  **Data Source Setup:**
    - Navigate to Connections -> Add data source -> Prometheus.
    - Set URL to `http://prometheus:9090`.
    - Click Save & Test.
4.  **Dashboard Import:**
    - Go to Dashboards -> New -> Import.
    - Upload the `deploy/grafana_dashboard.json` file.
    - Select the Prometheus data source and click Import.

## Verification

- **Prometheus Status:** Monitor targets at `http://localhost:9091/targets`. All scrapers should show an "UP" state.
- **Data Flow:** Metrics are generated during active heartbeat cycles and replication operations. Run a stress test at the client layer to verify throughput visualization.
