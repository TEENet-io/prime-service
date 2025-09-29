package client

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/bnb-chain/tss-lib/v2/crypto/paillier"
	pb "github.com/TEENet-io/prime-service/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// PrimeServiceClient wraps the gRPC client for the prime service
type PrimeServiceClient struct {
	conn   *grpc.ClientConn
	client pb.PrimeServiceClient
}

// NewClient creates a new prime service client
func NewClient(address string) (*PrimeServiceClient, error) {
	conn, err := grpc.NewClient(address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	return &PrimeServiceClient{
		conn:   conn,
		client: pb.NewPrimeServiceClient(conn),
	}, nil
}

// Close closes the client connection
func (c *PrimeServiceClient) Close() error {
	return c.conn.Close()
}

// GetPreParams retrieves PreParamsData from the service
// count: number of parameters to retrieve (default 1 if 0)
func (c *PrimeServiceClient) GetPreParams(ctx context.Context, count uint32) ([]*PreParamsData, error) {
	if count == 0 {
		count = 1 // Default to 1 if not specified
	}

	resp, err := c.client.GetPreParams(ctx, &pb.GetPreParamsRequest{
		Count: count,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get pre-params: %w", err)
	}

	if len(resp.Params) == 0 {
		return nil, fmt.Errorf("no parameters returned from service")
	}

	// Convert from protobuf to internal format
	result := make([]*PreParamsData, len(resp.Params))
	for i, params := range resp.Params {
		result[i] = &PreParamsData{
			PaillierKey: &paillier.PrivateKey{
				PublicKey: paillier.PublicKey{
					N: new(big.Int).SetBytes(params.PaillierN),
				},
				LambdaN: new(big.Int).SetBytes(params.PaillierLambdaN),
				PhiN:    new(big.Int).SetBytes(params.PaillierPhiN),
				P:       new(big.Int).SetBytes(params.PaillierP),
				Q:       new(big.Int).SetBytes(params.PaillierQ),
			},
			NTildei: new(big.Int).SetBytes(params.NTildei),
			H1i:     new(big.Int).SetBytes(params.H1I),
			H2i:     new(big.Int).SetBytes(params.H2I),
			Alpha:   new(big.Int).SetBytes(params.Alpha),
			Beta:    new(big.Int).SetBytes(params.Beta),
			P:       new(big.Int).SetBytes(params.P),
			Q:       new(big.Int).SetBytes(params.Q),
			GeneratedAt: time.Unix(params.GeneratedAt, 0),
		}
	}

	return result, nil
}

// GetPoolStatus gets the current pool status
func (c *PrimeServiceClient) GetPoolStatus(ctx context.Context) (*pb.PoolStatus, error) {
	return c.client.GetPoolStatus(ctx, &pb.Empty{})
}