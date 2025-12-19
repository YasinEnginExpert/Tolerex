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
