package casambi

import (
	"fmt"
	"strings"
	"time"

	casambiv1 "github.com/joshp123/gohome/proto/gen/plugins/casambi/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"
)

// The bridge restarts on every deploy. gRPC's default reconnect backoff grows
// to two minutes, and calls fail fast meanwhile; cap it so reads resume within
// seconds of the bridge returning.
var reconnect = grpc.ConnectParams{
	Backoff:           backoff.Config{BaseDelay: time.Second, Multiplier: 1.6, Jitter: 0.2, MaxDelay: 5 * time.Second},
	MinConnectTimeout: 5 * time.Second,
}

// NewBridgeClient connects lazily; the tailnet (or loopback) is the security boundary.
func NewBridgeClient(address string) (casambiv1.CasambiServiceClient, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, fmt.Errorf("casambi bridge_address is required")
	}
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithConnectParams(reconnect))
	if err != nil {
		return nil, fmt.Errorf("casambi bridge %s: %w", address, err)
	}
	return casambiv1.NewCasambiServiceClient(conn), nil
}
