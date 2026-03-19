package main

import "fmt"

type UnifiedBackend struct {
	shared       unifiedMemoryDAGView
	scratch      *pooledScratch
	strategyName string
}

func (b *UnifiedBackend) Mode() BackendMode { return BackendUnified }

func (b *UnifiedBackend) Description() string {
	name := b.strategyName
	if name == "" {
		name = "go-heap"
	}
	return fmt.Sprintf("managed unified memory backend (dag-allocation=%s)", name)
}

func (b *UnifiedBackend) Prepare(dag *DAG) error {
	if dag == nil {
		return ErrNilDAG
	}
	shared, err := newUnifiedMemoryDAGViewFromBytes(dag.spec, dag.Bytes())
	if err != nil {
		return err
	}
	b.shared = shared
	if dag.alloc != nil {
		b.strategyName = dag.alloc.Name()
	}
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

func (b *UnifiedBackend) HashBatch(header []byte, startNonce uint64, count uint64, dag *DAG) ([]HashResult, error) {
	if b.shared.NodeCount() == 0 {
		if err := b.Prepare(dag); err != nil {
			return nil, err
		}
	}
	results := make([]HashResult, count)
	for i := uint64(0); i < count; i++ {
		results[i] = b.Hash(header, startNonce+i, dag)
	}
	return results, nil
}
