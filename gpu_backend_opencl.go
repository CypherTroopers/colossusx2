//go:build opencl

package main

import "fmt"

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
}

type GPUDispatchResult struct {
	Hashes []HashResult
	Plan   GPUExecutionPlan
}

type GPUDispatcher interface {
	Prepare(*DAG, gpuKernelConfig) error
	Dispatch(header []byte, startNonce uint64, batch int, dag *DAG) (GPUDispatchResult, error)
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
		MemoryModel:  GPUMemoryModelDiscrete,
		VerifySample: cfg.VerifierPct,
	}
	if cfg.Source == "" {
		return fmt.Errorf("gpu backend requires an embedded OpenCL kernel source")
	}
	return ErrNotImplemented("OpenCL kernel source is not yet hash-equivalent to CPU reference implementation")
}

func (d *openclDispatcherStub) Dispatch(header []byte, startNonce uint64, batch int, dag *DAG) (GPUDispatchResult, error) {
	_, _, _, _ = header, startNonce, batch, dag
	return GPUDispatchResult{}, ErrNotImplemented("OpenCL dispatch is not implemented")
}
func (d *openclDispatcherStub) Plan() GPUExecutionPlan { return d.plan }

type GPUBackend struct {
	config     gpuKernelConfig
	dispatcher GPUDispatcher
}

func (b *GPUBackend) Mode() BackendMode { return BackendGPU }
func (b *GPUBackend) Description() string {
	return "gpu miner disabled until the OpenCL kernel matches the CPU reference implementation"
}
func (b *GPUBackend) Prepare(dag *DAG) error {
	if b.config.WorkgroupSize == 0 {
		b.config = gpuKernelConfig{WorkgroupSize: 64, BatchNonces: 1024, Source: openclKernelSource, VerifierPct: 100}
	}
	if b.dispatcher == nil {
		b.dispatcher = &openclDispatcherStub{}
	}
	return b.dispatcher.Prepare(dag, b.config)
}
func (b *GPUBackend) Hash(header []byte, nonce uint64, dag *DAG) HashResult {
	results, err := b.HashBatch(header, nonce, 1, dag)
	if err != nil || len(results) == 0 {
		return HashResult{}
	}
	return results[0]
}
func (b *GPUBackend) HashBatch(header []byte, startNonce uint64, count uint64, dag *DAG) ([]HashResult, error) {
	if b.dispatcher == nil {
		b.dispatcher = &openclDispatcherStub{}
	}
	result, err := b.dispatcher.Dispatch(header, startNonce, int(count), dag)
	if err != nil {
		return nil, err
	}
	return result.Hashes, nil
}

func (b *GPUBackend) KernelSource() string            { return b.config.Source }
func (b *GPUBackend) WorkgroupSize() int              { return b.config.WorkgroupSize }
func (b *GPUBackend) BatchSize() int                  { return b.config.BatchNonces }
func (b *GPUBackend) ExecutionPlan() GPUExecutionPlan { return b.dispatcher.Plan() }

func NewGPUBackend() (HashBackend, error) {
	return nil, ErrNotImplemented("GPU backend is not enabled until OpenCL kernel is hash-equivalent to CPU reference")
}

const openclKernelSource = `
// disabled: this kernel is not hash-equivalent to the CPU reference implementation
`
