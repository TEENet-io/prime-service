package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/TEENet-io/prime-service/client"
)

func main() {
	// Create a client
	c, err := client.NewClient("localhost:50055")
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer c.Close()

	ctx := context.Background()

	// Test 1: Get pool status
	fmt.Println("Test 1: Getting pool status...")
	status, err := c.GetPoolStatus(ctx)
	if err != nil {
		log.Printf("Failed to get pool status: %v", err)
	} else {
		if pool, ok := status.Pools["1024_true"]; ok {
			fmt.Printf("  Pool size: %d PreParams\n", pool.Available)
			fmt.Printf("  Target size: %d PreParams\n", pool.TargetSize)
		}
		fmt.Printf("  Total generated: %d\n", status.TotalGenerated)
		fmt.Printf("  Total served: %d\n", status.TotalServed)
	}

	// Test 2: Get a single PreParamsData
	fmt.Println("\nTest 2: Getting a single PreParamsData...")
	start := time.Now()
	params, err := c.GetPreParams(ctx, 1)
	if err != nil {
		log.Printf("Failed to get pre-params: %v", err)
	} else {
		fmt.Printf("  Got %d PreParamsData in %v\n", len(params), time.Since(start))
		if len(params) > 0 {
			fmt.Printf("  Paillier N: %d bits\n", params[0].PaillierKey.N.BitLen())
			fmt.Printf("  NTildei: %d bits\n", params[0].NTildei.BitLen())
			fmt.Printf("  Generated at: %s\n", params[0].GeneratedAt)
		}
	}

	// Test 3: Get batch PreParamsData
	fmt.Println("\nTest 3: Getting batch PreParamsData (3 params)...")
	start = time.Now()
	batch, err := c.GetPreParams(ctx, 3)
	if err != nil {
		log.Printf("Failed to get batch: %v", err)
	} else {
		fmt.Printf("  Got %d PreParamsData in %v\n", len(batch), time.Since(start))
		for i, p := range batch {
			fmt.Printf("  Param %d: Paillier N = %d bits\n", i+1, p.PaillierKey.N.BitLen())
		}
	}

	fmt.Println("\n✅ Client library test completed!")
}
