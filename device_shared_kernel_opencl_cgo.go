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
    typedef void* cl_command_queue;
    typedef void* cl_kernel;
    typedef unsigned int cl_uint;
    typedef unsigned long cl_ulong;
    typedef int cl_int;
    #define CL_SUCCESS 0
    static const char* colossusx_opencl_enqueue_shared_hash(
        cl_command_queue queue, cl_kernel kernel,
        const void *header, cl_uint header_len,
        cl_ulong nonce_lo, cl_ulong count,
        cl_uint node_size, cl_ulong node_count, cl_ulong reads_per_hash,
        void *out
    ) {
        (void)queue; (void)kernel; (void)header; (void)header_len;
        (void)nonce_lo; (void)count; (void)node_size; (void)node_count;
        (void)reads_per_hash; (void)out;
        return "opencl headers were not found at build time";
    }
#else
    static const char* colossusx_opencl_enqueue_shared_hash(
        cl_command_queue queue, cl_kernel kernel,
        const void *header, cl_uint header_len,
        cl_ulong nonce_lo, cl_ulong count,
        cl_uint node_size, cl_ulong node_count, cl_ulong reads_per_hash,
        void *out
    ) {
        cl_int err = CL_SUCCESS;
        err = clSetKernelArg(kernel, 1, sizeof(void*), &header);
        if (err != CL_SUCCESS) return "clSetKernelArg(header) failed";
        err = clSetKernelArg(kernel, 2, sizeof(cl_uint), &header_len);
        if (err != CL_SUCCESS) return "clSetKernelArg(header_len) failed";
        err = clSetKernelArg(kernel, 3, sizeof(cl_ulong), &nonce_lo);
        if (err != CL_SUCCESS) return "clSetKernelArg(start_nonce) failed";
        err = clSetKernelArg(kernel, 4, sizeof(cl_uint), &node_size);
        if (err != CL_SUCCESS) return "clSetKernelArg(node_size) failed";
        err = clSetKernelArg(kernel, 5, sizeof(cl_ulong), &node_count);
        if (err != CL_SUCCESS) return "clSetKernelArg(node_count) failed";
        err = clSetKernelArg(kernel, 6, sizeof(cl_ulong), &reads_per_hash);
        if (err != CL_SUCCESS) return "clSetKernelArg(reads_per_hash) failed";
        err = clSetKernelArg(kernel, 7, sizeof(void*), &out);
        if (err != CL_SUCCESS) return "clSetKernelArg(out) failed";
        size_t global = (size_t)count;
        err = clEnqueueNDRangeKernel(queue, kernel, 1, NULL, &global, NULL, 0, NULL, NULL);
        if (err != CL_SUCCESS) return "clEnqueueNDRangeKernel failed";
        err = clFinish(queue);
        if (err != CL_SUCCESS) return "clFinish failed";
        return NULL;
    }
#endif
*/
import "C"
import (
	"fmt"
	"unsafe"

	cx "colossusx/colossusx"
)

type nativeOpenCLSharedAllocationKernel struct{}

func newOpenCLSharedAllocationKernel() openCLSharedAllocationKernel {
	return nativeOpenCLSharedAllocationKernel{}
}

func (nativeOpenCLSharedAllocationKernel) HashBatchOpenCL(ctx OpenCLContext, spec Spec, header []byte, startNonce cx.Nonce, count uint64, dag rawContiguousDAGBuffer) ([]HashResult, error) {
	if count == 0 {
		return nil, nil
	}
	if !ctx.valid() || ctx.Queue == nil || ctx.Kernel == nil {
		return nil, fmt.Errorf("opencl kernel dispatch requires a prepared context, queue, and kernel")
	}
	start, ok := nonceUint64(startNonce)
	if !ok {
		return nil, fmt.Errorf("opencl shared-allocation kernel currently requires uint64 nonces")
	}
	out := make([]HashResult, int(count))
	var headerPtr unsafe.Pointer
	if len(header) > 0 {
		headerPtr = unsafe.Pointer(&header[0])
	}
	if err := C.colossusx_opencl_enqueue_shared_hash(
		C.cl_command_queue(ctx.Queue),
		C.cl_kernel(ctx.Kernel),
		headerPtr,
		C.cl_uint(len(header)),
		C.cl_ulong(start),
		C.cl_ulong(count),
		C.cl_uint(dag.NodeSize),
		C.cl_ulong(dag.NodeCount),
		C.cl_ulong(spec.ReadsPerHash),
		unsafe.Pointer(&out[0]),
	); err != nil {
		return nil, fmt.Errorf("%s", C.GoString(err))
	}
	return out, nil
}

func nonceUint64(n cx.Nonce) (uint64, bool) {
	if n == nil {
		return 0, true
	}
	b := n.AppendTo(nil)
	if len(b) != 8 {
		return 0, false
	}
	var v uint64
	for i := 0; i < 8; i++ {
		v |= uint64(b[i]) << (8 * i)
	}
	return v, true
}
