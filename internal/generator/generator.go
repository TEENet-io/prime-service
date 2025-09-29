package generator

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/bnb-chain/tss-lib/v2/common"
	"github.com/bnb-chain/tss-lib/v2/crypto/paillier"
	"github.com/bnb-chain/tss-lib/v2/ecdsa/keygen"
)

type Generator struct {
	mu              sync.Mutex
	generationCount int64
	totalTime       time.Duration
}

// PreParamsData represents complete pre-computed parameters for ECDSA DKG
// This matches exactly with TEE DAO's PreParamsData structure
type PreParamsData struct {
	PaillierKey *paillier.PrivateKey `json:"paillier_key"`
	NTildei     *big.Int             `json:"n_tildei"`
	H1i         *big.Int             `json:"h1i"`
	H2i         *big.Int             `json:"h2i"`
	Alpha       *big.Int             `json:"alpha"`
	Beta        *big.Int             `json:"beta"`
	P           *big.Int             `json:"p"` // safe prime
	Q           *big.Int             `json:"q"` // safe prime
	GeneratedAt time.Time            `json:"generated_at"`
}

func NewGenerator() *Generator {
	return &Generator{}
}

// GeneratePreParams generates complete pre-computed parameters for ECDSA DKG
// This is the exact implementation from TEE DAO's generateSinglePreParams
func (g *Generator) GeneratePreParams(primeBitSize, paillierBitSize int) (*PreParamsData, error) {
	start := time.Now()
	defer func() {
		g.mu.Lock()
		g.generationCount++
		g.totalTime += time.Since(start)
		g.mu.Unlock()
	}()

	// Generate Paillier key pair (exact same as TEE DAO)
	ctx1, cancel1 := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel1()

	paillierSK, _, err := paillier.GenerateKeyPair(ctx1, rand.Reader, paillierBitSize, 4)
	if err != nil {
		return nil, fmt.Errorf("failed to generate Paillier key: %w", err)
	}

	// Generate safe primes for NTildei (exact same as TEE DAO)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel2()

	sgps, err := common.GetRandomSafePrimesConcurrent(ctx2, primeBitSize, 2, 4, rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate safe primes: %w", err)
	}

	// Calculate NTildei from the safe primes
	nTildei := new(big.Int).Mul(sgps[0].SafePrime(), sgps[1].SafePrime())

	// Generate h1, h2 in Z*_NTildei (exact same as TEE DAO)
	primeP, primeQ := sgps[0].Prime(), sgps[1].Prime()
	modPQ := common.ModInt(new(big.Int).Mul(primeP, primeQ))
	modNTildeI := common.ModInt(nTildei)

	f1 := common.GetRandomPositiveRelativelyPrimeInt(rand.Reader, nTildei)
	alpha := common.GetRandomPositiveRelativelyPrimeInt(rand.Reader, nTildei)
	beta := modPQ.ModInverse(alpha)
	h1 := modNTildeI.Mul(f1, f1)
	h2 := modNTildeI.Exp(h1, alpha)

	return &PreParamsData{
		PaillierKey: paillierSK,
		NTildei:     nTildei,
		H1i:         h1,
		H2i:         h2,
		Alpha:       alpha,
		Beta:        beta,
		P:           primeP,
		Q:           primeQ,
		GeneratedAt: time.Now(),
	}, nil
}

// ConvertToLocalPreParams converts PreParamsData to keygen.LocalPreParams
// This is for compatibility with tss-lib
func (p *PreParamsData) ConvertToLocalPreParams() *keygen.LocalPreParams {
	return &keygen.LocalPreParams{
		PaillierSK: p.PaillierKey,
		NTildei:    p.NTildei,
		H1i:        p.H1i,
		H2i:        p.H2i,
		Alpha:      p.Alpha,
		Beta:       p.Beta,
		P:          p.P,
		Q:          p.Q,
	}
}

// GeneratePreParamsBatch generates multiple PreParamsData concurrently
func (g *Generator) GeneratePreParamsBatch(primeBitSize, paillierBitSize int, count uint32) ([]*PreParamsData, error) {
	var params []*PreParamsData
	var mu sync.Mutex
	var wg sync.WaitGroup
	errCh := make(chan error, count)

	// Use limited concurrency to avoid overwhelming the system
	semaphore := make(chan struct{}, 2) // Max 2 concurrent generations (heavy operation)

	for i := uint32(0); i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			semaphore <- struct{}{}        // Acquire
			defer func() { <-semaphore }() // Release

			param, err := g.GeneratePreParams(primeBitSize, paillierBitSize)
			if err != nil {
				errCh <- err
				return
			}

			mu.Lock()
			params = append(params, param)
			mu.Unlock()
		}()
	}

	wg.Wait()
	close(errCh)

	// Check for errors
	for err := range errCh {
		if err != nil {
			return nil, fmt.Errorf("batch generation failed: %w", err)
		}
	}

	return params, nil
}

// GetStatistics returns generation statistics
func (g *Generator) GetStatistics() (int64, time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.generationCount, g.totalTime
}

// GetAverageGenerationTime returns the average time to generate parameters
func (g *Generator) GetAverageGenerationTime() time.Duration {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.generationCount == 0 {
		return 0
	}

	return g.totalTime / time.Duration(g.generationCount)
}

// GeneratePrime generates a prime number with the specified number of bits
// (kept for backward compatibility)
func (g *Generator) GeneratePrime(bits uint32, safePrime bool) (*big.Int, error) {
	start := time.Now()
	defer func() {
		g.mu.Lock()
		g.generationCount++
		g.totalTime += time.Since(start)
		g.mu.Unlock()
	}()

	if safePrime {
		return g.generateSafePrime(bits)
	}
	return g.generateRegularPrime(bits)
}

// generateRegularPrime generates a regular prime number
func (g *Generator) generateRegularPrime(bits uint32) (*big.Int, error) {
	if bits < 2 {
		return nil, fmt.Errorf("prime size must be at least 2-bits")
	}

	prime, err := rand.Prime(rand.Reader, int(bits))
	if err != nil {
		return nil, fmt.Errorf("failed to generate prime: %w", err)
	}

	return prime, nil
}

// generateSafePrime generates a safe prime using tss-lib
func (g *Generator) generateSafePrime(bits uint32) (*big.Int, error) {
	if bits < 3 {
		return nil, fmt.Errorf("safe prime size must be at least 3-bits")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	sgps, err := common.GetRandomSafePrimesConcurrent(ctx, int(bits), 1, 4, rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate safe prime: %w", err)
	}

	return sgps[0].SafePrime(), nil
}

// GenerateBatch generates multiple primes concurrently
func (g *Generator) GenerateBatch(bits uint32, safePrime bool, count uint32) ([]*big.Int, error) {
	var primes []*big.Int
	var mu sync.Mutex
	var wg sync.WaitGroup
	errCh := make(chan error, count)

	// Use limited concurrency to avoid overwhelming the system
	semaphore := make(chan struct{}, 4) // Max 4 concurrent generations

	for i := uint32(0); i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			semaphore <- struct{}{}        // Acquire
			defer func() { <-semaphore }() // Release

			prime, err := g.GeneratePrime(bits, safePrime)
			if err != nil {
				errCh <- err
				return
			}

			mu.Lock()
			primes = append(primes, prime)
			mu.Unlock()
		}()
	}

	wg.Wait()
	close(errCh)

	// Check for errors
	for err := range errCh {
		if err != nil {
			return nil, fmt.Errorf("batch generation failed: %w", err)
		}
	}

	return primes, nil
}