package main

type UnifiedBackend struct {
	scratch *pooledScratch
}

func (b *UnifiedBackend) Mode() BackendMode { return BackendUnified }
func (b *UnifiedBackend) Description() string {
	return "unified memory backend with contiguous DAG traversal and pooled hashing scratch"
}
func (b *UnifiedBackend) Prepare(*DAG) error {
	if b.scratch == nil {
		b.scratch = newPooledScratch()
	}
	return nil
}
func (b *UnifiedBackend) Hash(header []byte, nonce uint64, dag *DAG) HashResult {
	s := b.scratch.acquire(len(header))
	defer b.scratch.release(s)
	return latticeHashWithAccessor(header, nonce, dag.spec.ReadsPerHash, contiguousDAGView{dag: dag}, s)
}
