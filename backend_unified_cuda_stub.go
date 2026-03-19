//go:build !cuda || !cgo

package main

func allocCUDAManaged(size uint64) (managedAllocation, error) {
	_ = size
	return nil, ErrNotImplemented("cuda managed memory requires a cgo + cuda build")
}
