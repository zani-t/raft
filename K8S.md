# Deploying Raft Consensus to Minikube

This guide explains how to deploy a distributed Raft consensus implementation to a Minikube Kubernetes cluster. The implementation provides a fault-tolerant key-value store with automatic leader election and log replication across multiple nodes.

## Prerequisites

- **Minikube** installed and running
- **kubectl** configured to access your Minikube cluster
- **Go 1.25** or later (for building locally)
- **Docker** (Minikube uses Docker for containers)
- Basic familiarity with Kubernetes concepts

### Start Minikube

```bash
minikube start
```

## Project Structure

```
.
├── main.go                 # Entry point with commit listener
├── client.go               # RPC client for cluster interaction
├── Dockerfile              # Container image definition
├── go.mod                  # Go module dependencies
├── reset.sh                # Script to rebuild and redeploy
├── src/
│   ├── raft.go            # Core Raft consensus algorithm
│   ├── server.go          # RPC server and peer management
│   ├── storage.go         # In-memory key-value storage
│   ├── testharness.go     # Testing utilities
│   └── raft_test.go       # Unit tests
└── k8s/
    └── deployment.yaml    # Kubernetes StatefulSet & Service
```

## Key Components

### 1. Core Raft Implementation (`src/raft.go`)

The `ConsensusModule` implements the Raft algorithm with:
- **Persistent state**: Current term, voted-for tracking, and log entries
- **Volatile state**: Commit index, last applied index, and server state (Follower/Candidate/Leader)
- **Leader state**: Next index and match index for each peer
- **RPC mechanisms**: RequestVote and AppendEntries calls

The module handles:
- Election timeouts and leader election
- Log replication to followers
- Commit index advancement based on majority replication
- State transitions between Follower, Candidate, and Leader

### 2. RPC Server (`src/server.go`)

The `Server` manages:
- **Network communication** via Go RPC on port 8080
- **Peer discovery** using Kubernetes StatefulSet DNS names (`raft-cluster-{id}.raft-service`)
- **Command submission** through the `Submit` RPC method
- **State reporting** via the `Report` method
- **Storage integration** for reading committed values

Key feature: Kubernetes environment detection via `HOSTNAME` allows automatic peer ID assignment.

### 3. Storage Layer (`src/storage.go`)

The `MapStorage` interface provides:
- Thread-safe in-memory key-value storage
- Simple Get/Set operations
- Used for both consensus state persistence and application data

### 4. Client (`client.go`)

A command-line client supporting:
- `status`: Query node state (term, leader status)
- `write`: Submit key-value pairs to the cluster
- `read`: Read values from the cluster
- `test-consensus`: Verify data replication across nodes

### 5. Entry Point (`main.go`)

Initializes the cluster and crucially, **applies committed entries to storage**:

```go
go func() {
    for entry := range commit {
        if cmd, ok := entry.Command.(raft.Command); ok {
            storage.Set(cmd.Key, []byte(cmd.Value))
            log.Printf("Applied command: key=%s, value=%s", cmd.Key, cmd.Value)
        }
    }
}()
```

This listener consumes the commit channel and persists commands to storage.

## Deployment Steps

### Step 1: Build the Docker Image

Configure Docker to use Minikube's daemon:

```bash
eval $(minikube docker-env)
docker build -t raft-server:latest .
```

The `Dockerfile` builds a Go binary and runs it in an Alpine Linux container.

### Step 2: Deploy to Kubernetes

Apply the StatefulSet and Service:

```bash
kubectl apply -f k8s/deployment.yaml
```

This creates:
- **StatefulSet** `raft-cluster` with 3 replicas
- **Headless Service** `raft-service` for inter-pod DNS resolution

### Step 3: Verify the Deployment

Check pod status:

```bash
kubectl get pods -l app=raft-node
kubectl logs raft-cluster-0
kubectl logs raft-cluster-1
kubectl logs raft-cluster-2
```

Watch for log messages indicating:
- Successful listener startup
- Peer connection attempts
- Leader election (one node becomes leader, others are followers)

### Step 4: Set Up Port Forwarding

Forward local ports to access the cluster:

```bash
kubectl port-forward raft-cluster-0 8080:8080 &
kubectl port-forward raft-cluster-1 8081:8080 &
kubectl port-forward raft-cluster-2 8082:8080 &
```

## Testing the Cluster

### Verify Node Status

```bash
go run client.go status localhost:8080
go run client.go status localhost:8081
go run client.go status localhost:8082
```

Output shows the term and which node is the leader:
```
Node 0 (at localhost:8080):
  Term: 5
  Is Leader: true
```

### Write Data

Write to the leader (verify leadership first):

```bash
go run client.go write localhost:8080 mykey myvalue
```

### Read Data

Read from any node (data should be consistent):

```bash
go run client.go read localhost:8080 mykey
go run client.go read localhost:8081 mykey
```

### Run Consensus Test

Verify data replication across the cluster:

```bash
go run client.go test-consensus
```

This writes to one node and reads from another, confirming consensus.

## How Kubernetes Integration Works

### StatefulSet DNS

Kubernetes StatefulSet automatically provides stable DNS names:
- `raft-cluster-0.raft-service` → Pod 0
- `raft-cluster-1.raft-service` → Pod 1
- `raft-cluster-2.raft-service` → Pod 2

The `server.go` extracts the pod ordinal from the `HOSTNAME` environment variable and uses these DNS names for peer discovery.

### Automatic Peer Discovery

In `server.go`, the `NewServer` function detects Kubernetes:

```go
if hostname := os.Getenv("HOSTNAME"); strings.HasPrefix(hostname, "raft-cluster-") {
    ordinal := strings.TrimPrefix(hostname, "raft-cluster-")
    if id, err := strconv.Atoi(ordinal); err == nil {
        serverId = id
        // Generate peer IDs for 3-node cluster
    }
}
```

This eliminates the need for external configuration—each pod automatically discovers its peers.

### Fixed Port Binding

The server detects Kubernetes via the `KUBERNETES_SERVICE_HOST` environment variable:

```go
port := ":0"  // Local testing: OS assigns random port
if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
    port = ":8080"  // Kubernetes: use fixed port
}
```

## Updating and Redeploying

Use the provided `reset.sh` script to rebuild and redeploy:

```bash
./reset.sh
```

This script:
1. Configures Docker to use Minikube
2. Builds the container image
3. Deletes old pods
4. Applies the new deployment

Alternatively, manually:

```bash
eval $(minikube docker-env)
docker build -t raft-server:latest .
kubectl delete pod -l app=raft-node
kubectl apply -f k8s/deployment.yaml
```

## Troubleshooting

### Pods Not Starting

Check logs:
```bash
kubectl logs raft-cluster-0
kubectl describe pod raft-cluster-0
```

Common issues:
- Image not found: Ensure `eval $(minikube docker-env)` before building
- Port conflicts: Ensure port 8080 is available

### Peers Not Connecting

Verify service and DNS:
```bash
kubectl get svc raft-service
kubectl run -it --rm debug --image=alpine --restart=Never -- sh
# Inside pod: nslookup raft-service
```

### Consensus Not Reaching

Check that all pods are running and the commit listener in `main.go` is properly applied. Verify logs show:
```
Applied command: key=..., value=...
```

## Scaling the Cluster

To modify the cluster size, edit `k8s/deployment.yaml`:

```yaml
spec:
  replicas: 5  # Change from 3 to 5
```

Then:
```bash
kubectl apply -f k8s/deployment.yaml
```

Note: Update the hardcoded replicas count in `server.go` if needed:
```go
replicas := 5  // Update this constant
```

## Architecture Summary

```
┌─────────────────────────────────────────┐
│      Kubernetes Cluster (Minikube)      │
├─────────────────────────────────────────┤
│                                         │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐  │
│  │ Raft-0  │  │ Raft-1  │  │ Raft-2  │  │
│  │ (Pod)   │  │ (Pod)   │  │ (Pod)   │  │
│  └────┬────┘  └────┬────┘  └────┬────┘  │
│       │            │            │       │
│       └────────────┼────────────┘       │
│                    │                    │
│    StatefulSet DNS Resolution           │
│    raft-cluster-{0,1,2}.raft-service    │
│                                         │
│  ┌──────────────────────────────────┐   │
│  │  Headless Service (raft-service) │   │
│  └──────────────────────────────────┘   │
│                                         │
└─────────────────────────────────────────┘
         │
         │ Port Forwarding
         ▼
    localhost:{8080,8081,8082}
         │
    [Client - client.go]
```

## Performance Considerations

- **Election timeout**: Randomized between 150-300ms to prevent split brain
- **Heartbeat interval**: 50ms for followers to detect leader failure
- **RPC latency simulation**: 1-5ms added to RPC calls for realistic testing
- **In-memory storage**: Fast but limited to available RAM

For production, replace `MapStorage` with persistent backends (RocksDB, etcd, etc.).

## Further Customization

### Modify Cluster Size

Update `replicas` in `k8s/deployment.yaml` and adjust the hardcoded count in `server.go`:

```go
replicas := 3  // src/server.go, line ~67
```

### Add Persistence

Replace `MapStorage` with a persistent backend:

```go
// src/storage.go
type PersistentStorage struct {
    // Use SQLite, RocksDB, or similar
}
```

### Implement Log Compaction

The current implementation stores the entire log. Add snapshotting to `raft.go` for large deployments.

### Add Metrics

Integrate Prometheus metrics to monitor:
- Election frequency
- Log replication latency
- Consensus time
- RPC error rates

## References

- [Raft Consensus Algorithm](https://raft.github.io/)
- [Kubernetes StatefulSet Documentation](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/)
- [Go net/rpc Documentation](https://golang.org/pkg/net/rpc/)
- [Minikube Documentation](https://minikube.sigs.k8s.io/)
