package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/TEENet-io/prime-service/internal/generator"
	"github.com/TEENet-io/prime-service/internal/pool"
	"github.com/TEENet-io/prime-service/internal/server"
)

type Config struct {
	Server struct {
		Address string `json:"address"`
	} `json:"server"`
	Pool struct {
		MinPoolSize     int `json:"min_pool_size"`
		MaxPoolSize     int `json:"max_pool_size"`
		RefillThreshold int `json:"refill_threshold"`
		PrimeBitSize    int `json:"prime_bit_size"`
		MaxConcurrent   int `json:"max_concurrent"`
		PoolDir         string `json:"pool_dir"`
		AutoSave        bool   `json:"auto_save"`
		BackgroundGen   bool   `json:"background_gen"`
		RefillInterval  int    `json:"refill_interval"` // seconds
	} `json:"pool"`
	Logging struct {
		Level string `json:"level"`
	} `json:"logging"`
}

func loadConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var config Config
	if err := json.NewDecoder(file).Decode(&config); err != nil {
		return nil, err
	}

	// Set defaults if not specified
	if config.Server.Address == "" {
		config.Server.Address = ":50055"
	}
	if config.Pool.PoolDir == "" {
		config.Pool.PoolDir = "./prime_pool"
	}
	if config.Pool.MinPoolSize == 0 {
		config.Pool.MinPoolSize = 10
	}
	if config.Pool.MaxPoolSize == 0 {
		config.Pool.MaxPoolSize = 20
	}
	if config.Pool.RefillThreshold == 0 {
		config.Pool.RefillThreshold = 5
	}
	if config.Pool.PrimeBitSize == 0 {
		config.Pool.PrimeBitSize = 1024
	}
	if config.Pool.MaxConcurrent == 0 {
		config.Pool.MaxConcurrent = 2
	}
	if config.Pool.RefillInterval == 0 {
		config.Pool.RefillInterval = 30
	}

	return &config, nil
}

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "config.json", "Configuration file path")
	flag.Parse()

	// Load configuration
	config, err := loadConfig(configPath)
	if err != nil {
		log.Printf("Failed to load config file, using defaults: %v", err)
		// Use default config
		config = &Config{}
		config.Server.Address = ":50055"
		config.Pool.MinPoolSize = 10
		config.Pool.MaxPoolSize = 20
		config.Pool.RefillThreshold = 5
		config.Pool.PrimeBitSize = 1024
		config.Pool.MaxConcurrent = 2
		config.Pool.PoolDir = "./prime_pool"
		config.Pool.AutoSave = true
		config.Pool.BackgroundGen = true
		config.Pool.RefillInterval = 30
	}

	log.Printf("Starting with config: server=%s, pool_size=%d-%d, storage=%s",
		config.Server.Address, config.Pool.MinPoolSize, config.Pool.MaxPoolSize, config.Pool.PoolDir)

	// Initialize generator
	gen := generator.NewGenerator()

	// Initialize pool manager with config
	simpleConfig := pool.SimpleConfig{
		MinPoolSize:     config.Pool.MinPoolSize,
		MaxPoolSize:     config.Pool.MaxPoolSize,
		RefillThreshold: config.Pool.RefillThreshold,
		PrimeBitSize:    config.Pool.PrimeBitSize,
		MaxConcurrent:   config.Pool.MaxConcurrent,
		PoolDir:         config.Pool.PoolDir,
		AutoSave:        config.Pool.AutoSave,
		BackgroundGen:   config.Pool.BackgroundGen,
		RefillInterval:  time.Duration(config.Pool.RefillInterval) * time.Second,
	}
	poolManager := pool.NewManager(gen, simpleConfig)

	// Start pool manager
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := poolManager.Start(ctx); err != nil {
		log.Fatalf("Failed to start pool manager: %v", err)
	}
	defer poolManager.Stop()

	// Start gRPC server
	go func() {
		if err := server.StartGRPCServer(config.Server.Address, poolManager); err != nil {
			log.Fatalf("Failed to start gRPC server: %v", err)
		}
	}()

	log.Printf("Prime service started on %s", config.Server.Address)

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down prime service...")
	cancel() // Cancel context to stop background operations
}