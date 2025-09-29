package pool

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/TEENet-io/prime-service/internal/generator"
	"github.com/bnb-chain/tss-lib/v2/crypto/paillier"
)

// PreParamsData represents complete pre-computed parameters
type PreParamsData struct {
	PaillierKey *paillier.PrivateKey `json:"paillier_key"`
	NTildei     *big.Int             `json:"n_tildei"`
	H1i         *big.Int             `json:"h1i"`
	H2i         *big.Int             `json:"h2i"`
	Alpha       *big.Int             `json:"alpha"`
	Beta        *big.Int             `json:"beta"`
	P           *big.Int             `json:"p"` // safe prime for NTildei
	Q           *big.Int             `json:"q"` // safe prime for NTildei
	GeneratedAt time.Time            `json:"generated_at"`
}

// SimpleConfig contains configuration for the pool
type SimpleConfig struct {
	// Pool size limits
	MinPoolSize     int `json:"min_pool_size"`    // Minimum items to maintain in pool
	MaxPoolSize     int `json:"max_pool_size"`    // Maximum items in pool
	RefillThreshold int `json:"refill_threshold"` // When to start refilling

	// Generation settings
	PrimeBitSize    int `json:"prime_bit_size"`     // Bit size for safe primes (default: 1024)
	PaillierBitSize int `json:"paillier_bit_size"` // Bit size for Paillier modulus (default: 2048)
	MaxConcurrent   int `json:"max_concurrent"`   // Maximum concurrent parameter generation (default: 4)

	// Persistence
	PoolDir  string `json:"pool_dir"`  // Directory to store pool data
	AutoSave bool   `json:"auto_save"` // Auto save pool to disk

	// Background generation
	BackgroundGen  bool          `json:"background_gen"`  // Enable background generation
	RefillInterval time.Duration `json:"refill_interval"` // How often to check and refill
}

// Manager manages a pool of pre-generated cryptographic parameters
type Manager struct {
	mu        sync.RWMutex
	config    *SimpleConfig
	generator *generator.Generator

	// Pool storage
	preParams []*PreParamsData `json:"pre_params"`

	// Background generation
	stopCh       chan struct{}
	ticker       *time.Ticker
	tickerMu     sync.Mutex
	generatingMu sync.Mutex
	isGenerating bool

	// Save state
	savingMu sync.Mutex
	isSaving bool

	// File paths
	poolFilePath string

	// Startup delay
	startTime time.Time

	// Statistics
	totalGenerated int64
	totalServed    int64
}

// NewManager creates a new pool manager
func NewManager(gen *generator.Generator, config SimpleConfig) *Manager {
	// Set defaults
	if config.MinPoolSize == 0 {
		config.MinPoolSize = 10
	}
	if config.MaxPoolSize == 0 {
		config.MaxPoolSize = 20
	}
	if config.RefillThreshold == 0 {
		config.RefillThreshold = 5
	}
	if config.PrimeBitSize == 0 {
		config.PrimeBitSize = 1024
	}
	if config.PaillierBitSize == 0 {
		config.PaillierBitSize = 2048
	}
	if config.MaxConcurrent == 0 {
		config.MaxConcurrent = 4
	}
	if config.PoolDir == "" {
		config.PoolDir = "./prime_pool"
	}
	if config.RefillInterval == 0 {
		config.RefillInterval = 30 * time.Second
	}

	// Ensure pool directory exists
	os.MkdirAll(config.PoolDir, 0755)

	pool := &Manager{
		config:       &config,
		generator:    gen,
		preParams:    make([]*PreParamsData, 0),
		stopCh:       make(chan struct{}),
		poolFilePath: filepath.Join(config.PoolDir, "prime_pool.json"),
		startTime:    time.Now(),
	}

	// Load existing pool data
	pool.loadFromDisk()

	return pool
}

// Start starts the pool manager
func (m *Manager) Start(ctx context.Context) error {
	log.Println("Starting prime pool manager...")

	// Start background generation if enabled
	if m.config.BackgroundGen {
		go m.backgroundGeneration()
	}

	// Initial fill if pool is empty
	if len(m.preParams) < m.config.RefillThreshold {
		go m.refillPool()
	}

	return nil
}

// Stop stops the pool manager
func (m *Manager) Stop() {
	log.Println("Stopping prime pool manager")

	// Stop background generation
	close(m.stopCh)

	// Stop ticker
	m.tickerMu.Lock()
	if m.ticker != nil {
		m.ticker.Stop()
		m.ticker = nil
	}
	m.tickerMu.Unlock()

	// Save current state
	m.saveToDisk()
}

// GetPreParams retrieves and consumes pre-computed parameters from the pool
// count: number of parameters to retrieve (1 for single, >1 for batch)
func (m *Manager) GetPreParams(ctx context.Context, count uint32) ([]*PreParamsData, error) {
	m.mu.Lock()

	// Check if we need to refill the pool
	if len(m.preParams) <= m.config.RefillThreshold {
		log.Printf("Prime pool running low (size: %d), triggering generation", len(m.preParams))
		go m.refillPool()
	}

	// Default count to 1 if not specified
	if count == 0 {
		count = 1
	}

	result := make([]*PreParamsData, 0, count)

	// Get from pool what we can
	available := len(m.preParams)
	if available > 0 {
		take := int(count)
		if take > available {
			take = available
		}
		result = m.preParams[:take]
		m.preParams = m.preParams[take:]
		log.Printf("Retrieved %d pre-computed parameters from pool (remaining: %d)", take, len(m.preParams))
	}

	m.totalServed += int64(len(result))
	m.mu.Unlock()

	// Generate remaining if needed
	remaining := int(count) - len(result)
	if remaining > 0 {
		log.Printf("Prime pool insufficient, generating %d parameters synchronously (this may be slow)", remaining)
		for i := 0; i < remaining; i++ {
			params, err := m.generateSinglePreParams()
			if err != nil {
				return nil, fmt.Errorf("failed to generate params: %w", err)
			}
			result = append(result, params)
		}
		m.mu.Lock()
		m.totalServed += int64(remaining)
		m.mu.Unlock()
	}

	// Save updated pool if auto-save is enabled
	if m.config.AutoSave {
		go m.saveToDisk()
	}

	return result, nil
}

// GetPoolStatus returns current pool statistics
func (m *Manager) GetPoolStatus() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var oldestGenTime time.Time
	var newestGenTime time.Time

	if len(m.preParams) > 0 {
		oldestGenTime = m.preParams[0].GeneratedAt
		newestGenTime = m.preParams[len(m.preParams)-1].GeneratedAt
	}

	return map[string]interface{}{
		"pool_size":        len(m.preParams),
		"min_size":         m.config.MinPoolSize,
		"max_size":         m.config.MaxPoolSize,
		"refill_threshold": m.config.RefillThreshold,
		"is_generating":    m.isGenerating,
		"oldest_item":      oldestGenTime,
		"newest_item":      newestGenTime,
		"pool_file":        m.poolFilePath,
		"total_generated":  m.totalGenerated,
		"total_served":     m.totalServed,
	}
}

// generateSinglePreParams generates a single set of pre-computed parameters
func (m *Manager) generateSinglePreParams() (*PreParamsData, error) {
	start := time.Now()
	log.Println("Generating single pre-computed parameters")

	params, err := m.generator.GeneratePreParams(m.config.PrimeBitSize, m.config.PaillierBitSize)
	if err != nil {
		return nil, fmt.Errorf("failed to generate parameters: %w", err)
	}

	elapsed := time.Since(start)
	log.Printf("Generated single pre-computed parameters (duration: %s)", elapsed)

	m.totalGenerated++

	return &PreParamsData{
		PaillierKey: params.PaillierKey,
		NTildei:     params.NTildei,
		H1i:         params.H1i,
		H2i:         params.H2i,
		Alpha:       params.Alpha,
		Beta:        params.Beta,
		P:           params.P,
		Q:           params.Q,
		GeneratedAt: params.GeneratedAt,
	}, nil
}

// refillPool fills the pool to minimum size
func (m *Manager) refillPool() {
	// Check if still in startup delay period (10 seconds for testing)
	if time.Since(m.startTime) < 10*time.Second {
		log.Println("Skipping prime generation during startup delay")
		return
	}

	m.generatingMu.Lock()
	if m.isGenerating {
		m.generatingMu.Unlock()
		return
	}
	m.isGenerating = true
	m.generatingMu.Unlock()

	defer func() {
		m.generatingMu.Lock()
		m.isGenerating = false
		m.generatingMu.Unlock()
	}()

	m.mu.RLock()
	currentSize := len(m.preParams)
	m.mu.RUnlock()

	if currentSize >= m.config.MinPoolSize {
		return
	}

	needed := m.config.MinPoolSize - currentSize
	log.Printf("Starting pool refill (current: %d, needed: %d, min: %d)",
		currentSize, needed, m.config.MinPoolSize)

	start := time.Now()
	generated := 0

	// Use limited concurrent generation to avoid CPU overload
	maxConcurrent := m.config.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 1 // Default to single thread for CPU-limited systems
	}
	// For 3 CPU cores, limit to 1 concurrent generation to leave resources for other tasks
	if maxConcurrent > 1 && runtime.NumCPU() <= 3 {
		maxConcurrent = 1
		log.Println("Limiting prime generation to 1 concurrent worker for CPU-limited system")
	}
	if needed < maxConcurrent {
		maxConcurrent = needed
	}

	// Channel to collect generated parameters
	paramsCh := make(chan *PreParamsData, needed)
	errorCh := make(chan error, needed)

	// WaitGroup to track concurrent generation
	var genWg sync.WaitGroup

	// Start concurrent parameter generation with lower priority
	for i := 0; i < maxConcurrent; i++ {
		genWg.Add(1)
		go func() {
			defer genWg.Done()

			// Lower the priority of this goroutine to reduce impact on other tasks
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()

			for {
				select {
				case <-m.stopCh:
					return
				default:
				}

				// Check if we have enough parameters
				m.mu.RLock()
				currentSize := len(m.preParams)
				m.mu.RUnlock()

				if currentSize >= m.config.MinPoolSize {
					return // Pool has enough parameters
				}

				params, err := m.generateSinglePreParams()
				if err != nil {
					errorCh <- err
					return
				}

				// Add significant delay to minimize CPU impact
				// 1s delay ensures prime generation has minimal impact on other tasks
				time.Sleep(1 * time.Second)

				select {
				case paramsCh <- params:
				case <-m.stopCh:
					return
				}
			}
		}()
	}

	// Goroutine to close channels when generation is done
	go func() {
		genWg.Wait()
		close(paramsCh)
		close(errorCh)
	}()

	// Collect generated parameters
	for {
		select {
		case <-m.stopCh:
			log.Println("Pool generation stopped")
			return
		case err := <-errorCh:
			if err != nil {
				log.Printf("Failed to generate parameters during concurrent refill: %v", err)
				return // Stop generation on error
			}
		case preParamsData, ok := <-paramsCh:
			if !ok {
				// Channel closed, generation complete
				goto done
			}

			m.mu.Lock()
			if len(m.preParams) < m.config.MaxPoolSize {
				m.preParams = append(m.preParams, preParamsData)
				generated++
				currentSize := len(m.preParams)
				m.mu.Unlock()

				log.Printf("Generated parameter set %d/%d (pool size: %d)", generated, needed, currentSize)

				if m.config.AutoSave {
					go m.saveToDisk()
				}

				// Continue collecting until all goroutines are done
			} else {
				m.mu.Unlock()
				log.Println("Pool reached max capacity, discarding extra parameter")
			}
		}
	}

done:
	elapsed := time.Since(start)
	log.Printf("Pool refill completed (generated: %d, duration: %s, avg: %s)",
		generated, elapsed, elapsed/time.Duration(generated))

	// Save updated pool
	if m.config.AutoSave {
		m.saveToDisk()
	}
}

// backgroundGeneration runs periodic pool maintenance
func (m *Manager) backgroundGeneration() {
	m.tickerMu.Lock()
	m.ticker = time.NewTicker(m.config.RefillInterval)
	m.tickerMu.Unlock()

	defer func() {
		m.tickerMu.Lock()
		if m.ticker != nil {
			m.ticker.Stop()
			m.ticker = nil
		}
		m.tickerMu.Unlock()
	}()

	log.Printf("Started background prime generation (interval: %s)", m.config.RefillInterval)

	for {
		select {
		case <-m.ticker.C:
			m.mu.RLock()
			currentSize := len(m.preParams)
			m.mu.RUnlock()

			if currentSize <= m.config.RefillThreshold {
				log.Printf("Background refill triggered (pool size: %d)", currentSize)
				m.refillPool()
			}

		case <-m.stopCh:
			log.Println("Background generation stopped")
			return
		}
	}
}

// saveToDisk saves the pool to disk
func (m *Manager) saveToDisk() {
	m.savingMu.Lock()
	if m.isSaving {
		m.savingMu.Unlock()
		return // Already saving, skip this call
	}
	m.isSaving = true
	m.savingMu.Unlock()

	defer func() {
		m.savingMu.Lock()
		m.isSaving = false
		m.savingMu.Unlock()
	}()

	m.mu.RLock()
	defer m.mu.RUnlock()

	data := struct {
		PreParams []*PreParamsData `json:"pre_params"`
		SavedAt   time.Time        `json:"saved_at"`
		Config    *SimpleConfig    `json:"config"`
	}{
		PreParams: m.preParams,
		SavedAt:   time.Now(),
		Config:    m.config,
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		log.Printf("Failed to marshal pool data: %v", err)
		return
	}

	if err := ioutil.WriteFile(m.poolFilePath, jsonData, 0600); err != nil {
		log.Printf("Failed to save pool to disk: %v", err)
		return
	}

	log.Printf("Pool saved to disk (file: %s, size: %d)", m.poolFilePath, len(m.preParams))
}

// loadFromDisk loads the pool from disk
func (m *Manager) loadFromDisk() {
	if _, err := os.Stat(m.poolFilePath); os.IsNotExist(err) {
		log.Printf("Pool file does not exist, starting with empty pool: %s", m.poolFilePath)
		return
	}

	data, err := ioutil.ReadFile(m.poolFilePath)
	if err != nil {
		log.Printf("Failed to read pool file: %v", err)
		return
	}

	var poolData struct {
		PreParams []*PreParamsData `json:"pre_params"`
		SavedAt   time.Time        `json:"saved_at"`
		Config    *SimpleConfig    `json:"config"`
	}

	if err := json.Unmarshal(data, &poolData); err != nil {
		log.Printf("Failed to unmarshal pool data: %v", err)
		return
	}

	m.preParams = poolData.PreParams
	if m.preParams == nil {
		m.preParams = make([]*PreParamsData, 0)
	}

	// Remove any nil entries (data corruption protection)
	validParams := make([]*PreParamsData, 0, len(m.preParams))
	for _, param := range m.preParams {
		if param != nil && param.PaillierKey != nil {
			validParams = append(validParams, param)
		}
	}
	m.preParams = validParams

	log.Printf("Pool loaded from disk (file: %s, size: %d, saved: %s)",
		m.poolFilePath, len(m.preParams), poolData.SavedAt)
}
