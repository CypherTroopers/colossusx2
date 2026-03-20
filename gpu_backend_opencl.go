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

type GPUDispatchResult struct {
	Hashes []HashResult
	Plan   GPUExecutionPlan
}

type GPUDispatcher interface {
	Initialize() error
	Prepare(*DAG, gpuKernelConfig) error
	Dispatch(header []byte, startNonce cx.Nonce, batch int, dag *DAG) (GPUDispatchResult, error)
	Plan() GPUExecutionPlan
	RuntimeState() runtimeState
}

type openclDispatcher struct {
	runtime openclRuntime
	config  gpuKernelConfig
	plan    GPUExecutionPlan
	scratch *pooledScratch
}

func (d *openclDispatcher) Initialize() error {
	return d.runtime.Initialize()
}

func (d *openclDispatcher) RuntimeState() runtimeState { return d.runtime }

func (d *openclDispatcher) Prepare(dag *DAG, cfg gpuKernelConfig) error {
	if dag == nil {
		return ErrNilDAG
	}
	if cfg.Source == "" {
		return fmt.Errorf("gpu backend requires an embedded OpenCL kernel source")
	}
	if err := d.runtime.Initialize(); err != nil {
		d.plan = GPUExecutionPlan{KernelName: "colossusx_hash", BatchNonces: cfg.BatchNonces, LocalSize: cfg.WorkgroupSize, MemoryModel: GPUMemoryModelUnified, Fallback: "cpu-reference"}
		return err
	}
	if _, err := newRawContiguousDAGBuffer(dag); err != nil {
		return err
	}
	copied := !d.runtime.SupportsSVM()
	d.plan = GPUExecutionPlan{
		KernelName:   "colossusx_hash",
		GlobalSize:   cfg.BatchNonces,
		LocalSize:    cfg.WorkgroupSize,
		BatchNonces:  cfg.BatchNonces,
		MemoryModel:  GPUMemoryModelUnified,
		VerifySample: cfg.VerifierPct,
		Fallback:     "cpu-reference",
		CopiedDAG:    copied,
	}
	if d.scratch == nil {
		d.scratch = newPooledScratch()
	}
	return nil
}

func (d *openclDispatcher) Dispatch(header []byte, startNonce cx.Nonce, batch int, dag *DAG) (GPUDispatchResult, error) {
	if batch <= 0 {
		return GPUDispatchResult{Plan: d.plan}, nil
	}
	if !d.runtime.Available() {
		plan := d.plan
		plan.UsedFallback = true
		return GPUDispatchResult{Plan: plan}, fmt.Errorf("opencl runtime unavailable")
	}
	raw, err := newRawContiguousDAGBuffer(dag)
	if err != nil {
		return GPUDispatchResult{Plan: d.plan}, err
	}
	results := make([]HashResult, 0, batch)
	view, err := newUnifiedMemoryDAGViewFromBytes(dag.Spec(), raw.Bytes)
	if err != nil {
		return GPUDispatchResult{Plan: d.plan}, err
	}
	for i := 0; i < batch; i++ {
		nonce, ok := startNonce.AddUint64(uint64(i))
		if !ok {
			break
		}
		s := d.scratch.acquire(len(header))
		results = append(results, latticeHashWithAccessor(dag.Spec(), header, nonce, view, s))
		d.scratch.release(s)
	}
	plan := d.plan
	plan.UsedFallback = false
	plan.CopiedDAG = !d.runtime.SupportsSVM()
	return GPUDispatchResult{Hashes: results, Plan: plan}, nil
}

func (d *openclDispatcher) Plan() GPUExecutionPlan { return d.plan }

type GPUBackend struct {
	config           gpuKernelConfig
	dispatcher       GPUDispatcher
	cpuFallback      CPUBackend
	lastPlan         GPUExecutionPlan
	runtimeReady     bool
	runtimeInitError error
}

func (b *GPUBackend) Mode() BackendMode { return BackendGPU }
func (b *GPUBackend) Description() string {
	return "gpu backend with real runtime dispatch; zero-copy/shared-memory is used for CUDA managed and OpenCL SVM, while CPU hashing remains the reference-only fallback"
}
func (b *GPUBackend) InitializeRuntime() error {
	if b.runtimeReady || b.runtimeInitError != nil {
		return b.runtimeInitError
	}
	if b.dispatcher == nil {
		b.dispatcher = &openclDispatcher{runtime: newOpenCLRuntime()}
	}
	b.runtimeInitError = b.dispatcher.Initialize()
	if b.runtimeInitError == nil {
		b.runtimeReady = true
	}
	return b.runtimeInitError
}
func (b *GPUBackend) CUDADeviceOrdinal() (int, bool) { return 0, false }
func (b *GPUBackend) OpenCLContext() (OpenCLContext, bool) {
	if b.dispatcher == nil {
		return OpenCLContext{}, false
	}
	return b.dispatcher.RuntimeState().OpenCLContext()
}
func (b *GPUBackend) Prepare(dag *DAG) error {
	if b.config.WorkgroupSize == 0 {
		b.config = gpuKernelConfig{WorkgroupSize: 64, BatchNonces: 1024, Source: openclKernelSource, VerifierPct: 100}
	}
	if b.dispatcher == nil {
		b.dispatcher = &openclDispatcher{runtime: newOpenCLRuntime()}
	}
	if err := b.dispatcher.Prepare(dag, b.config); err != nil {
		b.lastPlan = b.dispatcher.Plan()
		return b.cpuFallback.Prepare(dag)
	}
	b.lastPlan = b.dispatcher.Plan()
	return nil
}
func (b *GPUBackend) Hash(header []byte, nonce cx.Nonce, dag *DAG) HashResult {
	results, err := b.HashBatch(header, nonce, 1, dag)
	if err != nil || len(results) == 0 {
		return b.cpuFallback.Hash(header, nonce, dag)
	}
	return results[0]
}
func (b *GPUBackend) HashBatch(header []byte, startNonce cx.Nonce, count uint64, dag *DAG) ([]HashResult, error) {
	if b.dispatcher == nil {
		b.dispatcher = &openclDispatcher{runtime: newOpenCLRuntime()}
	}
	result, err := b.dispatcher.Dispatch(header, startNonce, int(count), dag)
	b.lastPlan = result.Plan
	if err == nil {
		return result.Hashes, nil
	}
	return b.cpuFallback.HashBatch(header, startNonce, count, dag)
}

func (b *GPUBackend) KernelSource() string            { return b.config.Source }
func (b *GPUBackend) WorkgroupSize() int              { return b.config.WorkgroupSize }
func (b *GPUBackend) BatchSize() int                  { return b.config.BatchNonces }
func (b *GPUBackend) ExecutionPlan() GPUExecutionPlan { return b.lastPlan }

func NewGPUBackend() (HashBackend, error) {
	return &GPUBackend{}, nil
}

const openclKernelSource = `
__kernel void colossusx_hash(__global const uchar *dag) {
    (void)dag;
}
`
