package main

import "testing"

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

func TestCPUAndUnifiedBackendsProduceSameHash(t *testing.T) {
	spec := Spec{DAGSizeBytes: 1024 * 1024, NodeSize: DefaultNodeSize, ReadsPerHash: 8, EpochBlocks: DefaultEpochBlocks}
	dag, err := NewDAG(spec)
	if err != nil {
		t.Fatalf("NewDAG: %v", err)
	}
	seed := []byte("0123456789abcdef0123456789abcdef")
	if err := GenerateDAG(dag, seed, 2); err != nil {
		t.Fatalf("GenerateDAG: %v", err)
	}
	header := []byte("header")
	nonce := uint64(42)

	cpu := CPUBackend{}
	unified := UnifiedBackend{}

	cpuHash := cpu.Hash(header, nonce, dag)
	unifiedHash := unified.Hash(header, nonce, dag)
	if cpuHash != unifiedHash {
		t.Fatalf("expected cpu and unified backends to match; cpu=%x unified=%x", cpuHash.Pow256, unifiedHash.Pow256)
	}
}
