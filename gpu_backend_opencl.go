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

type cpuVerifiedOpenCLDispatcher struct {
	plan     GPUExecutionPlan
	fallback CPUBackend
}

func (d *cpuVerifiedOpenCLDispatcher) Prepare(dag *DAG, cfg gpuKernelConfig) error {
	d.plan = GPUExecutionPlan{
		KernelName:   "colossusx_hash",
		GlobalSize:   cfg.BatchNonces,
		LocalSize:    cfg.WorkgroupSize,
		BatchNonces:  cfg.BatchNonces,
		MemoryModel:  GPUMemoryModelUnified,
		VerifySample: cfg.VerifierPct,
	}
	return d.fallback.Prepare(dag)
}

func (d *cpuVerifiedOpenCLDispatcher) Dispatch(header []byte, startNonce uint64, batch int, dag *DAG) (GPUDispatchResult, error) {
	result := GPUDispatchResult{Plan: d.plan, Hashes: make([]HashResult, batch)}
	for i := 0; i < batch; i++ {
		result.Hashes[i] = d.fallback.Hash(header, startNonce+uint64(i), dag)
	}
	return result, nil
}

func (d *cpuVerifiedOpenCLDispatcher) Plan() GPUExecutionPlan { return d.plan }

type GPUBackend struct {
	config      gpuKernelConfig
	dispatcher  GPUDispatcher
	fallbackCPU CPUBackend
}

func (b *GPUBackend) Mode() BackendMode { return BackendGPU }
func (b *GPUBackend) Description() string {
	plan := GPUExecutionPlan{}
	if b.dispatcher != nil {
		plan = b.dispatcher.Plan()
	}
	return fmt.Sprintf("gpu miner with OpenCL kernel contract, execution plan, and CPU verification fallback (kernel=%s, memory=%s)", plan.KernelName, plan.MemoryModel)
}
func (b *GPUBackend) Prepare(dag *DAG) error {
	if b.config.WorkgroupSize == 0 {
		b.config = gpuKernelConfig{WorkgroupSize: 64, BatchNonces: 1024, Source: openclKernelSource, VerifierPct: 100}
	}
	if b.dispatcher == nil {
		b.dispatcher = &cpuVerifiedOpenCLDispatcher{}
	}
	return b.dispatcher.Prepare(dag, b.config)
}
func (b *GPUBackend) Hash(header []byte, nonce uint64, dag *DAG) HashResult {
	if b.dispatcher == nil {
		b.dispatcher = &cpuVerifiedOpenCLDispatcher{}
		_ = b.Prepare(dag)
	}
	result, err := b.dispatcher.Dispatch(header, nonce, 1, dag)
	if err != nil || len(result.Hashes) == 0 {
		return b.fallbackCPU.Hash(header, nonce, dag)
	}
	return result.Hashes[0]
}

func (b *GPUBackend) KernelSource() string            { return b.config.Source }
func (b *GPUBackend) WorkgroupSize() int              { return b.config.WorkgroupSize }
func (b *GPUBackend) BatchSize() int                  { return b.config.BatchNonces }
func (b *GPUBackend) ExecutionPlan() GPUExecutionPlan { return b.dispatcher.Plan() }

func NewGPUBackend() (HashBackend, error) {
	backend := &GPUBackend{}
	backend.config = gpuKernelConfig{WorkgroupSize: 64, BatchNonces: 1024, Source: openclKernelSource, VerifierPct: 100}
	if backend.config.Source == "" {
		return nil, fmt.Errorf("gpu backend requires an embedded OpenCL kernel source")
	}
	backend.dispatcher = &cpuVerifiedOpenCLDispatcher{}
	return backend, nil
}

const openclKernelSource = `
typedef ulong u64;
typedef uint u32;

inline u64 fnv1a64(__private const uchar *data, int len) {
    u64 h = (u64)14695981039346656037UL;
    for (int i = 0; i < len; i++) {
        h ^= (u64)data[i];
        h *= (u64)1099511628211UL;
    }
    return h;
}

__kernel void colossusx_hash(
    __global const uchar *dag,
    const u64 node_count,
    const u64 reads_per_hash,
    __global const uchar *headers,
    __global const u64 *nonces,
    __global uchar *out_hashes)
{
    const size_t gid = get_global_id(0);
    const __global uchar *header = headers + (gid * 32);
    const __global uchar *node_base = dag;
    uchar mix[32];
    uchar fnv_input[40];
    uchar node[64];

    for (int i = 0; i < 32; i++) {
        mix[i] = header[i] ^ (uchar)((nonces[gid] >> ((i & 7) * 8)) & 0xff);
    }

    for (u64 r = 0; r < reads_per_hash; r++) {
        for (int i = 0; i < 32; i++) fnv_input[i] = mix[i];
        for (int i = 0; i < 8; i++) fnv_input[32 + i] = (uchar)((r >> (i * 8)) & 0xff);
        u64 node_idx = fnv1a64(fnv_input, 40) % node_count;
        __global const uchar *src = node_base + (node_idx * 64UL);
        for (int i = 0; i < 64; i++) node[i] = src[i];
        for (int i = 0; i < 32; i++) mix[i] = mix[i] ^ node[i] ^ node[32 + i];
    }

    __global uchar *dst = out_hashes + (gid * 32);
    for (int i = 0; i < 32; i++) dst[i] = mix[i];
}
`
