package main

type UnifiedBackend struct {
	shared  unifiedMemoryDAGView
	scratch *pooledScratch
}

func (b *UnifiedBackend) Mode() BackendMode { return BackendUnified }
func (b *UnifiedBackend) Description() string {
	return "unified memory miner with a contiguous DAG buffer designed for CPU/GPU shared-memory access"
}
func (b *UnifiedBackend) Prepare(dag *DAG) error {
	shared, err := newUnifiedMemoryDAGView(dag)
	if err != nil {
		return err
	}
	b.shared = shared
	if b.scratch == nil {
		b.scratch = newPooledScratch()
	}
	return nil
}
func (b *UnifiedBackend) Hash(header []byte, nonce uint64, dag *DAG) HashResult {
	if b.shared.NodeCount() == 0 {
		if err := b.Prepare(dag); err != nil {
			return HashResult{}
		}
	}
	s := b.scratch.acquire(len(header))
	defer b.scratch.release(s)
	return latticeHashWithAccessor(header, nonce, dag.spec.ReadsPerHash, b.shared, s)
}
