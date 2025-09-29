package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	pb "github.com/TEENet-io/prime-service/proto"
	"github.com/TEENet-io/prime-service/internal/pool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	pb.UnimplementedPrimeServiceServer
	poolManager *pool.Manager
	startTime   time.Time
}

func NewServer(poolManager *pool.Manager) *Server {
	return &Server{
		poolManager: poolManager,
		startTime:   time.Now(),
	}
}

// GetPreParams returns PreParamsData for ECDSA DKG (single or batch)
func (s *Server) GetPreParams(ctx context.Context, req *pb.GetPreParamsRequest) (*pb.GetPreParamsResponse, error) {
	start := time.Now()

	// Default to 1 if count not specified
	count := req.Count
	if count == 0 {
		count = 1
	}

	// Validate count
	if count > 100 {
		return nil, status.Errorf(codes.InvalidArgument, "count must be between 1 and 100")
	}

	// Get parameters from pool manager
	paramsList, err := s.poolManager.GetPreParams(ctx, count)
	if err != nil {
		log.Printf("Failed to get pre-params: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to get pre-params: %v", err)
	}

	// Convert to protobuf format
	pbParams := make([]*pb.PreParamsData, len(paramsList))
	for i, params := range paramsList {
		pbParams[i] = &pb.PreParamsData{
			PaillierP:       params.PaillierKey.P.Bytes(),
			PaillierQ:       params.PaillierKey.Q.Bytes(),
			PaillierN:       params.PaillierKey.N.Bytes(),
			PaillierPhiN:    params.PaillierKey.PhiN.Bytes(),
			PaillierLambdaN: params.PaillierKey.LambdaN.Bytes(),
			NTildei:         params.NTildei.Bytes(),
			H1I:             params.H1i.Bytes(),
			H2I:             params.H2i.Bytes(),
			Alpha:           params.Alpha.Bytes(),
			Beta:            params.Beta.Bytes(),
			P:               params.P.Bytes(),
			Q:               params.Q.Bytes(),
			GeneratedAt:     params.GeneratedAt.Unix(),
		}
	}

	return &pb.GetPreParamsResponse{
		Params:           pbParams,
		GenerationTimeMs: time.Since(start).Milliseconds(),
	}, nil
}


func (s *Server) HealthCheck(ctx context.Context, req *pb.Empty) (*pb.HealthStatus, error) {
	uptime := time.Since(s.startTime).Seconds()

	return &pb.HealthStatus{
		Healthy:        true,
		Message:        "Prime service is running",
		UptimeSeconds:  int64(uptime),
	}, nil
}

func (s *Server) GetPoolStatus(ctx context.Context, req *pb.Empty) (*pb.PoolStatus, error) {
	status := s.poolManager.GetPoolStatus()

	// Create pools map
	pools := make(map[string]*pb.PoolInfo)

	// For the new structure, we have a single pool
	poolSize := uint32(0)
	if v, ok := status["pool_size"].(int); ok {
		poolSize = uint32(v)
	}

	minSize := uint32(10)
	if v, ok := status["min_size"].(int); ok {
		minSize = uint32(v)
	}

	// maxSize is available but not used in PoolInfo (we use minSize as TargetSize)
	// maxSize := uint32(20)
	// if v, ok := status["max_size"].(int); ok {
	// 	maxSize = uint32(v)
	// }

	isGenerating := false
	if v, ok := status["is_generating"].(bool); ok {
		isGenerating = v
	}

	generatingCount := uint32(0)
	if isGenerating {
		generatingCount = 1 // Simplified: we don't track exact count
	}

	// Add the single pool entry
	pools["1024_true"] = &pb.PoolInfo{
		Bits:           1024,
		SafePrime:      true,
		Available:      poolSize,
		TargetSize:     minSize,
		Generating:     generatingCount,
		LastRefillTime: 0, // Not tracked
	}

	// Safely get numeric values with defaults
	totalGenerated := int64(0)
	if v, ok := status["total_generated"].(int64); ok {
		totalGenerated = v
	}

	totalServed := int64(0)
	if v, ok := status["total_served"].(int64); ok {
		totalServed = v
	}

	return &pb.PoolStatus{
		Pools:          pools,
		TotalGenerated: totalGenerated,
		TotalServed:    totalServed,
		GenerationRate: 0, // Not calculated in new structure
	}, nil
}

func StartGRPCServer(addr string, poolManager *pool.Manager) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	grpcServer := grpc.NewServer()
	server := NewServer(poolManager)
	pb.RegisterPrimeServiceServer(grpcServer, server)

	log.Printf("Starting gRPC server on %s", addr)
	return grpcServer.Serve(lis)
}