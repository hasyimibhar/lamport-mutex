package main

import (
	"context"
	"log"
	"net"
	"sync/atomic"
	"bufio"
	"os"
	"fmt"
	"strings"
	"sort"
	"sync"
	"hash/fnv"
	"errors"

	"google.golang.org/grpc"
	pb "github.com/hasyimibhar/lamport-mutex/mutex"
)

type LockRequest struct {
	NodeID uint64
	T uint64
}

type Server struct {
	pb.UnimplementedNodeServer

	t uint64
	addr string
	id uint64
	server *grpc.Server
	clients []*NodeClient

	requestsMutex sync.Mutex
	requests []LockRequest

	lockCv *sync.Cond
}

func NewServer(addr string, clientAddrs []string) (*Server, error) {
	clients := make([]*NodeClient, len(clientAddrs))
	for i, addr := range clientAddrs {
		client, err := NewNodeClient(addr)
		if err != nil {
			return nil, err
		}

		clients[i] = client
	}

	h := fnv.New64a()
        h.Write([]byte(addr))

	server := grpc.NewServer()
	s := &Server{
		id: h.Sum64(),
		addr: addr,
		clients: clients,
		server: server,
		requests: []LockRequest{},
	}
	s.lockCv = sync.NewCond(&s.requestsMutex)

	pb.RegisterNodeServer(server, s)

	return s, nil
}

func (s *Server) Run() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
		return err
	}

	if err := s.server.Serve(ln); err != nil {
		log.Fatalf("failed to serve: %v", err)
		return err
	}

	return nil
}

func (s *Server) addRequest(req LockRequest) {
	s.requestsMutex.Lock()
	defer s.requestsMutex.Unlock()

	s.requests = append(s.requests, req)
	sort.Sort(byTotalOrder(s.requests))
}

func (s *Server) hasLock() bool {
	s.requestsMutex.Lock()
	defer s.requestsMutex.Unlock()

	if len(s.requests) == 0 {
		return false
	}

	return s.requests[0].NodeID == s.ID()
}

func (s *Server) ConnectTo(addr string) error {
	client, err := NewNodeClient(addr)
	if err != nil {
		return err
	}

	s.clients = append(s.clients, client)
	return nil
}

func (s *Server) TryLock() error {
	if s.hasLock() {
		return errors.New("you already have the lock")
	}

	s.requestsMutex.Lock()
	for i := 0; i < len(s.requests); i++ {
		if s.requests[i].NodeID == s.ID() {
			return errors.New("you have already requested for the lock")
		}
	}
	s.requestsMutex.Unlock()

	req := LockRequest{NodeID: s.ID(), T: s.T()}
	s.addRequest(req)

	for _, client := range s.clients {
		if err := client.Lock(req); err != nil {
			return err
		}
	}

	// Wait until I have the lock
	s.requestsMutex.Lock()
	for s.requests[0].NodeID != s.ID() {
		s.lockCv.Wait()
	}
	s.requestsMutex.Unlock()

	return nil
}

func (s *Server) TryUnlock() error {
	if !s.hasLock() {
		return errors.New("you don't have the lock")
	}

	s.requestsMutex.Lock()
	s.requests = s.requests[1:]
	s.requestsMutex.Unlock()

	for _, client := range s.clients {
		if err := client.Unlock(s); err != nil {
			return err
		}
	}

	return nil
}

func (s *Server) Lock(ctx context.Context, request *pb.LockRequest) (*pb.LockResponse, error) {
	//fmt.Printf("[trace] received lock request: t=%d, nodeId=%d\n", request.T, request.NodeId)
	atomic.AddUint64(&s.t, request.T)

	s.addRequest(LockRequest{T: request.T, NodeID: request.NodeId})

	response := &pb.LockResponse{T: s.T(), NodeId: s.ID()}
	return response, nil
}

func (s *Server) Unlock(ctx context.Context, request *pb.UnlockRequest) (*pb.UnlockResponse, error) {
	//fmt.Printf("[trace] received unlock request: t=%d, nodeId=%d\n", request.T, request.NodeId)
	atomic.AddUint64(&s.t, request.T)

	s.requestsMutex.Lock()
	for i := 0; i < len(s.requests); i++ {
		if s.requests[i].NodeID == request.NodeId {
			s.requests = append(s.requests[:i], s.requests[i+1:]...)
			break
		}
	}
	s.requestsMutex.Unlock()

	s.lockCv.Broadcast()

	response := &pb.UnlockResponse{T: s.T(), NodeId: s.ID()}
	return response, nil
}

func (s *Server) Addr() string {
	return s.addr
}

func (s *Server) ID() uint64 {
	return s.id
}

func (s *Server) T() uint64 {
	return atomic.AddUint64(&s.t, 1)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: go run . <addr>")
		os.Exit(1)
	}

	port := os.Args[1]

	clients := []string{}
	if len(os.Args) > 2 {
		clients = os.Args[2:]
	}

	server, err := NewServer(port, clients)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("listening to", server.Addr())
	go func() {
		if err := server.Run(); err != nil {
			log.Fatal(err)
		}
	}()

	reader := bufio.NewReader(os.Stdin)

CommandLoop:
	for {
		fmt.Print("> ")
		cmd, _ := reader.ReadString('\n')
		cmd = strings.TrimSpace(cmd)
		tokens := strings.Split(cmd, " ")

		switch tokens[0] {
		case "connect":
			if len(tokens) < 2 {
				fmt.Println("usage: connect <addr>")
				goto CommandLoop
			}

			if err := server.ConnectTo(tokens[1]); err != nil {
				fmt.Println("error:", err)
				goto CommandLoop
			}

			fmt.Println("connected to", tokens[1])


		case "lock":
			if err := server.TryLock(); err != nil {
				fmt.Println("error:", err)
				goto CommandLoop
			}

			fmt.Println("lock acquired")

		case "unlock":
			if err := server.TryUnlock(); err != nil {
				fmt.Println("error:", err)
				goto CommandLoop
			}

			fmt.Println("lock released")

		default:
			fmt.Println("unknown command")
		}
	}
}

type byTotalOrder []LockRequest

func (s byTotalOrder) Len() int {
	return len(s)
}

func (s byTotalOrder) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s byTotalOrder) Less(i, j int) bool {
	if s[i].T < s[j].T {
		return true
	} else if s[i].T > s[j].T {
		return false
	}

	return s[i].NodeID < s[j].NodeID
}
