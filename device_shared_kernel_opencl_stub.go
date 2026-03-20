//go:build !(cgo && opencl)

package main

func newOpenCLSharedAllocationKernel() openCLSharedAllocationKernel { return nil }
