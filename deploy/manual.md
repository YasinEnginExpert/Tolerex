# Tolerex Monitoring Setup

This folder contains the infrastructure to monitor your running Tolerex cluster using **Prometheus** (data collector) and **Grafana** (dashboard).

## prerequisites
- Docker Desktop installed and running.

## usage

1. **Start the Leader**:
   ```bash
   go run cmd/leader/main.go
   # Metrics will be at localhost:9090
   ```

2. **Start Members** (Run in separate terminals):
   Currently, you must specify a unique **gRPC port** AND a unique **Metrics port** for each member.

   **Member 1:**
   ```bash
   go run cmd/member/main.go -port 5556 -metrics 9092
   ```

   **Member 2:**
   ```bash
   go run cmd/member/main.go -port 5557 -metrics 9093
   ```

   **Member 3:**
   ```bash
   go run cmd/member/main.go -port 5558 -metrics 9094
   ```

3. **Start Monitoring Stack**:
   Open a terminal in this `deploy` folder and run:
   ```bash
   docker-compose up -d
   ```

4. **Monitoring Multiple Members**:
   PROMETHEUS needs to know where to find your new members.
   
   - Open `deploy/prometheus.yml`
   - Add the new ports to the list:
     ```yaml
     - targets: 
       - 'host.docker.internal:9092'
       - 'host.docker.internal:9093'
       - 'host.docker.internal:9094'
     ```
   - Restart Prometheus to apply changes:
     ```bash
     docker-compose restart prometheus
     ```

## Accessing Grafana

1. **Open**: [http://localhost:3000](http://localhost:3000)
2. **Login**: `admin` / `admin`
3. **Connect Prometheus**:
   - Connections -> Add data source -> Prometheus
   - URL: `http://prometheus:9090`
   - Save & Test.
4. **Import Dashboard (Recommended)**:
   - Go to **Dashboards** (icon on left) -> **New** -> **Import**.
   - **Upload dashboard JSON file**: Select the `deploy/grafana_dashboard.json` file I created for you.
   - Or copy-paste the content of `deploy/grafana_dashboard.json`.
   - Select your **Prometheus** data source at the bottom.
   - Click **Import**.

   You will see graphs for:
   - Request Rate
   - P95 Latency
   - Error Rates

## Troubleshooting: "No Data" in Dashboard

If your dashboard is empty, check these steps:

1.  **Check Prometheus Targets**:
    -   Open [http://localhost:9091/targets](http://localhost:9091/targets) in your browser.
    -   **State MUST be "UP"** (Green).
    -   If State is **"DOWN"** (Red):
        -   Docker cannot reach your host machine.
        -   Ensure `go run` is actually running.
        -   Check Windows Firewall (Allow "main.exe" or turn off firewall briefly to test).

2.  **Generate Traffic**:
    -   Metrics only appear when requests happen.
    -   **Leader Metrics**: Driven by Heartbeats (automatic) and Registrations.
    -   **Member Metrics**: Driven by Replication (`SET` commands).
    -   Run the client loop to generate load: `go run client/test_client.go -measure`

3.  **Check Data Source**:
    -   In Grafana -> Data Sources -> Prometheus -> Click "Save & Test".
    -   It should say "Success".
