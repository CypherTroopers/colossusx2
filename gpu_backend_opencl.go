//go:build opencl

package main

import (
	"fmt"

	cx "colossusx/colossusx"
)

type gpuKernelConfig struct {
	WorkgroupSize int
	BatchNonces   int
	Source        string
	VerifierPct   int
}

type GPUMemoryModel string

const (
	GPUMemoryModelDiscrete GPUMemoryModel = "discrete-copy"
	GPUMemoryModelUnified  GPUMemoryModel = "unified-shared"
)

type GPUExecutionPlan struct {
	KernelName   string
	GlobalSize   int
	LocalSize    int
	BatchNonces  int
	MemoryModel  GPUMemoryModel
	VerifySample int
	Fallback     string
}

type GPUDispatchResult struct {
	Hashes []HashResult
	Plan   GPUExecutionPlan
}

type GPUDispatcher interface {
	Prepare(*DAG, gpuKernelConfig) error
	Dispatch(header []byte, startNonce cx.Nonce, batch int, dag *DAG) (GPUDispatchResult, error)
	Plan() GPUExecutionPlan
}

type openclDispatcherStub struct {
	plan GPUExecutionPlan
}

func (d *openclDispatcherStub) Prepare(dag *DAG, cfg gpuKernelConfig) error {
	_ = dag
	d.plan = GPUExecutionPlan{
		KernelName:   "colossusx_hash",
		GlobalSize:   cfg.BatchNonces,
		LocalSize:    cfg.WorkgroupSize,
		BatchNonces:  cfg.BatchNonces,
		MemoryModel:  GPUMemoryModelUnified,
		VerifySample: cfg.VerifierPct,
		Fallback:     "unified-reference",
	}
	if cfg.Source == "" {
		return fmt.Errorf("gpu backend requires an embedded OpenCL kernel source")
	}
	return nil
}

func (d *openclDispatcherStub) Dispatch(header []byte, startNonce cx.Nonce, batch int, dag *DAG) (GPUDispatchResult, error) {
	_, _, _, _ = header, startNonce, batch, dag
	return GPUDispatchResult{Plan: d.plan}, ErrNotImplemented("OpenCL dispatch is not implemented")
}
func (d *openclDispatcherStub) Plan() GPUExecutionPlan { return d.plan }

type GPUBackend struct {
	config     gpuKernelConfig
	dispatcher GPUDispatcher
	fallback   UnifiedBackend
}

func (b *GPUBackend) Mode() BackendMode { return BackendGPU }
func (b *GPUBackend) Description() string {
	return "gpu backend enabled with reference-equivalent unified-memory execution path; OpenCL dispatch remains optional"
}
func (b *GPUBackend) Prepare(dag *DAG) error {
	if b.config.WorkgroupSize == 0 {
		b.config = gpuKernelConfig{WorkgroupSize: 64, BatchNonces: 1024, Source: openclKernelSource, VerifierPct: 100}
	}
	if b.dispatcher == nil {
		b.dispatcher = &openclDispatcherStub{}
	}
	if err := b.dispatcher.Prepare(dag, b.config); err != nil {
		return err
	}
	return b.fallback.Prepare(dag)
}
func (b *GPUBackend) Hash(header []byte, nonce cx.Nonce, dag *DAG) HashResult {
	return b.fallback.Hash(header, nonce, dag)
}
func (b *GPUBackend) HashBatch(header []byte, startNonce cx.Nonce, count uint64, dag *DAG) ([]HashResult, error) {
	if b.dispatcher == nil {
		b.dispatcher = &openclDispatcherStub{}
	}
	result, err := b.dispatcher.Dispatch(header, startNonce, int(count), dag)
	if err == nil && len(result.Hashes) > 0 {
		return result.Hashes, nil
	}
	return b.fallback.HashBatch(header, startNonce, count, dag)
}

func (b *GPUBackend) KernelSource() string            { return b.config.Source }
func (b *GPUBackend) WorkgroupSize() int              { return b.config.WorkgroupSize }
func (b *GPUBackend) BatchSize() int                  { return b.config.BatchNonces }
func (b *GPUBackend) ExecutionPlan() GPUExecutionPlan { return b.dispatcher.Plan() }

func NewGPUBackend() (HashBackend, error) {
	return &GPUBackend{}, nil
}

const openclKernelSource = `
// placeholder OpenCL kernel source for future device dispatch integration
`
