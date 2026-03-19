package main

import (
	"errors"
	"sync"

	cx "colossusx/colossusx"
)

var ErrNilDAG = errors.New("backend requires a dag")

type hashScratch = cx.HashScratch

func newHashScratch(headerLen int) *hashScratch     { return cx.NewHashScratch(headerLen) }
func ensureSeedInput(s *hashScratch, headerLen int) { cx.EnsureSeedInput(s, headerLen) }

func latticeHashWithAccessor(spec Spec, header []byte, nonce uint64, accessor cx.DAGAccessor, scratch *hashScratch) HashResult {
	return cx.LatticeHash(spec, header, nonce, accessor, scratch)
}

type contiguousDAGView struct{ dag *DAG }

func (v contiguousDAGView) NodeCount() uint64                { return v.dag.NodeCount() }
func (v contiguousDAGView) ReadNode(i uint64, out *[64]byte) { v.dag.ReadNode(i, out) }

type unifiedMemoryDAGView struct {
	buf       []byte
	nodeSize  uint64
	nodeCount uint64
}

func newUnifiedMemoryDAGViewFromBytes(spec Spec, buf []byte) (unifiedMemoryDAGView, error) {
	if uint64(len(buf)) < spec.DAGSizeBytes {
		return unifiedMemoryDAGView{}, errors.New("managed allocation is smaller than the DAG")
	}
	return unifiedMemoryDAGView{buf: buf[:spec.DAGSizeBytes], nodeSize: spec.NodeSize, nodeCount: spec.NodeCount()}, nil
}

func (v unifiedMemoryDAGView) NodeCount() uint64 { return v.nodeCount }
func (v unifiedMemoryDAGView) ReadNode(i uint64, out *[64]byte) {
	off := i * v.nodeSize
	copy(out[:], v.buf[off:off+v.nodeSize])
}

type pooledScratch struct{ pool sync.Pool }

func newPooledScratch() *pooledScratch {
	return &pooledScratch{pool: sync.Pool{New: func() any { return newHashScratch(0) }}}
}

func (p *pooledScratch) acquire(headerLen int) *hashScratch {
	s := p.pool.Get().(*hashScratch)
	ensureSeedInput(s, headerLen)
	return s
}

func (p *pooledScratch) release(s *hashScratch) { p.pool.Put(s) }
