package main

import (
	"fmt"
	"log"
	"net/rpc"
	"os"
	"time"
)

// RaftClient wraps RPC client with helper methods
type RaftClient struct {
	client *rpc.Client
	addr   string
}

func NewRaftClient(addr string) (*RaftClient, error) {
	client, err := rpc.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %v", addr, err)
	}
	return &RaftClient{client: client, addr: addr}, nil
}

func (rc *RaftClient) Close() {
	rc.client.Close()
}

// Get node status (term, isLeader, etc)
func (rc *RaftClient) GetStatus() (int, int, bool, error) {
	var reply struct {
		Id       int
		Term     int
		IsLeader bool
	}
	err := rc.client.Call("ConsensusModule.Report", struct{}{}, &reply)
	if err != nil {
		return 0, 0, false, err
	}
	return reply.Id, reply.Term, reply.IsLeader, nil
}

// Submit a command to the Raft cluster
func (rc *RaftClient) Submit(key string, value string) error {
	cmd := struct{ Key, Value string }{key, value}
	var reply bool
	err := rc.client.Call("ConsensusModule.Submit", cmd, &reply)
	if err != nil {
		return fmt.Errorf("submit failed: %v", err)
	}
	if !reply {
		return fmt.Errorf("not the leader")
	}
	return nil
}

// Read a value from storage
func (rc *RaftClient) Read(key string) (string, bool, error) {
	var reply struct {
		Value []byte
		Found bool
	}
	err := rc.client.Call("ConsensusModule.Get", key, &reply)
	if err != nil {
		return "", false, fmt.Errorf("read failed: %v", err)
	}
	return string(reply.Value), reply.Found, nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("Usage: %s <operation> [args...]\n", os.Args[0])
		fmt.Println("Operations:")
		fmt.Println("  status <node_addr>           - Get node status")
		fmt.Println("  write <node_addr> key value  - Write key-value to node")
		fmt.Println("  read <node_addr> key         - Read key from node")
		fmt.Println("  test-consensus               - Run consensus test across nodes")
		os.Exit(1)
	}

	operation := os.Args[1]

	switch operation {
	case "status":
		if len(os.Args) != 3 {
			log.Fatal("Usage: client status <node_addr>")
		}
		addr := os.Args[2]
		client, err := NewRaftClient(addr)
		if err != nil {
			log.Fatal(err)
		}
		defer client.Close()

		id, term, isLeader, err := client.GetStatus()
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Node %d (at %s):\n", id, addr)
		fmt.Printf("  Term: %d\n", term)
		fmt.Printf("  Is Leader: %v\n", isLeader)

	case "write":
		if len(os.Args) != 5 {
			log.Fatal("Usage: client write <node_addr> <key> <value>")
		}
		addr, key, value := os.Args[2], os.Args[3], os.Args[4]
		client, err := NewRaftClient(addr)
		if err != nil {
			log.Fatal(err)
		}
		defer client.Close()

		err = client.Submit(key, value)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Successfully wrote %s=%s to node at %s\n", key, value, addr)

	case "read":
		if len(os.Args) != 4 {
			log.Fatal("Usage: client read <node_addr> <key>")
		}
		addr, key := os.Args[2], os.Args[3]
		client, err := NewRaftClient(addr)
		if err != nil {
			log.Fatal(err)
		}
		defer client.Close()

		value, found, err := client.Read(key)
		if err != nil {
			log.Fatal(err)
		}
		if !found {
			fmt.Printf("Key %s not found\n", key)
		} else {
			fmt.Printf("Value for key %s: %s\n", key, value)
		}

	case "test-consensus":
		// Test writing to one node and reading from another
		writeAddr := "localhost:8082"  // port-forward to raft-cluster-0
		readAddr := "localhost:8081"   // port-forward to raft-cluster-1
		
		// Connect to write node
		writeClient, err := NewRaftClient(writeAddr)
		if err != nil {
			log.Fatal(err)
		}
		defer writeClient.Close()

		// Write test data
		testKey := fmt.Sprintf("test-key-%d", time.Now().Unix())
		testValue := "test-value"
		
		fmt.Printf("Writing %s=%s to %s\n", testKey, testValue, writeAddr)
		err = writeClient.Submit(testKey, testValue)
		if err != nil {
			log.Fatal(err)
		}

		// Wait a bit for replication
		time.Sleep(2 * time.Second)

		// Connect to read node
		readClient, err := NewRaftClient(readAddr)
		if err != nil {
			log.Fatal(err)
		}
		defer readClient.Close()

		// Read and verify
		fmt.Printf("Reading %s from %s\n", testKey, readAddr)
		value, found, err := readClient.Read(testKey)
		if err != nil {
			log.Fatal(err)
		}

		if !found {
			fmt.Printf("❌ Consensus test failed: Key %s not found on second node\n", testKey)
			os.Exit(1)
		}
		if value != testValue {
			fmt.Printf("❌ Consensus test failed: Expected %s, got %s\n", testValue, value)
			os.Exit(1)
		}
		fmt.Printf("✅ Consensus test passed! Value successfully replicated across nodes\n")
	}
}