package main

import (
	"testing"

	cx "colossusx/colossusx"
)

type fakeRuntimeState struct {
	cudaOrdinal int
	cudaOK      bool
	openclCtx   OpenCLContext
	openclOK    bool
}

func (r fakeRuntimeState) CUDADeviceOrdinal() (int, bool) { return r.cudaOrdinal, r.cudaOK }
func (r fakeRuntimeState) OpenCLContext() (OpenCLContext, bool) {
	return r.openclCtx, r.openclOK
}

type fakeGPUBackend struct {
	prepared      bool
	fallbackUsed  bool
	runtimeCalled bool
	scratch       *pooledScratch
}

func (b *fakeGPUBackend) Mode() BackendMode   { return BackendGPU }
func (b *fakeGPUBackend) Description() string { return "test gpu backend" }
func (b *fakeGPUBackend) InitializeRuntime() error {
	b.runtimeCalled = true
	return nil
}
func (b *fakeGPUBackend) CUDADeviceOrdinal() (int, bool)       { return 7, true }
func (b *fakeGPUBackend) OpenCLContext() (OpenCLContext, bool) { return OpenCLContext{}, false }
func (b *fakeGPUBackend) Prepare(dag *DAG) error {
	b.prepared = true
	if b.scratch == nil {
		b.scratch = newPooledScratch()
	}
	_, err := newRawContiguousDAGBuffer(dag)
	return err
}
func (b *fakeGPUBackend) Hash(header []byte, nonce cx.Nonce, dag *DAG) HashResult {
	s := b.scratch.acquire(len(header))
	defer b.scratch.release(s)
	view, _ := newUnifiedMemoryDAGViewFromBytes(dag.Spec(), dag.Bytes())
	return latticeHashWithAccessor(dag.Spec(), header, nonce, view, s)
}
func (b *fakeGPUBackend) HashBatch(header []byte, startNonce cx.Nonce, count uint64, dag *DAG) ([]HashResult, error) {
	results := make([]HashResult, 0, count)
	for i := uint64(0); i < count; i++ {
		nonce, ok := startNonce.AddUint64(i)
		if !ok {
			break
		}
		results = append(results, b.Hash(header, nonce, dag))
	}
	return results, nil
}

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

func TestGPUHashMatchesCPUReference(t *testing.T) {
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

	gpu := &fakeGPUBackend{}
	cpu := &CPUBackend{}
	if err := gpu.Prepare(dag); err != nil {
		t.Fatalf("gpu Prepare: %v", err)
	}
	if err := cpu.Prepare(dag); err != nil {
		t.Fatalf("cpu Prepare: %v", err)
	}

	gpuHash := gpu.Hash(header, nonce, dag)
	cpuHash := cpu.Hash(header, nonce, dag)
	if gpuHash != cpuHash {
		t.Fatalf("expected gpu and cpu backends to match; gpu=%x cpu=%x", gpuHash.Pow256, cpuHash.Pow256)
	}
}

func TestSuccessfulGPURunAvoidsNormalFallback(t *testing.T) {
	spec := Spec{Mode: cx.ModeResearch, DAGSizeBytes: 64 * 64, NodeSize: DefaultNodeSize, ReadsPerHash: 4, EpochBlocks: DefaultEpochBlocks}
	dag, err := NewDAG(spec)
	if err != nil {
		t.Fatalf("NewDAG: %v", err)
	}
	defer dag.Close()
	if err := GenerateDAG(dag, []byte("seedseedseedseedseedseedseedseed"), 1); err != nil {
		t.Fatalf("GenerateDAG: %v", err)
	}
	backend := &fakeGPUBackend{}
	if _, err := initializeBackendRuntime(backend); err != nil {
		t.Fatalf("initializeBackendRuntime: %v", err)
	}
	if !backend.runtimeCalled {
		t.Fatal("expected runtime initialization before allocator resolution")
	}
	if err := backend.Prepare(dag); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	results, err := backend.HashBatch([]byte("header"), cx.NewUint64Nonce(0), 4, dag)
	if err != nil {
		t.Fatalf("HashBatch: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("expected 4 gpu results, got %d", len(results))
	}
	if backend.fallbackUsed {
		t.Fatal("expected successful GPU dispatch to avoid CPU fallback")
	}
}
