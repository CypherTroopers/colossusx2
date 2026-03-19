package main

import (
	"fmt"
	"strings"

	cx "colossusx/colossusx"
)

type managedAllocation = cx.Allocation

type MemoryStrategy interface {
	cx.Allocator
}

type fallbackMemoryStrategy struct {
	name       string
	strategies []MemoryStrategy
}

func (m fallbackMemoryStrategy) Alloc(size uint64) (cx.Allocation, error) {
	var errs []string
	for _, strategy := range m.strategies {
		if strategy == nil {
			continue
		}
		alloc, err := strategy.Alloc(size)
		if err == nil {
			return alloc, nil
		}
		errs = append(errs, fmt.Sprintf("%s: %v", strategy.Name(), err))
	}
	if len(errs) == 0 {
		return nil, fmt.Errorf("%s: no allocation strategies configured", m.Name())
	}
	return nil, fmt.Errorf("%s: %s", m.Name(), strings.Join(errs, "; "))
}

func (m fallbackMemoryStrategy) Name() string {
	if m.name == "" {
		return "fallback"
	}
	return m.name
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

type GoHeapMemory struct{}

func (GoHeapMemory) Alloc(size uint64) (cx.Allocation, error) {
	return &sliceAllocation{name: "go-heap", buf: make([]byte, size)}, nil
}
func (GoHeapMemory) Name() string { return "go-heap" }

type PinnedMemory struct{}

func (PinnedMemory) Alloc(size uint64) (cx.Allocation, error) {
	_ = size
	return nil, ErrNotImplemented("pinned memory requires platform-specific implementation")
}
func (PinnedMemory) Name() string { return "pinned-host" }

type CUDAManagedMemory struct{}

func (m CUDAManagedMemory) Alloc(size uint64) (cx.Allocation, error) {
	return allocCUDAManaged(size)
}
func (CUDAManagedMemory) Name() string { return "cuda-managed" }

type OpenCLSVM struct {
	Context OpenCLContext
}

func (m OpenCLSVM) Alloc(size uint64) (cx.Allocation, error) {
	return allocOpenCLSVM(m.Context, size)
}
func (OpenCLSVM) Name() string { return "opencl-svm" }

type notImplementedError string

func (e notImplementedError) Error() string { return "not implemented: " + string(e) }
func ErrNotImplemented(s string) error      { return notImplementedError(s) }

func selectDAGStrategy(backend BackendMode, dagAlloc string) (MemoryStrategy, error) {
	choice := strings.ToLower(strings.TrimSpace(dagAlloc))
	if choice == "" {
		choice = "auto"
	}
	if choice == "auto" {
		switch backend {
		case BackendCPU, BackendUnified, BackendGPU:
			return fallbackMemoryStrategy{
				name: "auto",
				strategies: []MemoryStrategy{
					CUDAManagedMemory{},
					OpenCLSVM{},
					GoHeapMemory{},
				},
			}, nil
		default:
			return nil, fmt.Errorf("unsupported backend %q", backend)
		}
	}
	switch choice {
	case "go", "go-heap":
		return GoHeapMemory{}, nil
	case "pinned", "pinned-host":
		return PinnedMemory{}, nil
	case "cuda", "cuda-managed":
		return CUDAManagedMemory{}, nil
	case "opencl", "opencl-svm", "svm":
		return OpenCLSVM{}, nil
	default:
		return nil, fmt.Errorf("unsupported dag allocation strategy %q (expected one of: auto, go-heap, pinned-host, cuda-managed, opencl-svm)", dagAlloc)
	}
}
