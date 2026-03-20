//go:build cuda

package main

import (
	"fmt"

	cx "colossusx/colossusx"
)

type cudaRuntime interface {
	runtimeState
	Initialize() error
	Available() bool
}

type nativeCUDARuntime struct {
	available   bool
	device      int
	initialized bool
	initErr     error
}

func newCUDARuntime() cudaRuntime { return &nativeCUDARuntime{} }

func (r *nativeCUDARuntime) Initialize() error {
	if r.initialized {
		return r.initErr
	}
	r.initialized = true
	device, err := currentCUDADeviceOrdinal()
	if err != nil {
		r.initErr = err
		return err
	}
	r.device = device
	r.available = true
	return nil
}
func (r *nativeCUDARuntime) Available() bool                      { return r.available }
func (r *nativeCUDARuntime) CUDADeviceOrdinal() (int, bool)       { return r.device, r.available }
func (r *nativeCUDARuntime) OpenCLContext() (OpenCLContext, bool) { return OpenCLContext{}, false }

type CUDAHashBackend struct {
	runtime  cudaRuntime
	scratch  *pooledScratch
	lastPlan GPUExecutionPlan
}

func (b *CUDAHashBackend) Mode() BackendMode { return BackendGPU }
func (b *CUDAHashBackend) Description() string {
	return "cuda backend that prepares cuda-managed DAG residency, but still reports host-reference execution until a dedicated CUDA LatticeHash kernel is implemented"
}
func (b *CUDAHashBackend) InitializeRuntime() error {
	if b.runtime == nil {
		b.runtime = newCUDARuntime()
	}
	return b.runtime.Initialize()
}
func (b *CUDAHashBackend) CUDADeviceOrdinal() (int, bool) {
	if b.runtime == nil {
		return 0, false
	}
	return b.runtime.CUDADeviceOrdinal()
}
func (b *CUDAHashBackend) OpenCLContext() (OpenCLContext, bool) { return OpenCLContext{}, false }
func (b *CUDAHashBackend) Prepare(dag *DAG) error {
	if dag == nil {
		return ErrNilDAG
	}
	if b.runtime == nil {
		b.runtime = newCUDARuntime()
	}
	if err := b.runtime.Initialize(); err != nil {
		b.lastPlan = GPUExecutionPlan{KernelName: "colossusx_cuda_hash", MemoryModel: GPUMemoryModelUnified, Fallback: "runtime-unavailable", UsedFallback: true, ExecutionBackend: "cuda", ExecutionPath: GPUExecutionPathHostReference}
		return err
	}
	if dag.AllocationName() != "cuda-managed" {
		return fmt.Errorf("cuda backend requires cuda-managed DAG allocation")
	}
	if _, err := newRawContiguousDAGBuffer(dag); err != nil {
		return err
	}
	if b.scratch == nil {
		b.scratch = newPooledScratch()
	}
	b.lastPlan = GPUExecutionPlan{KernelName: "colossusx_cuda_hash", BatchNonces: 1024, MemoryModel: GPUMemoryModelUnified, Fallback: "cpu-reference", UsedFallback: true, CopiedDAG: false, ExecutionBackend: "cuda", ExecutionPath: GPUExecutionPathHostReference, DeviceDAGCopyPerformed: false, DeviceDispatchAttempted: false}
	return nil
}
func (b *CUDAHashBackend) Hash(header []byte, nonce cx.Nonce, dag *DAG) HashResult {
	results, err := b.HashBatch(header, nonce, 1, dag)
	if err != nil || len(results) == 0 {
		return HashResult{}
	}
	return results[0]
}
func (b *CUDAHashBackend) HashBatch(header []byte, startNonce cx.Nonce, count uint64, dag *DAG) ([]HashResult, error) {
	if b.runtime == nil || !b.runtime.Available() {
		b.lastPlan.UsedFallback = true
		b.lastPlan.ExecutionPath = GPUExecutionPathHostReference
		return nil, fmt.Errorf("cuda runtime unavailable")
	}
	raw, err := newRawContiguousDAGBuffer(dag)
	if err != nil {
		return nil, err
	}
	view, err := newUnifiedMemoryDAGViewFromBytes(dag.Spec(), raw.Bytes)
	if err != nil {
		return nil, err
	}
	results := make([]HashResult, 0, count)
	for i := uint64(0); i < count; i++ {
		nonce, ok := startNonce.AddUint64(i)
		if !ok {
			break
		}
		s := b.scratch.acquire(len(header))
		results = append(results, latticeHashWithAccessor(dag.Spec(), header, nonce, view, s))
		b.scratch.release(s)
	}
	b.lastPlan.UsedFallback = true
	b.lastPlan.ExecutionPath = GPUExecutionPathHostReference
	b.lastPlan.ExecutionBackend = "cpu-reference"
	b.lastPlan.DeviceDispatchAttempted = false
	return results, nil
}
func (b *CUDAHashBackend) ExecutionPlan() GPUExecutionPlan { return b.lastPlan }
