package main

import (
	"encoding/binary"
	"errors"
	"sync"

	"github.com/zeebo/blake3"
	"golang.org/x/crypto/sha3"
)

var ErrNilDAG = errors.New("backend requires a dag")

type hashScratch struct {
	seedInput  []byte
	finalInput [96]byte
	fnvInput   [40]byte
	blakeInput [64]byte
}

func newHashScratch(headerLen int) *hashScratch {
	return &hashScratch{seedInput: make([]byte, headerLen+8)}
}

func ensureSeedInput(s *hashScratch, headerLen int) {
	if cap(s.seedInput) < headerLen+8 {
		s.seedInput = make([]byte, headerLen+8)
		return
	}
	s.seedInput = s.seedInput[:headerLen+8]
}

type dagAccessor interface {
	NodeCount() uint64
	ReadNode(uint64, *[64]byte)
}

func latticeHashWithAccessor(header []byte, nonce uint64, readsPerHash uint64, accessor dagAccessor, scratch *hashScratch) HashResult {
	var out HashResult
	if accessor.NodeCount() == 0 {
		return out
	}
	ensureSeedInput(scratch, len(header))
	copy(scratch.seedInput, header)
	binary.LittleEndian.PutUint64(scratch.seedInput[len(header):], nonce)
	seed512 := sha3.Sum512(scratch.seedInput)

	var mix [32]byte
	copy(mix[:], seed512[:32])

	var node [64]byte
	for r := uint64(0); r < readsPerHash; r++ {
		copy(scratch.fnvInput[:32], mix[:])
		binary.LittleEndian.PutUint64(scratch.fnvInput[32:], r)

		nodeIdx := fnv1a64(scratch.fnvInput[:]) % accessor.NodeCount()
		accessor.ReadNode(nodeIdx, &node)

		for i := 0; i < 32; i++ {
			scratch.blakeInput[i] = mix[i] ^ node[i]
			scratch.blakeInput[32+i] = node[32+i]
		}

		sum := blake3.Sum256(scratch.blakeInput[:])
		copy(mix[:], sum[:])
	}

	copy(scratch.finalInput[:64], seed512[:])
	copy(scratch.finalInput[64:], mix[:])
	final512 := sha3.Sum512(scratch.finalInput[:])
	copy(out.Full512[:], final512[:])
	copy(out.Pow256[:], final512[:32])
	return out
}

type contiguousDAGView struct{ dag *DAG }

func (v contiguousDAGView) NodeCount() uint64 { return v.dag.NodeCount() }
func (v contiguousDAGView) ReadNode(i uint64, out *[64]byte) {
	copy(out[:], v.dag.Node(i))
}

type unifiedMemoryDAGView struct {
	buf       []byte
	nodeSize  uint64
	nodeCount uint64
}

func newUnifiedMemoryDAGView(dag *DAG) (unifiedMemoryDAGView, error) {
	if dag == nil {
		return unifiedMemoryDAGView{}, ErrNilDAG
	}
	return newUnifiedMemoryDAGViewFromBytes(dag.spec, dag.Bytes())
}

func newUnifiedMemoryDAGViewFromBytes(spec Spec, buf []byte) (unifiedMemoryDAGView, error) {
	if uint64(len(buf)) < spec.DAGSizeBytes {
		return unifiedMemoryDAGView{}, errors.New("managed allocation is smaller than the DAG")
	}
	return unifiedMemoryDAGView{
		buf:       buf[:spec.DAGSizeBytes],
		nodeSize:  spec.NodeSize,
		nodeCount: spec.NodeCount(),
	}, nil
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
