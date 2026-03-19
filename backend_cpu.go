package main

import "fmt"

type cpuNode [64]byte

type cpuDAGView struct{ nodes []cpuNode }

func (v cpuDAGView) NodeCount() uint64                { return uint64(len(v.nodes)) }
func (v cpuDAGView) ReadNode(i uint64, out *[64]byte) { *out = [64]byte(v.nodes[i]) }

type CPUBackend struct {
	nodes   []cpuNode
	scratch *pooledScratch
}

func (b *CPUBackend) Mode() BackendMode { return BackendCPU }
func (b *CPUBackend) Description() string {
	return "cpu backend with a dedicated prepared node table and pooled worker scratch"
}
func (b *CPUBackend) Prepare(dag *DAG) error {
	if dag == nil {
		return fmt.Errorf("cpu backend requires a dag")
	}
	if b.scratch == nil {
		b.scratch = newPooledScratch()
	}
	count := dag.NodeCount()
	b.nodes = make([]cpuNode, count)
	for i := uint64(0); i < count; i++ {
		copy(b.nodes[i][:], dag.Node(i))
	}
	return nil
}
func (b *CPUBackend) Hash(header []byte, nonce uint64, dag *DAG) HashResult {
	if len(b.nodes) == 0 && dag != nil {
		_ = b.Prepare(dag)
	}
	s := b.scratch.acquire(len(header))
	defer b.scratch.release(s)
	return latticeHashWithAccessor(header, nonce, dag.spec.ReadsPerHash, cpuDAGView{nodes: b.nodes}, s)
}
