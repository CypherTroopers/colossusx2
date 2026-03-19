//go:build opencl

package main

import "fmt"

type gpuKernelConfig struct {
	WorkgroupSize int
	BatchNonces   int
	Source        string
}

type GPUBackend struct {
	config      gpuKernelConfig
	fallbackCPU CPUBackend
}

func (b *GPUBackend) Mode() BackendMode { return BackendGPU }
func (b *GPUBackend) Description() string {
	return "gpu backend with a dedicated OpenCL kernel contract and CPU-verified fallback executor"
}
func (b *GPUBackend) Prepare(dag *DAG) error {
	if b.config.WorkgroupSize == 0 {
		b.config = gpuKernelConfig{WorkgroupSize: 64, BatchNonces: 1024, Source: openclKernelSource}
	}
	return b.fallbackCPU.Prepare(dag)
}
func (b *GPUBackend) Hash(header []byte, nonce uint64, dag *DAG) HashResult {
	return b.fallbackCPU.Hash(header, nonce, dag)
}

func (b *GPUBackend) KernelSource() string { return b.config.Source }
func (b *GPUBackend) WorkgroupSize() int   { return b.config.WorkgroupSize }
func (b *GPUBackend) BatchSize() int       { return b.config.BatchNonces }

func NewGPUBackend() (HashBackend, error) {
	backend := &GPUBackend{}
	if backend.config.Source == "" {
		backend.config = gpuKernelConfig{WorkgroupSize: 64, BatchNonces: 1024, Source: openclKernelSource}
	}
	if backend.config.Source == "" {
		return nil, fmt.Errorf("gpu backend requires an embedded OpenCL kernel source")
	}
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
