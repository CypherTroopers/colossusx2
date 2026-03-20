//go:build !opencl || !cgo

package main

type OpenCLContext struct {
	Context any
	Device  any
	Queue   any
	Program any
	Kernel  any
	Flags   uint64
}

func (c OpenCLContext) valid() bool {
	return c.Context != nil && c.Device != nil
}

type openclRuntime interface {
	runtimeState
	Initialize() error
	Available() bool
	SupportsSVM() bool
}

type stubOpenCLRuntime struct{}

func newOpenCLRuntime() openclRuntime { return stubOpenCLRuntime{} }
func (stubOpenCLRuntime) Initialize() error {
	return ErrNotImplemented("opencl svm requires a cgo + opencl build")
}
func (stubOpenCLRuntime) Available() bool                { return false }
func (stubOpenCLRuntime) SupportsSVM() bool              { return false }
func (stubOpenCLRuntime) CUDADeviceOrdinal() (int, bool) { return 0, false }
func (stubOpenCLRuntime) OpenCLContext() (OpenCLContext, bool) {
	return OpenCLContext{}, false
}

func allocOpenCLSVM(ctx OpenCLContext, size uint64) (managedAllocation, error) {
	_, _ = ctx, size
	return nil, ErrNotImplemented("opencl svm requires a cgo + opencl build")
}

func buildOpenCLProgram(ctx OpenCLContext, source string) (OpenCLContext, error) {
	_, _ = ctx, source
	return OpenCLContext{}, ErrNotImplemented("opencl build helpers require a cgo + opencl build")
}

func setOpenCLSVMKernelArg(ctx OpenCLContext, index uint32, ptr any) error {
	_, _, _ = ctx, index, ptr
	return ErrNotImplemented("opencl svm arg helpers require a cgo + opencl build")
}
