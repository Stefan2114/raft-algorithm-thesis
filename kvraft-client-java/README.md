# KVRaft Java Clerk

A high-performance, asynchronous Java client for the KVRaft distributed consensus cluster. 

## Features
- **Industry Standard Async API:** Built with `CompletableFuture` for non-blocking integration.
- **Automatic Leader Discovery:** Intelligently finds the active leader node in the Raft cluster.
- **Failover & Retries:** Automatically handles node closures and leadership changes.
- **Strong Typing:** Fully integrated with Protocol Buffers for type safety.

## How it Works
The `Clerk` acts as a gateway to your KVRaft cluster. In a Raft system, only the **Leader** can process write requests. The `Clerk`:
1.  Takes a list of all node addresses.
2.  Maintains an internal pointer to the "last known leader."
3.  If a request fails (or target node is a Follower), it cycles through the node list until it connects to the new leader.

## Prerequisites
- Java 21 or higher (configured for Java 26 in `pom.xml`).
- Maven 3.8+.
- A running KVRaft Go cluster.

## Installation
Build the project using Maven:
```bash
mvn clean compile
```

## Usage

### Creating the Clerk
```java
List<String> nodes = List.of("localhost:8000", "localhost:8001", "localhost:8002");
Clerk clerk = new Clerk(nodes);
```

### Performing Operations (Async)
```java
clerk.put("my-key", "my-value", 0)
     .thenAccept(v -> System.out.println("Success!"))
     .exceptionally(ex -> { ex.printStackTrace(); return null; });
```

### Performing Operations (Sync)
If you prefer a synchronous style for simple tools:
```java
GetResult result = clerk.get("my-key").join();
System.out.println("Value: " + result.getValue());
```

## Testing the Connection

To verify that the Java client can communicate with your Go cluster, follow these steps:

### 1. Start the Go Cluster
Run three instances of the Go server in separate terminals (from the `kvraft` directory):
```bash
# Node 0
./kvserver --id 0 --datadir ./data/node0
# Node 1
./kvserver --id 1 --datadir ./data/node1
# Node 2
./kvserver --id 2 --datadir ./data/node2
```

### 2. Run the Java Demo
Run the provided demo in another terminal:
```bash
mvn exec:java -Dexec.mainClass="com.kvraft.Main"
```

The output will show the Java client connecting, finding the leader, and performing both `Put` and `Get` operations.

## Configuration Details
- **Default Ports:** The client is configured to match `cluster.json` using ports `8000`, `8001`, and `8002`.
- **Proto definitions:** The client uses the shared `.proto` files located in the project root `/proto`.
