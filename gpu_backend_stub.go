//go:build !opencl && !cuda

package main

import cx "colossusx/colossusx"

type GPUBackend struct {
	cpuFallback CPUBackend
	lastPlan    GPUExecutionPlan
}

func (b *GPUBackend) Mode() BackendMode { return BackendGPU }

func (b *GPUBackend) Description() string {
	return "gpu backend using CPU reference hashing only because no GPU runtime build tags are enabled"
}
func (b *GPUBackend) InitializeRuntime() error             { return nil }
func (b *GPUBackend) CUDADeviceOrdinal() (int, bool)       { return 0, false }
func (b *GPUBackend) OpenCLContext() (OpenCLContext, bool) { return OpenCLContext{}, false }
func (b *GPUBackend) Prepare(dag *DAG) error               { return b.cpuFallback.Prepare(dag) }
func (b *GPUBackend) Hash(header []byte, nonce cx.Nonce, dag *DAG) HashResult {
	return b.cpuFallback.Hash(header, nonce, dag)
}
func (b *GPUBackend) HashBatch(header []byte, startNonce cx.Nonce, count uint64, dag *DAG) ([]HashResult, error) {
	b.lastPlan = GPUExecutionPlan{Fallback: "cpu-reference", UsedFallback: true}
	return b.cpuFallback.HashBatch(header, startNonce, count, dag)
}
func (b *GPUBackend) ExecutionPlan() GPUExecutionPlan { return b.lastPlan }

func NewGPUBackend() (HashBackend, error) {
	return &GPUBackend{}, nil
}
