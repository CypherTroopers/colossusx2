package main

import (
	"testing"

	cx "colossusx/colossusx"
)

type testAllocation struct {
	buf   []byte
	freed bool
}

func (a *testAllocation) Bytes() []byte { return a.buf }
func (a *testAllocation) Free() error   { a.freed = true; return nil }
func (a *testAllocation) Name() string  { return "test-allocation" }

func TestNewDAGWithStrategyGoHeap(t *testing.T) {
	spec := Spec{Mode: cx.ModeResearch, DAGSizeBytes: 1024, NodeSize: DefaultNodeSize, ReadsPerHash: 4, EpochBlocks: DefaultEpochBlocks}
	dag, err := NewDAGWithStrategy(spec, GoHeapMemory{})
	if err != nil {
		t.Fatalf("NewDAGWithStrategy: %v", err)
	}
	defer dag.Close()
	if got := uint64(len(dag.Bytes())); got != spec.DAGSizeBytes {
		t.Fatalf("expected DAG bytes %d, got %d", spec.DAGSizeBytes, got)
	}
}

func TestUnsupportedStrategyReturnsExplicitError(t *testing.T) {
	if _, err := NewDAGWithStrategy(Spec{Mode: cx.ModeResearch, DAGSizeBytes: 1024, NodeSize: DefaultNodeSize, ReadsPerHash: 4, EpochBlocks: DefaultEpochBlocks}, PinnedMemory{}); err == nil {
		t.Fatal("expected unsupported pinned strategy to fail")
	}
	if _, err := (CUDAManagedMemory{}).Alloc(64); err == nil {
		t.Fatal("expected CUDA managed memory stub to fail without cuda+cgo build")
	}
	if _, err := (OpenCLSVM{}).Alloc(64); err == nil {
		t.Fatal("expected OpenCL SVM stub to fail without opencl+cgo build")
	}
}

func TestDAGCloseReleasesOwnedAllocation(t *testing.T) {
	alloc := &testAllocation{buf: make([]byte, 1024)}
	dag, err := NewDAGWithAllocation(Spec{Mode: cx.ModeResearch, DAGSizeBytes: 1024, NodeSize: DefaultNodeSize, ReadsPerHash: 4, EpochBlocks: DefaultEpochBlocks}, alloc, true)
	if err != nil {
		t.Fatalf("NewDAGWithAllocation: %v", err)
	}
	if err := dag.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !alloc.freed {
		t.Fatal("expected DAG.Close to free owned allocation")
	}
}

func TestSelectDAGStrategy(t *testing.T) {
	strategy, err := selectDAGStrategy(BackendCPU, "auto")
	if err != nil {
		t.Fatalf("selectDAGStrategy: %v", err)
	}
	if strategy.Name() != "go-heap" {
		t.Fatalf("expected go-heap, got %q", strategy.Name())
	}
	if _, err := selectDAGStrategy(BackendUnified, "bogus"); err == nil {
		t.Fatal("expected invalid dag strategy to fail")
	}
}

func TestNewDAGWithAllocationRejectsShortBuffer(t *testing.T) {
	alloc := &testAllocation{buf: make([]byte, 8)}
	_, err := NewDAGWithAllocation(Spec{Mode: cx.ModeResearch, DAGSizeBytes: 1024, NodeSize: DefaultNodeSize, ReadsPerHash: 4, EpochBlocks: DefaultEpochBlocks}, alloc, false)
	if err == nil {
		t.Fatal("expected short allocation to fail")
	}
	if err.Error() != "managed allocation is smaller than the DAG" {
		t.Fatalf("unexpected error: %v", err)
	}
}
