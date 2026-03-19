//go:build opencl

package main

type GPUBackend struct{}

func (GPUBackend) Mode() BackendMode { return BackendGPU }
func (GPUBackend) Description() string {
	return "experimental gpu backend for OpenCL-enabled builds (currently shares the CPU hash path until a dedicated kernel is wired in)"
}
func (GPUBackend) Prepare(*DAG) error { return nil }
func (GPUBackend) Hash(header []byte, nonce uint64, dag *DAG) HashResult {
	return LatticeHash(header, nonce, dag)
}

func NewGPUBackend() (HashBackend, error) {
	return GPUBackend{}, nil
}
