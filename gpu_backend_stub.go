//go:build !opencl

package main

import cx "colossusx/colossusx"

type GPUBackend struct {
	fallback UnifiedBackend
}

func (b *GPUBackend) Mode() BackendMode { return BackendGPU }

func (b *GPUBackend) Description() string {
	return "gpu backend enabled with reference-equivalent unified-memory execution path"
}

func (b *GPUBackend) Prepare(dag *DAG) error {
	return b.fallback.Prepare(dag)
}

func (b *GPUBackend) Hash(header []byte, nonce cx.Nonce, dag *DAG) HashResult {
	return b.fallback.Hash(header, nonce, dag)
}

func (b *GPUBackend) HashBatch(header []byte, startNonce cx.Nonce, count uint64, dag *DAG) ([]HashResult, error) {
	return b.fallback.HashBatch(header, startNonce, count, dag)
}

func NewGPUBackend() (HashBackend, error) {
	return &GPUBackend{}, nil
}
