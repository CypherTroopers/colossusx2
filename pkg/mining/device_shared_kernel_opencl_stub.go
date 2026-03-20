//go:build !(cgo && opencl)

package mining

func newOpenCLSharedAllocationKernel() openCLSharedAllocationKernel { return nil }
