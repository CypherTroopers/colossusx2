package main

// ===============================
// Memory Strategy Abstraction
// ===============================

type MemoryStrategy interface {
	Alloc(size uint64) ([]byte, error)
	Free([]byte) error
	Name() string
}

// -------------------------------
// 1. Go heap (fallback)
// -------------------------------

type GoHeapMemory struct{}

func (GoHeapMemory) Alloc(size uint64) ([]byte, error) {
	return make([]byte, size), nil
}
func (GoHeapMemory) Free(_ []byte) error { return nil }
func (GoHeapMemory) Name() string        { return "go-heap" }

// -------------------------------
// 2. Pinned / mapped (placeholder)
// -------------------------------
// Actual implementation would use cgo with cudaHostAlloc / mmap / mlock, etc.

type PinnedMemory struct{}

func (PinnedMemory) Alloc(size uint64) ([]byte, error) {
	buf := make([]byte, size)
	// TODO: mlock / cudaHostAlloc (cgo)
	return buf, nil
}
func (PinnedMemory) Free(_ []byte) error { return nil }
func (PinnedMemory) Name() string        { return "pinned-host" }

// -------------------------------
// 3. CUDA Unified Memory (design hook)
// -------------------------------
// cgo is required for the actual implementation.

type CUDAManagedMemory struct{}

func (CUDAManagedMemory) Alloc(size uint64) ([]byte, error) {
	// TODO:
	// cudaMallocManaged(...)
	// return unsafe.Slice(ptr, size)

	return nil, ErrNotImplemented("cuda managed memory")
}
func (CUDAManagedMemory) Free(_ []byte) error { return nil }
func (CUDAManagedMemory) Name() string        { return "cuda-managed" }

// -------------------------------
// 4. OpenCL SVM (design hook)
// -------------------------------

type OpenCLSVM struct{}

func (OpenCLSVM) Alloc(size uint64) ([]byte, error) {
	// TODO:
	// clSVMAlloc(...)
	return nil, ErrNotImplemented("opencl svm")
}
func (OpenCLSVM) Free(_ []byte) error { return nil }
func (OpenCLSVM) Name() string        { return "opencl-svm" }

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
// Unified Backend (REAL DESIGN)
// ===============================

type UnifiedBackend struct {
	shared   unifiedMemoryDAGView
	scratch  *pooledScratch
	strategy MemoryStrategy
}

func (b *UnifiedBackend) Mode() BackendMode { return BackendUnified }

func (b *UnifiedBackend) Description() string {
	return "managed unified memory backend (pluggable: cuda/opencl/uma/pinned)"
}

// -------------------------------
// Strategy selector
// -------------------------------

func selectMemoryStrategy() MemoryStrategy {
	// TODO: In a real implementation, branch by runtime / build tag / env

	// Example:
	// if CUDA is available → CUDAManagedMemory{}
	// if OpenCL SVM is available → OpenCLSVM{}
	// if Apple Silicon → GoHeap (UMA is already shared)
	// fallback:
	return GoHeapMemory{}
}

// -------------------------------
// Prepare
// -------------------------------

func (b *UnifiedBackend) Prepare(dag *DAG) error {
	if dag == nil {
		return ErrNilDAG
	}

	if b.strategy == nil {
		b.strategy = selectMemoryStrategy()
	}

	// ⭐ This is the core part
	// In a real implementation, this is where you would do:
	// - cudaMallocManaged
	// - clSVMAlloc
	// - mmap shared

	shared, err := newUnifiedMemoryDAGView(dag)
	if err != nil {
		return err
	}

	b.shared = shared

	if b.scratch == nil {
		b.scratch = newPooledScratch()
	}

	return nil
}

// -------------------------------
// Hash
// -------------------------------

func (b *UnifiedBackend) Hash(header []byte, nonce uint64, dag *DAG) HashResult {
	if b.shared.NodeCount() == 0 {
		if err := b.Prepare(dag); err != nil {
			return HashResult{}
		}
	}

	s := b.scratch.acquire(len(header))
	defer b.scratch.release(s)

	return latticeHashWithAccessor(
		header,
		nonce,
		dag.spec.ReadsPerHash,
		b.shared,
		s,
	)
}
