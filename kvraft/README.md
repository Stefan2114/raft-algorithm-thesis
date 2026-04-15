# KVRaft: Distributed Fault-Tolerant Key-Value Store

This project is a high-performance, distributed key-value store built on top of the **Raft Consensus Algorithm**. It provides linearizable consistency and high availability despite node failures or network partitions.

## Overview: The Raft Algorithm

### What is Raft?
Raft is a consensus algorithm that manages a replicated log and ensures that all nodes in a cluster eventually agree on the same sequence of operations, even if some nodes fail.

### How it Works
Raft decomposes the consensus problem into three relatively independent subproblems:
1.  **Leader Election**: When a cluster starts or an existing leader fails, a new leader is elected through a randomized timeout and voting process.
2.  **Log Replication**: The leader accepts client commands, appends them to its log, and replicates them to other nodes (Followers).
3.  **Safety**: Raft ensures that if any node has applied a particular log entry to its state machine, then no other node can apply a different command for the same log index.

---

## Architecture: The RSM Layer

The **Replicated State Machine (RSM)** is the core architectural pattern that makes this system work.

-   **Logic Separation**: The RSM layer (found in `internal/rsm`) acts as a buffer between the raw consensus log (Raft) and the application logic (the KV Store).
-   **Deterministic Execution**: All replicas start in the same state and apply the same commands in the same order. This ensures they all stay in sync.
-   **Snapshotting**: When the Raft log grows too large, the RSM takes a snapshot of the current state and instructs Raft to discard the old log entries, saving disk space and speeding up recovery.

### Connection to MIT 6.824
This project is inspired by the **MIT 6.824 (Distributed Systems)** labs. It follows the rigorous design principles of lab 3 and 4, reaching beyond the academic implementation by using production-ready technologies like gRPC and Zap logging.

---

## The KV Server Concept

The KV Server provides a simple interface: `Get(key)` and `Put(key, value)`. 

### Real World: etcd
In the real world, projects like **etcd** use this exact structure to store the "source of truth" for entire cloud clusters. If your KV store is fault-tolerant, your entire system can recover from almost any hardware failure.

### System Orchestration (`node.go`)
In `internal/kvserver/node.go`, we find the "Orchestrator" of the system. A `Node` instance encapsulates:
1.  **A Raft instance**: To reach consensus.
2.  **An RSM instance**: To manage the application state.
3.  **A gRPC Server**: To listen for client requests and peer-to-peer communication.

While there are multiple server instances to ensure fault tolerance, there is typically only one **Clerk** (client) instance per application session.

---

## The Clerk & Client SDK

The **Clerk** (implemented in `pkg/clerk`) is the SDK for the project.

-   **Abstraction**: The user doesn't need to know which node is the leader. They just call `clerk.Put("key", "value")`.
-   **Leader Discovery**: The Clerk maintains a list of server addresses. If a request fails or returns `ErrWrongLeader`, the Clerk automatically tries the next server in the list until it finds the current leader.
-   **Linearizability**: The Clerk uses unique IDs for every operation, ensuring that even if a request is retried due to a timeout, it is never executed twice by the RSM.

---

## Transportation: gRPC and Extensibility

This project uses **gRPC** for both client-to-server and server-to-server communication.

-   **Why gRPC?**: It provides strongly-typed interfaces via Protocol Buffers, built-in support for streaming, and excellent performance.
-   **Extensibility**: Using `grpc.NewClient` and protobuf definitions makes it trivial to write a Clerk in **Python, Java, or Rust** and have it interact with this Go-based KV cluster.

---

## Design Patterns

This project utilizes several classic design patterns to manage the complexity of distributed systems and ensure long-term extensibility.

### Creational: Static Factory Method
Found in `internal/raft/raft.go` and `internal/rsm/rsm.go`.

-   **Intent**: To encapsulate complex object construction that involves more than just field assignment—specifically, managing side effects like starting background processes.
-   **Problem Solved**: A distributed node is not functional until its internal loops are running. These factories handle the **complex orchestration** of spawning essential loops.
-   **Implementation (`internal/raft/raft.go`)**:
    ```go
    func Make(peers []Transport, me int,
        persister Persister, applyCh chan raftapi.ApplyMsg, logger *zap.Logger) raftapi.Raft {

        rf := &Raft{
            // ... internal state initialization ...
            state:          StateFollower,
            heartBeatTimer: time.NewTimer(StableHeartbeatTimeout()),
            electionTimer:  time.NewTimer(RandomizedElectionTimeout()),
            logger:         logger.With(zap.Int("node", me)),
        }

        for i := range peers {
            rf.replicatorCond[i] = sync.NewCond(&rf.mu)
            if i != me {
                go rf.replicator(i) // Spawning background replicator per peer
            }
        }

        rf.readPersist(persister.ReadRaftState())
        rf.applyCond = sync.NewCond(&rf.mu)

        go rf.ticker()  // Main election/heartbeat cycle
        go rf.applier() // Background loop to apply committed entries
        return rf
    }
    ```

---

### Structural: Adapter Pattern
Found in `raftransport/client.go` and `internal/kvserver/grpc.go`.

-   **Intent**: To convert the interface of a class into another interface that clients expect, allowing incompatible classes to work together.
-   **Example 1: The Transport Adapter (`raftransport/client.go`)**:
    -   **Incompatibility**: The core Raft algorithm expects a generic `Transport` interface with a `Call` method that takes `interface{}`. However, gRPC provides concrete, type-safe client stubs (e.g., `kvpb.RaftClient`) that require `context.Context` and specific Protobuf structs.
    -   **Adapter Solution**: `GRPCClient` "wraps" the gRPC client and translates the generic calls into gRPC-specific ones.
        ```go

    // internal/raft/raft.go
    type Transport interface {
        Call(method string, args interface{}, reply interface{}) bool
    }
    ```
    -   **The Solution**: `GRPCClient` "wraps" the gRPC client and translates the generic calls into gRPC-specific calls.
    ```go
    // GRPCClient adapts raft.Transport using a Raft gRPC client stub.
    type GRPCClient struct {
        Raft kvpb.RaftClient
    }

    func (c *GRPCClient) Call(method string, args interface{}, reply interface{}) bool {
        ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
        defer cancel()

        switch method {
        case "Raft.RequestVote":
            a := args.(*raft.RequestVoteArgs)
            r := reply.(*raft.RequestVoteReply)
            res, err := c.Raft.RequestVote(ctx, &kvpb.RequestVoteArgs{
                Term:         int32(a.Term),
                CandidateId:  int32(a.CandidateId),
                // ... mapping fields ...
            })
            if err != nil { return false }
            r.Term = int(res.Term)
            r.VoteGranted = res.VoteGranted
            return true
        // ... other methods ...
        }
    }
    ```

---

### Structural: Bridge Pattern
Found in `internal/rsm/rsm.go`, linking the **Consensus Layer** and the **Application Layer**.

-   **Intent**: To decouple an abstraction from its implementation so that the two can vary independently.
-   **The Bridge**: This pattern acts as a physical bridge between the **RSM (Abstraction)** and the **Store (Implementation)**.
-   **Implementation (`internal/rsm/rsm.go`)**:
    ```go
    type StateMachine interface {
        DoOp(any) any // The execution interface
        // ...
    }

    type RSM struct {
        rf raftapi.Raft // Consensus abstraction
        sm StateMachine // Execution implementation (Implementation: Store)
        // ...
    }
    ```
-   **Rationale**: The `RSM` handles the "distributed" complexity (logs, consistency), while the `Store` (defined in `internal/kvserver/store.go`) handles the "storage" complexity (Map/Persistent state). Even if you replace the storage engine with a real DB, the RSM code remains untouched.

---

### Behavioral: Command Pattern
Found in `internal/rsm/rsm.go` and `api/kv.go`.

-   **Intent**: To encapsulate a request as an object, letting you replicate and replay operations across a cluster.
-   **The Lifecycle**:
    1.  **Creation (Command Formulation)**:
        ```go
        // api/kv.go: Independent operation data
        type PutArgs struct { Key string; Value string; Version TVersion }
        ```
    2.  **Wrapping (In Submit)**:
        ```go
        // internal/rsm/rsm.go: Encapsulating the request
        func (rsm *RSM) Submit(req any) (api.Err, any) {
            op := Op{Me: rsm.me, Id: randValue(8), Req: req} // Command Object
            index, term, isLeader := rsm.rf.Start(op)        // Sending to Raft
        }
        ```
    3.  **Agnostic Replication (In Raft)**: **Raft does not know what resides inside the `Op`**. It treats the Command as a blind data blob to be replicated across nodes.
    4.  **Unwrapping (In Reader)**:
        ```go
        // internal/rsm/rsm.go: Handling committed entries
        func (rsm *RSM) handleCommand(msg raftapi.ApplyMsg) {
            op := msg.Command.(Op)       // Unwrapping
            resultVal := rsm.sm.DoOp(op.Req) // Execution
        }
        ```
-   **Rationale**: Handling "what" the operation is (Get, Put, etc.) is solely the business of the server/store; Raft's only concern is "that" the operation reaches all nodes reliably.

---

### Behavioral: Strategy Pattern
Found in `internal/raft/persister.go` and `persist/disk_persister.go`.

-   **Intent**: To define a family of algorithms/behaviors and make them interchangeable at runtime.
-   **The Strategy**: While the Bridge pattern (above) focuses on architectural separation, the Strategy pattern here focuses on **swapping persistence behavior**.
-   **Implementation**:
    ```go
    // internal/raft/persister.go: The Strategy Interface
    type Persister interface {
        ReadRaftState() ([]byte, error)
        Save(raftState []byte, snapshot []byte) error
        // ...
    }

    // persist/disk_persister.go: A Concrete Strategy
    func (ps *FilePersister) Save(raftState []byte, snapshot []byte) error {
        // Concrete implementation using disk files
    }
    ```
-   **Why it was used**: This allows the project to swap a `FilePersister` (for production) with an `InMemoryPersister` (for high-speed unit testing) without modifying the Raft core, which simply depends on the `Persister` interface.

---

## Getting Started

### 1. Generate Proto Files
If you modify the `.proto` files in the `proto/` directory, regenerate the Go code:
```bash
protoc --go_out=. --go_opt=module=kvraft --go-grpc_out=. --go-grpc_opt=module=kvraft proto/raft.proto proto/kv.proto
```

### 2. Build and Run
Build the server and CLI binaries:
```bash
go build -o kvserver ./cmd/kvserver
go build -o kvcli ./cmd/cli
```

To run a node:
```bash
./kvserver --config cluster.json --id 1 --dir ./data/node1
```