//go:build !opencl || !cgo

package main

type OpenCLContext struct {
	Context any
	Device  any
	Flags   uint64
}

func allocOpenCLSVM(ctx OpenCLContext, size uint64) (managedAllocation, error) {
	_, _ = ctx, size
	return nil, ErrNotImplemented("opencl svm requires a cgo + opencl build")
}
