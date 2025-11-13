package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/zani-t/raft/src"
)

func main() {
	// Create storage
	storage := raft.NewMapStorage()

	// Create channels
	ready := make(chan interface{})
	commit := make(chan raft.CommitEntry)

	// Create server with default values - these will be overridden if running in K8s
	server := raft.NewServer(0, []int{1, 2}, storage, ready, commit)

	// Start goroutine to apply committed entries to storage
	go func() {
		for entry := range commit {
			if cmd, ok := entry.Command.(raft.Command); ok {
				storage.Set(cmd.Key, []byte(cmd.Value))
				log.Printf("Applied command: key=%s, value=%s", cmd.Key, cmd.Value)
			}
		}
	}()

	// Start serving
	server.Serve()

	// Signal ready
	close(ready)

	// Handle shutdown gracefully
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
	server.Shutdown()
}