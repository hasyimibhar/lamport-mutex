package main

import (
	"context"
	"time"

	"google.golang.org/grpc"
	pb "github.com/hasyimibhar/lamport-mutex/mutex"
)

type NodeClient struct {
	conn *grpc.ClientConn
	client pb.NodeClient
}

func NewNodeClient(addr string) (*NodeClient, error) {
	conn, err := grpc.Dial(addr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, err
	}

	return &NodeClient{
		conn: conn,
		client: pb.NewNodeClient(conn),
	}, nil
}

func (n *NodeClient) Lock(req LockRequest) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := n.client.Lock(ctx, &pb.LockRequest{T: req.T, NodeId: req.NodeID})
	if err != nil {
		return err
	}

	return nil
}

func (n *NodeClient) Unlock(s *Server) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := n.client.Unlock(ctx, &pb.UnlockRequest{T: s.T(), NodeId: s.ID()})
	if err != nil {
		return err
	}

	return nil
}
