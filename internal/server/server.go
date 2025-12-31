package server

import (
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// GRPCServer wraps a gRPC server and listener.
type GRPCServer struct {
	Server   *grpc.Server
	Listener net.Listener
}

func NewGRPCServer(addr string) (*GRPCServer, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	s := grpc.NewServer()
	reflection.Register(s)

	return &GRPCServer{Server: s, Listener: ln}, nil
}

func (s *GRPCServer) Serve() error {
	return s.Server.Serve(s.Listener)
}
