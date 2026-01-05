package server

import (
	"context"
	"log"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

const defaultUnaryTimeout = 20 * time.Second

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

	s := grpc.NewServer(
		grpc.UnaryInterceptor(func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
			start := time.Now()
			if _, ok := ctx.Deadline(); !ok {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, defaultUnaryTimeout)
				defer cancel()
			}
			resp, err := handler(ctx, req)
			elapsed := time.Since(start)
			if err != nil {
				log.Printf("grpc unary %s failed after %s: %v", info.FullMethod, elapsed, err)
			} else {
				log.Printf("grpc unary %s ok in %s", info.FullMethod, elapsed)
			}
			return resp, err
		}),
	)
	reflection.Register(s)

	return &GRPCServer{Server: s, Listener: ln}, nil
}

func (s *GRPCServer) Serve() error {
	return s.Server.Serve(s.Listener)
}
