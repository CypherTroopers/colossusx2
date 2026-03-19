//go:build !opencl

package main

import "fmt"

type GPUBackend struct{}

func NewGPUBackend() (HashBackend, error) {
	return nil, fmt.Errorf("gpu backend requires an OpenCL-enabled build; rebuild with -tags opencl to enable the dedicated GPU kernel path")
}
