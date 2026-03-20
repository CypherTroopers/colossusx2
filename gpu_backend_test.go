package main

import (
	"testing"

	cx "colossusx/colossusx"
)

func TestGPUBackendCanBeConstructed(t *testing.T) {
	backend, err := NewGPUBackend()
	if err != nil {
		t.Fatalf("NewGPUBackend: %v", err)
	}
	if backend == nil {
		t.Fatal("expected gpu backend instance")
	}
	if backend.Mode() != BackendGPU {
		t.Fatalf("unexpected backend mode: %s", backend.Mode())
	}
}

func TestGPUBackendMatchesUnifiedHash(t *testing.T) {
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

	gpu, err := NewGPUBackend()
	if err != nil {
		t.Fatalf("NewGPUBackend: %v", err)
	}
	unified := &UnifiedBackend{}
	if err := gpu.Prepare(dag); err != nil {
		t.Fatalf("gpu Prepare: %v", err)
	}
	if err := unified.Prepare(dag); err != nil {
		t.Fatalf("unified Prepare: %v", err)
	}

	gpuHash := gpu.Hash(header, nonce, dag)
	unifiedHash := unified.Hash(header, nonce, dag)
	if gpuHash != unifiedHash {
		t.Fatalf("expected gpu and unified backends to match; gpu=%x unified=%x", gpuHash.Pow256, unifiedHash.Pow256)
	}
}
