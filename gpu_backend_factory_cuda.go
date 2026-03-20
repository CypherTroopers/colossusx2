//go:build cuda && !opencl

package main

func NewGPUBackend() (HashBackend, error) {
	return &CUDAHashBackend{}, nil
}
