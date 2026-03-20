package main

import (
	"os"
	"testing"

	"colossusx/pkg/chain"
	"colossusx/pkg/consensus"
	"colossusx/pkg/miner"
	"colossusx/pkg/mining"
	"colossusx/pkg/node"
)

func parseFlagsForTest(t *testing.T, args ...string) config {
	t.Helper()
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = append([]string{"colossusd"}, args...)
	cfg, err := parseFlags()
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func buildNodeForTest(t *testing.T, cfg config) {
	t.Helper()
	validatorBackend, err := mining.NewBackend(cfg.ValidatorBackend)
	if err != nil {
		t.Fatal(err)
	}
	validator, err := consensus.NewValidator(cfg.Chain, validatorBackend, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer validator.Close()

	minerBackend, err := mining.NewBackend(cfg.MinerBackend)
	if err != nil {
		t.Fatal(err)
	}
	runtimeState, err := mining.InitializeBackendRuntime(minerBackend)
	if err != nil {
		t.Fatal(err)
	}
	mode, err := mining.ParseBackendMode(cfg.MinerBackend)
	if err != nil {
		t.Fatal(err)
	}
	allocator, err := mining.ResolveDAGStrategy(mode, cfg.MinerDAGAlloc, runtimeState)
	if err != nil {
		t.Fatal(err)
	}
	minerSvc, err := miner.NewService(cfg.Chain.Spec, 1, minerBackend, allocator)
	if err != nil {
		t.Fatal(err)
	}
	defer minerSvc.Close()

	if _, err := node.New(node.Config{Chain: cfg.Chain, Genesis: cfg.Genesis, Mine: false, MaxNonces: cfg.MaxNonces, BlockTime: cfg.BlockTime}, validator, minerSvc, chain.NewMemoryStore()); err != nil {
		t.Fatal(err)
	}
}

func TestColossusdCanStartWithCPUMinerBackend(t *testing.T) {
	cfg := parseFlagsForTest(t, "--miner-backend=cpu", "--miner-dag-alloc=go-heap", "--datadir", t.TempDir())
	buildNodeForTest(t, cfg)
}

func TestColossusdCanStartWithUnifiedMinerBackendAndGoHeap(t *testing.T) {
	cfg := parseFlagsForTest(t, "--miner-backend=unified", "--miner-dag-alloc=go-heap", "--datadir", t.TempDir())
	buildNodeForTest(t, cfg)
}

func TestUnavailableExplicitAllocatorFailsCleanly(t *testing.T) {
	if _, err := mining.ResolveDAGStrategy(mining.BackendGPU, "opencl-svm", nil); err == nil {
		t.Fatal("expected explicit unavailable opencl-svm allocator to fail")
	}
}
