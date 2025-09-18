package main

import (
	"context"
	"flag"
	"github.com/eco-digit/energyprofiler/internal/benchmarks"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/eco-digit/energyprofiler/internal/results"
	"github.com/eco-digit/energyprofiler/internal/types"
	"github.com/eco-digit/energyprofiler/internal/utils"
)

func main() {
	config := parseFlags()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Received interrupt signal, shutting down...")
		cancel()
	}()

	log.Printf("Starting baremetal-profiles core")
	log.Printf("Resource: %s, Cycles: %d, Load levels: %v",
		config.Resource, config.Cycles, config.LoadLevels)

	benchmarkResults := &types.BenchmarkResults{
		PowerProfile: types.PowerProfile{},
	}

	switch config.Resource {
	case "cpu", "compute":
		if err := runCPUBenchmark(ctx, config, benchmarkResults); err != nil {
			log.Fatalf("CPU core failed: %v", err)
		}

	case "memory", "memory-capacity":
		if err := runMemoryCapacityBenchmark(ctx, config, benchmarkResults); err != nil {
			log.Fatalf("Memory capacity benchmark failed: %v", err)
		}

	case "memory-bandwidth":
		if err := runMemoryBandwidthBenchmark(ctx, config, benchmarkResults); err != nil {
			log.Fatalf("Memory bandwidth benchmark failed: %v", err)
		}
	case "network", "transfer":
		if err := runNetworkBenchmark(ctx, config, benchmarkResults); err != nil {
			log.Fatalf("Network benchmark failed: %v", err)
		}

	case "storage", "store":
		if err := runStorageBenchmark(ctx, config, benchmarkResults); err != nil {
			log.Fatalf("Storage benchmark failed: %v", err)
		}
	case "all":
		log.Println("Running all benchmarks")

		// CPU
		log.Println("### CPU Benchmark ###")
		if err := runCPUBenchmark(ctx, config, benchmarkResults); err != nil {
			log.Printf("Warning: CPU benchmark failed: %v", err)
		}

		// Memory Capacity
		log.Println("### Memory Capacity Benchmark ###")
		if err := runMemoryCapacityBenchmark(ctx, config, benchmarkResults); err != nil {
			log.Printf("Warning: Memory capacity benchmark failed: %v", err)
		}

		// Storage
		log.Println("### Storage Benchmark ###")
		if err := runStorageBenchmark(ctx, config, benchmarkResults); err != nil {
			log.Printf("Warning: Storage benchmark failed: %v", err)
		}

		// Tranfser/Network
		log.Println("### Network Benchmark ###")
		if err := runNetworkBenchmark(ctx, config, benchmarkResults); err != nil {
			log.Printf("Warning: NEtwork benchmark failed: %v", err)
		}

	default:
		log.Fatalf("Unknown resource type: %s", config.Resource)
	}

	// get total average
	benchmarkResults.PowerProfile.TotalAvg = calculateTotalAverage(benchmarkResults)

	if err := results.SaveResults(benchmarkResults, config.OutputFile); err != nil {
		log.Fatalf("Failed to save results: %v", err)
	}

	log.Printf("Benchmark completed successfully!")
	log.Printf("Results saved to: %s", config.OutputFile)
	log.Printf("Total average power: %.2f W", benchmarkResults.PowerProfile.TotalAvg)
}

func parseFlags() *types.Config {
	var (
		resource          = flag.String("resource", "cpu", "Resource to core")
		cycles            = flag.Int("cycles", 3, "Number of core cycles")
		loadLevelsStr     = flag.String("load-levels", "10,25,50,75,100", "Load levels (%)")
		stabilizeDurStr   = flag.String("stabilize", "30s", "Stabilization time")
		measurementDurStr = flag.String("measurement-duration", "60s", "Measurement duration")
		measurementIntStr = flag.String("measurement-interval", "10s", "Measurement interval")
		cooldownDurStr    = flag.String("cooldown", "30s", "Cooldown time")
		dataSource        = flag.String("data-source", "bmc", "Data source")
		outputFile        = flag.String("output", "benchmark-results.json", "Output file")
		useSystemPower    = flag.Bool("system-power", false, "Use system power")
		verbose           = flag.Bool("verbose", false, "Verbose logging")
		streamPath        = flag.String("stream-path", "/usr/local/bin/stream", "STREAM path")
		streamWorkdir     = flag.String("stream-workdir", "", "STREAM workdir")
		storageStressor   = flag.String("storage-stressor", "io", "Storage stressor type: io, hdd, ssd, iomix, aio")
		networkServer     = flag.String("network-server", "", "iperf3 server address")
		networkPort       = flag.Int("network-port", 5201, "iperf3 server port")
		networkTestMode   = flag.String("network-mode", "send", "Network test mode: send, receive, or bidirectional")
	)

	flag.Parse()

	return &types.Config{
		Resource:            *resource,
		Cycles:              *cycles,
		LoadLevels:          utils.ParseLoadLevels(*loadLevelsStr),
		StabilizeDuration:   utils.ParseDuration(*stabilizeDurStr),
		MeasurementDuration: utils.ParseDuration(*measurementDurStr),
		MeasurementInterval: utils.ParseDuration(*measurementIntStr),
		CooldownDuration:    utils.ParseDuration(*cooldownDurStr),
		DataSource:          *dataSource,
		OutputFile:          *outputFile,
		UseSystemPower:      *useSystemPower,
		Verbose:             *verbose,
		StreamPath:          *streamPath,
		StreamWorkdir:       *streamWorkdir,
		StorageStressor:     *storageStressor,
		NetworkServer:       *networkServer,
		NetworkPort:         *networkPort,
		NetworkTestMode:     *networkTestMode,
	}
}

func runCPUBenchmark(ctx context.Context, config *types.Config, results *types.BenchmarkResults) error {
	bench := benchmarks.NewCPUBenchmark(config)

	log.Printf("### Running %s Benchmark ###", bench.Name())

	profile, err := bench.Run(ctx)
	if err != nil {
		return err
	}

	// after each benchmark
	results.PowerProfile.Compute = profile
	return nil
}

func runMemoryCapacityBenchmark(ctx context.Context, config *types.Config, results *types.BenchmarkResults) error {
	bench := benchmarks.NewMemoryCapacityBenchmark(config)

	log.Printf("=== Running %s Benchmark ===", bench.Name())

	profile, err := bench.Run(ctx)
	if err != nil {
		return err
	}

	results.PowerProfile.Memorize = profile
	return nil
}

func runMemoryBandwidthBenchmark(ctx context.Context, config *types.Config, results *types.BenchmarkResults) error {
	bench := benchmarks.NewMemoryBandwidthBenchmark(config)

	log.Printf("### Running %s Benchmark ###", bench.Name())
	log.Printf("Note: Using predefined bandwidth configurations, ignoring --load-levels flag")

	profile, err := bench.Run(ctx)
	if err != nil {
		return err
	}

	results.PowerProfile.Memorize = profile
	return nil
}

func runStorageBenchmark(ctx context.Context, config *types.Config, results *types.BenchmarkResults) error {
	bench := benchmarks.NewStorageBenchmark(config)

	log.Printf("### Running %s Benchmark ###", bench.Name())

	profile, err := bench.Run(ctx)
	if err != nil {
		return err
	}

	results.PowerProfile.Store = profile
	return nil
}

func runNetworkBenchmark(ctx context.Context, config *types.Config, results *types.BenchmarkResults) error {
	bench := benchmarks.NewNetworkBenchmark(config)

	log.Printf("### Running %s Benchmark ###", bench.Name())

	profile, err := bench.Run(ctx)
	if err != nil {
		return err
	}

	results.PowerProfile.Transfer = profile
	return nil
}

func calculateTotalAverage(results *types.BenchmarkResults) float64 {
	var totalSum float64
	var resourceCount int

	if results.PowerProfile.Compute != nil {
		totalSum += results.PowerProfile.Compute.Avg
		resourceCount++
	}
	if results.PowerProfile.Memorize != nil {
		totalSum += results.PowerProfile.Memorize.Avg
		resourceCount++
	}
	if results.PowerProfile.Transfer != nil {
		totalSum += results.PowerProfile.Transfer.Avg
		resourceCount++
	}
	if results.PowerProfile.Store != nil {
		totalSum += results.PowerProfile.Store.Avg
		resourceCount++
	}

	if resourceCount > 0 {
		return totalSum / float64(resourceCount)
	}
	return 0
}
