//go:build cgo && opencl

package main

/*
#cgo LDFLAGS: -lOpenCL
#include <stdlib.h>
#if defined(__has_include)
#  if __has_include(<CL/opencl.h>)
#    include <CL/opencl.h>
#    define COLOSSUSX_HAVE_OPENCL_HEADERS 1
#  endif
#endif
#ifndef COLOSSUSX_HAVE_OPENCL_HEADERS
    typedef void* cl_context;
    typedef void* cl_device_id;
    typedef unsigned long cl_svm_mem_flags;
    typedef unsigned long cl_device_svm_capabilities;
    typedef int cl_int;
    #define CL_SUCCESS 0
    #define CL_DEVICE_SVM_CAPABILITIES 0
    #define CL_DEVICE_SVM_COARSE_GRAIN_BUFFER 0
    #define CL_DEVICE_SVM_FINE_GRAIN_BUFFER 0
    static int colossusx_missing_opencl_headers(void) { return 1; }
    static int colossusx_has_svm(cl_device_id device) { (void)device; return 0; }
    static void* colossusx_svm_alloc(cl_context context, cl_svm_mem_flags flags, size_t size) { (void)context; (void)flags; (void)size; return NULL; }
    static void colossusx_svm_free(cl_context context, void *ptr) { (void)context; (void)ptr; }
#else
    static int colossusx_missing_opencl_headers(void) { return 0; }
    static int colossusx_has_svm(cl_device_id device) {
        cl_device_svm_capabilities caps = 0;
        cl_int err = clGetDeviceInfo(device, CL_DEVICE_SVM_CAPABILITIES, sizeof(caps), &caps, NULL);
        if (err != CL_SUCCESS) return 0;
        return (caps & (CL_DEVICE_SVM_COARSE_GRAIN_BUFFER | CL_DEVICE_SVM_FINE_GRAIN_BUFFER)) != 0;
    }
    static void* colossusx_svm_alloc(cl_context context, cl_svm_mem_flags flags, size_t size) { return clSVMAlloc(context, flags, size, 0); }
    static void colossusx_svm_free(cl_context context, void *ptr) { clSVMFree(context, ptr); }
#endif
*/
import "C"
import (
	"fmt"
	"unsafe"
)

type OpenCLContext struct {
	Context unsafe.Pointer
	Device  unsafe.Pointer
	Flags   uint64
}

func (c OpenCLContext) valid() bool {
	return c.Context != nil && c.Device != nil
}

func allocOpenCLSVM(ctx OpenCLContext, size uint64) (managedAllocation, error) {
	if C.colossusx_missing_opencl_headers() != 0 {
		return nil, fmt.Errorf("opencl headers were not found at build time")
	}
	if !ctx.valid() {
		return nil, fmt.Errorf("opencl svm requires a live OpenCL context and device")
	}
	device := C.cl_device_id(ctx.Device)
	if C.colossusx_has_svm(device) == 0 {
		return nil, fmt.Errorf("opencl device does not advertise SVM support")
	}
	ptr := C.colossusx_svm_alloc(C.cl_context(ctx.Context), C.cl_svm_mem_flags(ctx.Flags), C.size_t(size))
	if ptr == nil {
		return nil, fmt.Errorf("clSVMAlloc(%d) returned NULL", size)
	}
	buf := unsafe.Slice((*byte)(ptr), int(size))
	return &sliceAllocation{
		name: "opencl-svm",
		buf:  buf,
		free: func() error {
			C.colossusx_svm_free(C.cl_context(ctx.Context), ptr)
			return nil
		},
	}, nil
}
