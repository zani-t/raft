package raft

import (
	"net"
	"net/rpc"
	"sync"
)

type Server struct {
	mu sync.Mutex

	serverId int
	peerIds  []int

	cm       *ConsensusModule
	rpcProxy *RPCProxy

	rpcServer *rpc.Server
	listener  net.Listener

	peerClients map[int]*rpc.Client

	ready <-chan interface{}
	quit  chan interface{}
	wg    sync.WaitGroup
}

func (s *Server) Call(id int, serviceMethod string, args interface{}, reply interface{}) error {
	return nil
}

type RPCProxy struct {
	cm *ConsensusModule
}
