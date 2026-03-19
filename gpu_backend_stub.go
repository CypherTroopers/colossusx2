//go:build !opencl

package main

func NewGPUBackend() (HashBackend, error) {
	return nil, ErrNotImplemented("GPU backend is not enabled until OpenCL kernel is hash-equivalent to CPU reference")
}
