package main

import (
	"testing"

	cx "colossusx/colossusx"
)

func TestParseBackendMode(t *testing.T) {
	for _, mode := range []string{"unified", "cpu", "gpu"} {
		if _, err := parseBackendMode(mode); err != nil {
			t.Fatalf("parseBackendMode(%q) returned error: %v", mode, err)
		}
	}
	if _, err := parseBackendMode("bogus"); err == nil {
		t.Fatal("expected invalid backend to fail")
	}
}

func TestParseCLIConfigStrictModeRejectsOverrides(t *testing.T) {
	_, err := parseCLIConfig([]string{"-mode", "strict", "-dag-mib", "1"})
	if err == nil {
		t.Fatal("expected strict mode override to fail")
	}
}

func TestParseCLIConfigResearchModeAllowsOverrides(t *testing.T) {
	cfg, err := parseCLIConfig([]string{"-mode", "research", "-dag-mib", "1", "-reads", "8", "-epoch-blocks", "16"})
	if err != nil {
		t.Fatalf("parseCLIConfig: %v", err)
	}
	if cfg.spec.Mode != cx.ModeResearch || cfg.spec.DAGSizeBytes != 1024*1024 || cfg.spec.ReadsPerHash != 8 || cfg.spec.EpochBlocks != 16 {
		t.Fatalf("unexpected research spec: %+v", cfg.spec)
	}
}

func TestCPUAndUnifiedBackendsProduceSameHash(t *testing.T) {
	spec := Spec{Mode: cx.ModeResearch, DAGSizeBytes: 1024 * 1024, NodeSize: DefaultNodeSize, ReadsPerHash: 8, EpochBlocks: DefaultEpochBlocks}
	dag, err := NewDAG(spec)
	if err != nil {
		t.Fatalf("NewDAG: %v", err)
	}
	defer dag.Close()
	seed := []byte("0123456789abcdef0123456789abcdef")
	if err := GenerateDAG(dag, seed, 2); err != nil {
		t.Fatalf("GenerateDAG: %v", err)
	}
	header := []byte("header")
	nonce := cx.NewUint64Nonce(42)

	cpu := &CPUBackend{}
	unified := &UnifiedBackend{}
	if err := cpu.Prepare(dag); err != nil {
		t.Fatalf("cpu Prepare: %v", err)
	}
	if err := unified.Prepare(dag); err != nil {
		t.Fatalf("unified Prepare: %v", err)
	}

	cpuHash := cpu.Hash(header, nonce, dag)
	unifiedHash := unified.Hash(header, nonce, dag)
	if cpuHash != unifiedHash {
		t.Fatalf("expected cpu and unified backends to match; cpu=%x unified=%x", cpuHash.Pow256, unifiedHash.Pow256)
	}
}

func TestUnifiedBackendUsesDAGAllocationDirectly(t *testing.T) {
	spec := Spec{Mode: cx.ModeResearch, DAGSizeBytes: 64 * 8, NodeSize: DefaultNodeSize, ReadsPerHash: 4, EpochBlocks: DefaultEpochBlocks}
	dag, err := NewDAG(spec)
	if err != nil {
		t.Fatalf("NewDAG: %v", err)
	}
	defer dag.Close()
	if err := GenerateDAG(dag, []byte("seedseedseedseedseedseedseedseed"), 1); err != nil {
		t.Fatalf("GenerateDAG: %v", err)
	}
	backend := &UnifiedBackend{}
	if err := backend.Prepare(dag); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	original := backend.Hash([]byte("header"), cx.NewUint64Nonce(1), dag)

	copy(dag.Bytes(), make([]byte, len(dag.Bytes())))
	mutated := backend.Hash([]byte("header"), cx.NewUint64Nonce(1), dag)
	if original == mutated {
		t.Fatal("expected unified backend to observe DAG mutations through shared memory")
	}
}

func TestCPUBackendCopiesPreparedDAG(t *testing.T) {
	spec := Spec{Mode: cx.ModeResearch, DAGSizeBytes: 64 * 8, NodeSize: DefaultNodeSize, ReadsPerHash: 4, EpochBlocks: DefaultEpochBlocks}
	dag, err := NewDAG(spec)
	if err != nil {
		t.Fatalf("NewDAG: %v", err)
	}
	defer dag.Close()
	if err := GenerateDAG(dag, []byte("seedseedseedseedseedseedseedseed"), 1); err != nil {
		t.Fatalf("GenerateDAG: %v", err)
	}
	backend := &CPUBackend{}
	if err := backend.Prepare(dag); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	original := backend.Hash([]byte("header"), cx.NewUint64Nonce(1), dag)

	copy(dag.Bytes(), make([]byte, len(dag.Bytes())))
	mutated := backend.Hash([]byte("header"), cx.NewUint64Nonce(1), dag)
	if original != mutated {
		t.Fatal("expected prepared CPU backend to keep using its own copied DAG")
	}
}

func TestRunInitializesBackendRuntimeBeforeResolvingAllocator(t *testing.T) {
	spec := Spec{Mode: cx.ModeResearch, DAGSizeBytes: 64 * 64, NodeSize: DefaultNodeSize, ReadsPerHash: 4, EpochBlocks: DefaultEpochBlocks}
	cfg := cliConfig{mode: cx.ModeResearch, backend: BackendGPU, dagAlloc: "auto", spec: spec, workers: 1, header: []byte("01"), epochSeed: []byte("seedseedseedseedseedseedseedseed"), target: cx.Target{}, maxNonces: 1, benchOnly: true}
	backend := &fakeGPUBackend{}
	if err := run(cfg, backend); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !backend.runtimeCalled {
		t.Fatal("expected run to initialize backend runtime before allocator resolution")
	}
	if !backend.prepared {
		t.Fatal("expected run to prepare backend after dag allocation/population")
	}
}
