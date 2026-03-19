package main

import (
	"fmt"
	"os"
	"strings"
)

// ===============================
// Memory Strategy Abstraction
// ===============================

type managedAllocation interface {
	Bytes() []byte
	Free() error
	Name() string
}

type MemoryStrategy interface {
	BindDAG(*DAG) (managedAllocation, error)
	Alloc(size uint64) (managedAllocation, error)
	Name() string
}

type sliceAllocation struct {
	name string
	buf  []byte
	free func() error
}

func (a *sliceAllocation) Bytes() []byte { return a.buf }
func (a *sliceAllocation) Name() string  { return a.name }
func (a *sliceAllocation) Free() error {
	if a == nil || a.free == nil {
		return nil
	}
	return a.free()
}

// -------------------------------
// 1. Go heap (shared fallback)
// -------------------------------

type GoHeapMemory struct{}

func (GoHeapMemory) BindDAG(dag *DAG) (managedAllocation, error) {
	if dag == nil {
		return nil, ErrNilDAG
	}
	return &sliceAllocation{name: "go-heap", buf: dag.Bytes()}, nil
}

func (GoHeapMemory) Alloc(size uint64) (managedAllocation, error) {
	return &sliceAllocation{name: "go-heap", buf: make([]byte, size)}, nil
}
func (GoHeapMemory) Name() string { return "go-heap" }

// -------------------------------
// 2. Pinned / mapped (portable copy)
// -------------------------------

type PinnedMemory struct{}

func (PinnedMemory) BindDAG(dag *DAG) (managedAllocation, error) {
	if dag == nil {
		return nil, ErrNilDAG
	}
	alloc, err := PinnedMemory{}.Alloc(uint64(len(dag.Bytes())))
	if err != nil {
		return nil, err
	}
	copy(alloc.Bytes(), dag.Bytes())
	return alloc, nil
}

func (PinnedMemory) Alloc(size uint64) (managedAllocation, error) {
	buf := make([]byte, size)
	return &sliceAllocation{name: "pinned-host", buf: buf}, nil
}
func (PinnedMemory) Name() string { return "pinned-host" }

// -------------------------------
// 3. CUDA Unified Memory
// -------------------------------

type CUDAManagedMemory struct{}

func (m CUDAManagedMemory) BindDAG(dag *DAG) (managedAllocation, error) {
	if dag == nil {
		return nil, ErrNilDAG
	}
	alloc, err := m.Alloc(uint64(len(dag.Bytes())))
	if err != nil {
		return nil, err
	}
	copy(alloc.Bytes(), dag.Bytes())
	return alloc, nil
}

func (m CUDAManagedMemory) Alloc(size uint64) (managedAllocation, error) {
	return allocCUDAManaged(size)
}
func (CUDAManagedMemory) Name() string { return "cuda-managed" }

// -------------------------------
// 4. OpenCL SVM
// -------------------------------

type OpenCLSVM struct {
	Context OpenCLContext
}

func (m OpenCLSVM) BindDAG(dag *DAG) (managedAllocation, error) {
	if dag == nil {
		return nil, ErrNilDAG
	}
	alloc, err := m.Alloc(uint64(len(dag.Bytes())))
	if err != nil {
		return nil, err
	}
	copy(alloc.Bytes(), dag.Bytes())
	return alloc, nil
}

func (m OpenCLSVM) Alloc(size uint64) (managedAllocation, error) {
	return allocOpenCLSVM(m.Context, size)
}
func (OpenCLSVM) Name() string { return "opencl-svm" }

// -------------------------------
// error helper
// -------------------------------

type notImplementedError string

func (e notImplementedError) Error() string {
	return "not implemented: " + string(e)
}

func ErrNotImplemented(s string) error {
	return notImplementedError(s)
}

// ===============================
// Unified Backend (real strategy wiring)
// ===============================

type UnifiedBackend struct {
	shared      unifiedMemoryDAGView
	scratch     *pooledScratch
	strategy    MemoryStrategy
	allocation  managedAllocation
	strategyEnv string
}

func (b *UnifiedBackend) Mode() BackendMode { return BackendUnified }

func (b *UnifiedBackend) Description() string {
	name := "auto"
	if b.strategy != nil {
		name = b.strategy.Name()
	}
	return fmt.Sprintf("managed unified memory backend (strategy=%s)", name)
}

func selectMemoryStrategy() MemoryStrategy {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("COLOSSUSX_UNIFIED_STRATEGY"))) {
	case "", "auto", "go", "go-heap":
		return GoHeapMemory{}
	case "pinned", "pinned-host":
		return PinnedMemory{}
	case "cuda", "cuda-managed":
		return CUDAManagedMemory{}
	case "opencl", "opencl-svm", "svm":
		return OpenCLSVM{}
	default:
		return GoHeapMemory{}
	}
}

func (b *UnifiedBackend) Prepare(dag *DAG) error {
	if dag == nil {
		return ErrNilDAG
	}
	if b.allocation != nil {
		_ = b.allocation.Free()
		b.allocation = nil
	}
	if b.strategy == nil {
		b.strategy = selectMemoryStrategy()
	}
	alloc, err := b.strategy.BindDAG(dag)
	if err != nil {
		return err
	}
	shared, err := newUnifiedMemoryDAGViewFromBytes(dag.spec, alloc.Bytes())
	if err != nil {
		_ = alloc.Free()
		return err
	}
	b.allocation = alloc
	b.shared = shared
	if b.scratch == nil {
		b.scratch = newPooledScratch()
	}
	return nil
}

func (b *UnifiedBackend) Hash(header []byte, nonce uint64, dag *DAG) HashResult {
	if b.shared.NodeCount() == 0 {
		if err := b.Prepare(dag); err != nil {
			return HashResult{}
		}
	}
	s := b.scratch.acquire(len(header))
	defer b.scratch.release(s)
	return latticeHashWithAccessor(header, nonce, dag.spec.ReadsPerHash, b.shared, s)
}
