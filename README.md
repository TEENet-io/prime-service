# Prime Service

A dedicated service for generating and caching pre-computed cryptographic parameters for TEE-DAO ECDSA key management system.

## Overview

This service pre-generates complete `PreParamsData` for ECDSA Distributed Key Generation (DKG), including:
- Paillier encryption keys (P, Q, N, PhiN, LambdaN)
- NTildei and its safe prime components
- Quadratic residues (H1i, H2i)
- Random values (Alpha, Beta)

Each parameter set takes 30-60 seconds to generate due to the need for 4 safe primes (2 for Paillier, 2 for NTildei). This service runs on dedicated high-performance machines to offload this expensive computation from TEE nodes.

## Features

- **Complete PreParamsData Generation**: All parameters needed for ECDSA DKG
- **Pre-computed Parameter Pool**: Maintains a pool of ready-to-use parameters
- **Background Generation**: Continuously generates new parameters to maintain pool
- **File Persistence**: Pool is saved to disk and restored on restart
- **Single & Batch Retrieval**: Get one or multiple parameter sets in a single call
- **Configurable Pool Size**: Adjust pool size based on your needs
- **Health Monitoring**: Check service status and pool statistics

## Quick Start

### 1. Build

```bash
go build -o server cmd/server/main.go
```

### 2. Configure

Edit `config.json` to set pool parameters:

```json
{
  "server": ":50055",
  "pool": {
    "min_pool_size": 20,
    "max_pool_size": 40,
    "refill_threshold": 10,
    "max_concurrent": 2
  }
}
```

For 5-node setup, use the optimized config:
```bash
./server -config-path config_optimized.json
```

### 3. Run

```bash
# Start the service
./server

# Or with custom config
./server -config-path config_optimized.json
```

## Client Usage

### Go Client

```go
import "github.com/TEENet-io/prime-service/client"

// Create client
c, err := client.NewClient("localhost:50055")
if err != nil {
    log.Fatal(err)
}
defer c.Close()

// Get single PreParamsData
params, err := c.GetPreParams(context.Background(), 1)

// Get batch for 5-node DKG
batch, err := c.GetPreParams(context.Background(), 5)
```

### Integration with TEE-DAO

1. Update TEE-DAO configuration (`config_global.json`):
```json
{
  "task_config": {
    "prime_pool": {
      "mode": "remote",
      "remote_service_addr": "localhost:50055"
    }
  }
}
```

2. The system will automatically use the remote prime service for ECDSA DKG.

## API

### gRPC Service (Port 50055)

- `GetPreParams(GetPreParamsRequest)`: Get one or more PreParamsData
  - `count`: Number of parameters to retrieve (default: 1)
- `HealthCheck()`: Check service health
- `GetPoolStatus()`: Get pool statistics

## Performance

- **Generation Time**: 30-60 seconds per PreParamsData
- **Service Response**: <10ms (from pool)
- **Pool Capacity**: 20-40 parameters (configurable)
- **Memory Usage**: ~100-200MB

### Recommended Configuration

For 4-core server with 5 TEE nodes:
- `min_pool_size`: 20 (supports 4 DKG rounds)
- `max_pool_size`: 40
- `max_concurrent`: 2 (for 4 cores)
- `refill_threshold`: 10

## Architecture

```
┌─────────────┐      gRPC        ┌──────────────┐
│  TEE Nodes  │ ───────────────> │ Prime Service │
│   (5 nodes) │ <─────────────── │              │
│             │   PreParamsData   │  High-Perf   │
└─────────────┘                  │    Server    │
                                 └──────────────┘
                                        │
                                        ▼
                                 ┌──────────────┐
                                 │  Parameter   │
                                 │     Pool     │
                                 │  (20-40 sets)│
                                 └──────────────┘
                                        │
                                        ▼
                                 ┌──────────────┐
                                 │  Background  │
                                 │  Generation  │
                                 │  (2 workers) │
                                 └──────────────┘
```

## Docker Deployment

```bash
# Build image
docker build -t prime-service .

# Run container
docker run -d \
  -p 50055:50055 \
  -v $(pwd)/prime_pool:/app/prime_pool \
  --name prime-service \
  prime-service
```

## Monitoring

Check pool status:
```bash
grpcurl -plaintext localhost:50055 prime.PrimeService/GetPoolStatus
```

Metrics available:
- `total_generated`: Total parameters generated
- `total_served`: Total parameters served
- `pool_size`: Current pool size
- `generating`: Parameters currently being generated

## Security Considerations

1. **Parameter Uniqueness**: Each PreParamsData is unique with negligible collision probability
2. **No Reuse**: Parameters are consumed from pool, not reused
3. **TLS Support**: Can configure mutual TLS for production
4. **Access Control**: Can add authentication for client connections

## Troubleshooting

### Slow Generation
- Increase CPU cores
- Check CPU throttling
- Adjust `max_concurrent` based on CPU count

### Pool Empty
- Increase `min_pool_size`
- Start service earlier to pre-generate
- Check generation errors in logs

### High Memory Usage
- Reduce `max_pool_size`
- Check for memory leaks
- Monitor with `pprof`

## License

Apache 2.0