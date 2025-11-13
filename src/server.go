package raft

import (
	"encoding/gob"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/rpc"
	"os"
	"sync"
	"time"
	"strings"
	"strconv"
)

// Command represents a key-value command in the Raft log
type Command struct {
	Key   string
	Value string
}

func init() {
	gob.Register(Command{})
}
// Helper type to implement net.Addr interface for static addresses
type staticAddr struct {
	addr string
}

func (sa *staticAddr) Network() string { return "tcp" }
func (sa *staticAddr) String() string  { return sa.addr }

// Wrapper for ConsensusModule and rpc.Server (to expose methods as endpoints)
// ,, Manages server peers
type Server struct {
	mu sync.Mutex

	serverId int
	peerIds  []int

	cm       *ConsensusModule
	storage  Storage
	rpcProxy *RPCProxy

	rpcServer *rpc.Server
	listener  net.Listener

	commitChan  chan<- CommitEntry
	peerClients map[int]*rpc.Client

	ready <-chan interface{}
	quit  chan interface{}
	wg    sync.WaitGroup
}

func NewServer(
	serverId int, peerIds []int, storage Storage, ready <-chan interface{}, commitChan chan<- CommitEntry) *Server {
	s := new(Server)
	
	// If running in Kubernetes, get pod ordinal from hostname
	if hostname := os.Getenv("HOSTNAME"); strings.HasPrefix(hostname, "raft-cluster-") {
		ordinal := strings.TrimPrefix(hostname, "raft-cluster-")
		if id, err := strconv.Atoi(ordinal); err == nil {
			serverId = id
			// Generate peer IDs based on StatefulSet size
			replicas := 3 // This should be configurable
			peerIds = make([]int, 0)
			for i := 0; i < replicas; i++ {
				if i != serverId {
					peerIds = append(peerIds, i)
				}
			}
		}
	}
	
	s.serverId = serverId
	s.peerIds = peerIds
	s.peerClients = make(map[int]*rpc.Client)
	s.storage = storage
	s.ready = ready
	s.commitChan = commitChan
	s.quit = make(chan interface{})
	return s
}

func (s *Server) Serve() {
	s.mu.Lock()
	s.cm = NewConsensusModule(s.serverId, s.peerIds, s, s.storage, s.ready, s.commitChan)

	// Create RPC server and register RPCProxy that forwards methods to cm
	s.rpcServer = rpc.NewServer()
	s.rpcProxy = &RPCProxy{
		cm:      s.cm,
		storage: s.storage,
	}
	s.rpcServer.RegisterName("ConsensusModule", s.rpcProxy)

	var err error
	// In Kubernetes use fixed port 8080, otherwise let OS assign port
	port := ":0"
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		port = ":8080"
	}
	s.listener, err = net.Listen("tcp", port)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("[%v] listening at %s", s.serverId, s.listener.Addr())
	s.mu.Unlock()

	// In Kubernetes, automatically connect to peers using StatefulSet DNS names
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		for _, peerId := range s.peerIds {
			peerAddr := fmt.Sprintf("raft-cluster-%d.raft-service:8080", peerId)
			log.Printf("[%v] attempting to connect to peer %d at %s", s.serverId, peerId, peerAddr)
			go func(id int, addr string) {
				for {
					err := s.ConnectToPeer(id, &staticAddr{addr})
					if err != nil {
						log.Printf("[%v] failed to connect to peer %d: %v", s.serverId, id, err)
						time.Sleep(1 * time.Second)
						continue
					}
					break
				}
			}(peerId, peerAddr)
		}
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		for {
			conn, err := s.listener.Accept()
			if err != nil {
				select {
				case <-s.quit:
					return
				default:
					log.Fatal("accept error:", err)
				}
			}
			s.wg.Add(1)
			go func() {
				s.rpcServer.ServeConn(conn)
				s.wg.Done()
			}()
		}
	}()
}

// Close all client connections to peers
func (s *Server) DisconnectAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id := range s.peerClients {
		if s.peerClients[id] != nil {
			s.peerClients[id].Close()
			s.peerClients[id] = nil
		}
	}
}

// Close server & shut down
func (s *Server) Shutdown() {
	s.cm.Stop()
	close(s.quit)
	s.listener.Close()
	s.wg.Wait()
}

func (s *Server) GetListenAddr() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listener.Addr()
}

func (s *Server) ConnectToPeer(peerId int, addr net.Addr) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.peerClients[peerId] == nil {
		var peerAddr string
		if addr != nil {
			peerAddr = addr.String()
		} else if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
			// In Kubernetes, use StatefulSet DNS names
			peerAddr = fmt.Sprintf("raft-cluster-%d.raft-service:8080", peerId)
		}
		
		if peerAddr == "" {
			return fmt.Errorf("no address available for peer %d", peerId)
		}
		
		client, err := rpc.Dial("tcp", peerAddr)
		if err != nil {
			return err
		}
		s.peerClients[peerId] = client
	}
	return nil
}

func (s *Server) DisconnectPeer(peerId int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.peerClients[peerId] != nil {
		err := s.peerClients[peerId].Close()
		s.peerClients[peerId] = nil
		return err
	}
	return nil
}

// Call service method to peer server
func (s *Server) Call(id int, serviceMethod string, args interface{}, reply interface{}) error {
	s.mu.Lock()
	peer := s.peerClients[id]
	s.mu.Unlock()

	if peer == nil {
		return fmt.Errorf("call client %d after it's closed", id)
	} else {
		return peer.Call(serviceMethod, args, reply)
	}
}

// Simulation of slight delay and unreliable connections
type RPCProxy struct {
	cm      *ConsensusModule
	storage Storage
}

// Report exposes ConsensusModule state
func (rpp *RPCProxy) Report(args struct{}, reply *struct{ Id, Term int; IsLeader bool }) error {
	reply.Id, reply.Term, reply.IsLeader = rpp.cm.Report()
	return nil
}

// Submit implements client command submission
func (rpp *RPCProxy) Submit(cmd Command, reply *bool) error {
	*reply = rpp.cm.Submit(cmd)
	return nil
}

// Get implements key-value storage read
func (rpp *RPCProxy) Get(key string, reply *struct{Value []byte; Found bool}) error {
	value, found := rpp.cm.storage.Get(key)
	reply.Value = value
	reply.Found = found
	return nil
}

func (rpp *RPCProxy) RequestVote(args RequestVoteArgs, reply *RequestVoteReply) error {
	if len(os.Getenv("RAFT_UNRELIABLE_RPC")) > 0 {
		dice := rand.Intn(10)
		if dice == 9 {
			rpp.cm.dlog("drop RequestVote")
			return fmt.Errorf("RPC failed")
		} else if dice == 8 {
			rpp.cm.dlog("delay RequestVote")
			time.Sleep(75 * time.Millisecond)
		}
	} else {
		time.Sleep(time.Duration(1+rand.Intn(5)) * time.Millisecond)
	}
	return rpp.cm.RequestVote(args, reply)
}

func (rpp *RPCProxy) AppendEntries(args AppendEntriesArgs, reply *AppendEntriesReply) error {
	if len(os.Getenv("RAFT_UNRELIABLE_RPC")) > 0 {
		dice := rand.Intn(10)
		if dice == 9 {
			rpp.cm.dlog("drop AppendEntries")
			return fmt.Errorf("RPC failed")
		} else if dice == 8 {
			rpp.cm.dlog("delay AppendEntries")
			time.Sleep(75 * time.Millisecond)
		}
	} else {
		time.Sleep(time.Duration(1+rand.Intn(5)) * time.Millisecond)
	}
	return rpp.cm.AppendEntries(args, reply)
}
