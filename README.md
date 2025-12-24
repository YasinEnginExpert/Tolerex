<div align="center">

<img src="https://readme-typing-svg.demolab.com?font=Fira+Code&size=24&duration=3000&pause=800&color=00ADD8&center=true&vCenter=true&width=750&lines=Distributed+Storage+System;Fault-Tolerant+Message+Queue;Leader-Based+Replication;Go+%2B+gRPC+%2B+mTLS;Inspired+by+Kafka+%26+RabbitMQ" />

</div>

<div align="center">

# Tolerex — HaToKuSe Distributed Message Storage System

<p>
  <img src="https://img.shields.io/badge/Language-Go-00ADD8?logo=go&logoColor=white" />
  <img src="https://img.shields.io/badge/gRPC-Protobuf-4A154B?logo=grpc&logoColor=white" />
  <img src="https://img.shields.io/badge/Protocol-HaToKuSe-orange" />
  <img src="https://img.shields.io/badge/Security-mTLS-success" />
  <img src="https://img.shields.io/badge/Status-Completed-8A2BE2" />
</p>

<p>
  <b>Tolerex</b> is a <b>fault-tolerant distributed message storage system</b>  
  developed as part of the <b>System Programming</b> course.
</p>

<p>
  Leader-based architecture • Configurable replication factor •  
  Crash-tolerant reads • Disk persistence • Secure gRPC communication
</p>

<p>
  <a href="#-quick-start">⚡ Quick Start</a> •
  <a href="#-architecture">🏗️ Architecture</a> •
  <a href="#-test-scenarios">🧪 Test Scenarios</a> •
  <a href="#-project-structure">📦 Project Structure</a>
</p>

</div>

<div align="center">

<img src="https://user-images.githubusercontent.com/74038190/212284068-b4bce7fa-2c74-4c5b-8c48-8e1f1c6e9b06.gif" width="800"/>

</div>

---

## Project Overview

This project implements a **distributed, fault-tolerant message storage system**
based on a **leader–replica (member) architecture**.

The system is inspired by modern distributed platforms such as  
**Apache Kafka** and **RabbitMQ**, and focuses on demonstrating:

- Replication and fault tolerance
- Load-balanced data distribution
- Crash-aware read recovery
- Disk-based persistent storage
- Secure inter-node communication using **gRPC + mTLS**

Client interaction is performed using a **custom lightweight text-based protocol**
called **HaToKuSe (Hata-Tolere Kuyruk Servisi)**.

---

## 🏗️ Architecture

Tolerex follows a **centralized leader-based architecture**.

### System Roles

- **Client**
  - Sends text-based `SET` and `GET` requests
  - Communicates only with the Leader

- **Leader Node (Coordinator)**
  - Parses HaToKuSe protocol commands
  - Reads replication factor from `tolerance.conf`
  - Selects replica nodes for data storage
  - Maintains metadata index  
    (`message_id → replica list`)
  - Handles failover during read operations

- **Replica Nodes (Members)**
  - Receive replicated data via gRPC
  - Persist messages to local disk
  - Periodically report storage statistics

---

### Request Flow Overview

1. Client sends a **text-based request** to the Leader
2. Leader parses the command and determines replication factor
3. Message is converted into a **Protobuf object**
4. Leader replicates the message to selected replica nodes
5. Replicas persist the message on disk and return ACKs
6. Leader aggregates responses and replies to the client

---

### System Architecture Diagram

```mermaid
flowchart TB

    %% ===== STYLE DEFINITIONS =====
    classDef client fill:#E3F2FD,stroke:#1E88E5,stroke-width:2px,color:#0D47A1
    classDef leader fill:#FFF3E0,stroke:#FB8C00,stroke-width:3px,color:#E65100
    classDef replica fill:#E8F5E9,stroke:#43A047,stroke-width:2px,color:#1B5E20

    %% ===== CLIENTS =====
    subgraph Clients["Clients (HaToKuSe Text Protocol)"]
        C1[Client_1]
        C2[Client_2]
        CN[Client_n]
    end
    class C1,C2,CN client

    %% ===== LEADER =====
    subgraph LeaderLayer["Leader Node"]
        Leader["Leader Node | Port 6666 | Port 5555 | tolerance.conf"]
    end
    class Leader leader

    %% ===== REPLICAS =====
    subgraph Replicas["Replica Nodes (gRPC / Protobuf)"]
        R1["Replica_1 | Port 5556"]
        R2["Replica_2 | Port 5557"]
        RN["Replica_n | Port 5555+n"]
    end
    class R1,R2,RN replica

    %% ===== FLOWS =====
    C1 -->|SET / GET| Leader
    C2 -->|SET / GET| Leader
    CN -->|SET / GET| Leader

    Leader -->|Protobuf| R1
    Leader -->|Protobuf| R2
    Leader -->|Protobuf| RN

    R1 -->|ACK| Leader
    R2 -->|ACK| Leader
    RN -->|ACK| Leader
````
---
### Project Structure
```text
.
├── go.mod
├── go.sum
├── LICENSE
├── README.md
│
├── client
│   └── test_client.go
│
├── cmd
│   ├── cluster_launcher
│   │   └── cluster_launcher.go
│   ├── leader
│   │   └── main.go
│   └── member
│       └── main.go
│
├── config
│   ├── tolerance.conf
│   └── tls
│       ├── ca.crt
│       ├── ca.key
│       ├── client.cnf
│       ├── client.crt
│       ├── client.key
│       ├── leader.cnf
│       ├── leader.crt
│       ├── leader.key
│       ├── member.cnf
│       ├── member.crt
│       └── member.key
│
├── internal
│   ├── config
│   │   └── config.go
│   ├── data
│   │   └── leader_state.json
│   ├── logger
│   │   └── logger.go
│   ├── metrics
│   │   └── metrics.go
│   ├── middleware
│   │   ├── interceptors.go
│   │   ├── logging.go
│   │   ├── metrics.go
│   │   └── recovery.go
│   ├── security
│   │   └── mtls.go
│   ├── server
│   │   ├── leader.go
│   │   └── member.go
│   └── storage
│       ├── reader.go
│       └── writer.go
│
├── logs
│   └── tolerex.log
│
└── proto
    ├── message.proto
    └── gen
        ├── message.pb.go
        └── message_grpc.pb.go
```
---

### Development Environment
This project is developed using Visual Studio Code with the following recommended extensions for an efficient Go + gRPC + Protobuf workflow:

- **Go (Go Team at Google)** – Go language support, formatting, and debugging
- **protobuf (kanging)** – Protobuf syntax highlightin
- **Proto Lint (Plex Systems)** – Protobuf linting and best practices
- **Error Lens (Alexander)** – Inline error highlighting
- **Better Comments (Aaron Bond)** – Improved code comment readability
- **Prettier (Prettier)** – Formatting for Markdown and configuration files
- **vscode-icons (VSCode Icons Team)** – Enhanced file explorer visuals
- **Docker (Microsoft, optional)** – Containerized development and testing

---

## Dependencies

The following Go libraries and frameworks are used in this project.

### Core Dependencies

- **google.golang.org/grpc**  
  Provides high-performance, type-safe RPC communication between distributed nodes.

- **google.golang.org/protobuf**  
  Used for defining and serializing structured gRPC messages.

- **github.com/google/uuid**  
  Generates universally unique identifiers (UUIDs) for requests and node identification.

- **github.com/prometheus/client_golang**  
  Exposes application metrics such as request counts and latency.

- **gopkg.in/natefinch/lumberjack.v2**  
  Handles log rotation and log file management.


## Observability and Instrumentation

The project uses OpenTelemetry for distributed tracing and metrics collection.

- **go.opentelemetry.io/otel**  
  Core OpenTelemetry API used for distributed tracing and observability.

- **go.opentelemetry.io/otel/sdk**  
  OpenTelemetry SDK implementation used to configure telemetry pipelines and exporters.

- **go.opentelemetry.io/otel/trace**  
  Tracing API for tracking request lifecycles across distributed gRPC calls.

- **go.opentelemetry.io/otel/metric**  
  Metrics API used for application-level instrumentation.

## Supporting and Indirect Dependencies

The following libraries are included indirectly as part of the dependency graph and are not
used directly in the application code:

- **github.com/prometheus/common**  
  Shared utilities for Prometheus clients.

- **github.com/prometheus/client_model**  
  Prometheus data model definitions.

- **github.com/prometheus/procfs**  
  Access to process and system metrics.

- **github.com/cespare/xxhash/v2**  
  High-performance hashing functions.

- **github.com/beorn7/perks**  
  Performance optimizations used by Prometheus.

- **golang.org/x/net**  
  Extended networking utilities.

- **golang.org/x/sys**  
  Low-level operating system abstractions.

- **golang.org/x/text**  
  Text processing and encoding support.

- **google.golang.org/genproto**  
  Generated Protocol Buffer definitions for Google APIs.

- **go.yaml.in/yaml/v2**  
  YAML parsing library used for configuration handling.

## Dependency Management

All dependencies are managed using **Go Modules**.
To download and verify all required packages, run:

```bash
go mod tidy
go mod download
```
---

### Test 1 – Initial System Validation
In Test 1, the Leader node is started, Member nodes join the cluster, and basic client operations are executed to verify correct communication and data flow.

[![Test 1 Video](https://img.youtube.com/vi/kz0HX8aq4wQ/0.jpg)](https://youtu.be/kz0HX8aq4wQ)

### Test 2 – Disk-Based Message Storage (Single Node)
In Test 2, messages are stored on disk using a single node. Each message is written to a separate file, and basic SET and GET operations are performed to verify correct disk read and write behavior. Buffered and unbuffered I/O approaches are introduced.

[![Test 2 Video](https://img.youtube.com/vi/mqYZ8ZRT5D4/0.jpg)](https://youtu.be/mqYZ8ZRT5D4)


### Test 3 – gRPC Message Model (Protobuf)
In Test 3, message exchange between the Leader and Members is modeled using Protobuf. A basic gRPC service is defined, and messages are sent and received using Protobuf-based data structures. At this stage, the focus is on establishing gRPC functionality rather than distributed execution.

[![Test 3 Video](https://img.youtube.com/vi/evnN6bgofg8/0.jpg)](https://youtu.be/evnN6bgofg8)


### Test 4 – Distributed Logging with Fault Tolerance (Tolerance=1,2)
In Test 4, a basic distributed logging mechanism is implemented for fault tolerance levels 1 and 2. The Leader stores incoming messages locally and replicates them to selected Members via gRPC based on the configured tolerance value. Read operations retrieve the message from the Leader or available Members.

[![Test 4 Video](https://img.youtube.com/vi/-CHNPo6JEkc/0.jpg)](https://youtu.be/-CHNPo6JEkc)


### Test 6 – General Fault Tolerance (n) and Load Balancing
In Test 6, a generalized fault tolerance mechanism is implemented for tolerance values from 1 to 7. Messages are distributed among Members using a balanced selection strategy (such as round-robin), and the system behavior under multiple SET operations is observed to evaluate load balancing.

[![Test 6 Video](https://img.youtube.com/vi/TSHtgNh90gI/0.jpg)](https://youtu.be/TSHtgNh90gI)

### Test 7 – Crash Scenarios and Recovery
In Test 7, crash scenarios are simulated by manually stopping Member nodes. During GET operations, the Leader detects failed Members, marks them as unavailable, and successfully retrieves messages from the remaining active nodes, demonstrating basic recovery behavior.

[![Test 7 Video](https://img.youtube.com/vi/3mGIgtAFrmg/0.jpg)](https://youtu.be/3mGIgtAFrmg)

### Books

- **Network Programming with Go**  
  *Essential Skills for Programming, Using and Securing Networks with Open Source Google Golang*  
  Jan Newmarch, Ronald Petty – 2nd Edition

- **Distributed Services with Go**  
  *Your Guide to Reliable, Scalable, and Maintainable Systems*  
  Travis Jeffery  
  Version: P1.0 (March 2021)

- **System Programming Essentials with Go**  
  *System calls, networking, efficiency, and security practices with practical projects*  
  Alex Rios

### Online Articles & Tutorials

- Murat Demirci – *Go and gRPC*  
  https://muratdemirci.com.tr/goandgrpc/

### Online Courses

- **Go Bootcamp with gRPC and Protocol Buffers** (Udemy)  
  https://www.udemy.com/course/gobootcampwithgrpcandprotocolbuffers

### Tools & Assistance

- **ChatGPT**  
  Used as an interactive assistant for:
  - Architectural discussions
  - Code review and refactoring
  - Documentation improvement

  - Conceptual explanations of distributed systems and Go internals



