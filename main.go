package miner

import (
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"

	cx "colossusx/colossusx"
)

const (
	DefaultDAGMiB      = cx.StrictDAGSizeBytes / (1024 * 1024)
	DefaultReadsPerH   = cx.StrictReadsPerHash
	DefaultNodeSize    = cx.StrictNodeSize
	DefaultEpochBlocks = cx.StrictEpochBlocks
)

type BackendMode = cx.BackendMode

const (
	BackendUnified = cx.BackendUnified
	BackendCPU     = cx.BackendCPU
	BackendGPU     = cx.BackendGPU
)

type Spec = cx.Spec
type Target = cx.Target
type HashResult = cx.HashResult
type DAG = cx.DAG
type Miner = cx.Miner
type MineResult = cx.MineResult
type HashBackend = cx.HashBackend

type runtimeBackend interface {
	HashBackend
	runtimeState
	InitializeRuntime() error
}

type CLIConfig struct {
	Mode       cx.Mode
	Backend    BackendMode
	DAGAlloc   string
	Spec       Spec
	Workers    int
	Header     []byte
	EpochSeed  []byte
	Target     Target
	StartNonce uint64
	MaxNonces  uint64
	BenchOnly  bool
}

func Main(args []string) error {
	cfg, err := ParseCLIConfig(args)
	if err != nil {
		return err
	}
	backend, err := NewBackend(cfg.Backend)
	if err != nil {
		return err
	}
	return Run(cfg, backend)
}

func main() {
	if err := Main(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func ParseCLIConfig(args []string) (CLIConfig, error) {
	fs := flag.NewFlagSet("colossusx", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)

	var (
		modeName     = fs.String("mode", string(cx.ModeStrict), "operating mode: strict or research")
		backendName  = fs.String("backend", string(BackendUnified), "mining backend: unified, cpu, or gpu")
		dagAlloc     = fs.String("dag-alloc", "auto", "dag allocation strategy: auto, go-heap, pinned-host, cuda-managed, opencl-svm")
		dagMiB       = fs.Uint64("dag-mib", DefaultDAGMiB, "DAG size in MiB")
		reads        = fs.Uint64("reads", DefaultReadsPerH, "random DAG reads per hash")
		workers      = fs.Int("workers", runtime.NumCPU(), "mining worker count")
		epochBlocks  = fs.Uint64("epoch-blocks", DefaultEpochBlocks, "blocks per epoch")
		headerHex    = fs.String("header", "434f4c4f535355532d582d544553542d4845414445522d303031", "header bytes in hex")
		epochSeedHex = fs.String("epoch-seed", "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff", "epoch seed in hex")
		targetHex    = fs.String("target", "00ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "32-byte big-endian target hex")
		startNonce   = fs.Uint64("start-nonce", 0, "starting nonce")
		maxNonces    = fs.Uint64("max-nonces", 200000, "0 = unbounded")
		benchOnly    = fs.Bool("bench", false, "benchmark hash loop only")
	)
	if err := fs.Parse(args); err != nil {
		return CLIConfig{}, err
	}

	mode, err := parseMode(*modeName)
	if err != nil {
		return CLIConfig{}, err
	}
	backend, err := ParseBackendMode(*backendName)
	if err != nil {
		return CLIConfig{}, err
	}
	spec := cx.ResearchSpec((*dagMiB)*1024*1024, *reads, *epochBlocks)
	if mode == cx.ModeStrict {
		spec = cx.StrictSpec()
		if (*dagMiB)*1024*1024 != cx.StrictDAGSizeBytes || *reads != cx.StrictReadsPerHash || *epochBlocks != cx.StrictEpochBlocks {
			return CLIConfig{}, fmt.Errorf("strict mode does not allow overriding DAG, reads, or epoch constants")
		}
	}
	if err := spec.Validate(); err != nil {
		return CLIConfig{}, err
	}

	header, err := hex.DecodeString(*headerHex)
	if err != nil {
		return CLIConfig{}, fmt.Errorf("invalid header hex: %w", err)
	}
	epochSeed, err := hex.DecodeString(*epochSeedHex)
	if err != nil {
		return CLIConfig{}, fmt.Errorf("invalid epoch-seed hex: %w", err)
	}
	target, err := cx.ParseTargetHex(*targetHex)
	if err != nil {
		return CLIConfig{}, fmt.Errorf("invalid target: %w", err)
	}

	return CLIConfig{Mode: mode, Backend: backend, DAGAlloc: *dagAlloc, Spec: spec, Workers: *workers, Header: header, EpochSeed: epochSeed, Target: target, StartNonce: *startNonce, MaxNonces: *maxNonces, BenchOnly: *benchOnly}, nil
}

func Run(cfg CLIConfig, backend HashBackend) error {
	rb, err := InitializeBackendRuntime(backend)
	if err != nil {
		return err
	}
	strategy, err := ResolveDAGStrategy(cfg.Backend, rb, cfg.DAGAlloc)
	if err != nil {
		return err
	}
	PrintConfig(cfg, backend, strategy)

	dag, err := cx.NewDAGWithAllocator(cfg.Spec, strategy)
	if err != nil {
		return err
	}
	defer func() {
		if err := dag.Close(); err != nil {
			log.Printf("warning: dag close failed: %v", err)
		}
	}()

	if err := cx.PopulateDAG(dag, cfg.EpochSeed, cfg.Workers); err != nil {
		return fmt.Errorf("generate dag: %w", err)
	}
	fmt.Println("dag generated")

	if err := backend.Prepare(dag); err != nil {
		return err
	}

	miner, err := cx.NewMiner(cfg.Spec, dag, cfg.Workers, skipPrepareBackend{backend})
	if err != nil {
		return err
	}
	if cfg.BenchOnly {
		res := cx.Benchmark(miner, cfg.Header, cx.NewUint64Nonce(cfg.StartNonce), cfg.MaxNonces)
		fmt.Println("benchmark complete")
		fmt.Printf("backend: %s\n", res.Backend)
		fmt.Printf("hashes: %d\n", res.Hashes)
		fmt.Printf("elapsed: %s\n", res.Elapsed)
		fmt.Printf("hashrate: %.2f H/s\n", res.HashRate)
		return nil
	}

	res, ok := miner.Mine(cfg.Header, cfg.Target, cx.NewUint64Nonce(cfg.StartNonce), cfg.MaxNonces)
	if !ok {
		fmt.Println("no solution found in range")
		return exitCodeError(1)
	}
	fmt.Println("solution found")
	fmt.Printf("nonce: %s\n", res.Nonce.String())
	fmt.Printf("hash256: %s\n", res.Hash256Hex)
	fmt.Printf("hash512: %s\n", res.Hash512Hex)
	fmt.Printf("elapsed: %s\n", res.Elapsed)
	fmt.Printf("hashes: %d\n", res.Hashes)
	fmt.Printf("hashrate: %.2f H/s\n", res.HashRate)
	return nil
}

type skipPrepareBackend struct{ HashBackend }

func (b skipPrepareBackend) Prepare(*DAG) error { return nil }

func InitializeBackendRuntime(backend HashBackend) (runtimeState, error) {
	if rb, ok := backend.(runtimeBackend); ok {
		if err := rb.InitializeRuntime(); err != nil {
			return nil, err
		}
		return rb, nil
	}
	return nil, nil
}

func PrintConfig(cfg CLIConfig, backend HashBackend, strategy MemoryStrategy) {
	fmt.Println("COLOSSUS-X miner")
	fmt.Printf("mode: %s\n", cfg.Mode)
	fmt.Printf("backend: %s (%s)\n", backend.Mode(), backend.Description())
	fmt.Printf("dag: %d MiB\n", cfg.Spec.DAGSizeBytes/(1024*1024))
	fmt.Printf("node size: %d bytes\n", cfg.Spec.NodeSize)
	fmt.Printf("node count: %d\n", cfg.Spec.NodeCount())
	fmt.Printf("reads/hash: %d\n", cfg.Spec.ReadsPerHash)
	fmt.Printf("epoch blocks: %d\n", cfg.Spec.EpochBlocks)
	fmt.Printf("workers: %d\n", cfg.Workers)
	fmt.Printf("target: %s\n", cfg.Target.String())
	fmt.Printf("dag allocation: %s\n", strategy.Name())
}

func parseMode(s string) (cx.Mode, error) {
	switch cx.Mode(s) {
	case cx.ModeStrict, cx.ModeResearch:
		return cx.Mode(s), nil
	default:
		return "", fmt.Errorf("unsupported mode %q (expected one of: %s, %s)", s, cx.ModeStrict, cx.ModeResearch)
	}
}

func ParseBackendMode(s string) (BackendMode, error) {
	switch BackendMode(s) {
	case BackendUnified, BackendCPU, BackendGPU:
		return BackendMode(s), nil
	default:
		return "", fmt.Errorf("unsupported backend %q (expected one of: %s, %s, %s)", s, BackendUnified, BackendCPU, BackendGPU)
	}
}

func NewBackend(mode BackendMode) (HashBackend, error) {
	switch mode {
	case BackendUnified:
		return &UnifiedBackend{}, nil
	case BackendCPU:
		return &CPUBackend{}, nil
	case BackendGPU:
		return NewGPUBackend()
	default:
		return nil, fmt.Errorf("unsupported backend %q", mode)
	}
}

type exitCodeError int

func (e exitCodeError) Error() string { return fmt.Sprintf("exit code %d", int(e)) }
