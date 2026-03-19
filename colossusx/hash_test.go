package colossusx

import (
	"bytes"
	"testing"
)

type sliceAccessor struct {
	spec Spec
	buf  []byte
}

func (a sliceAccessor) NodeCount() uint64 { return a.spec.NodeCount() }
func (a sliceAccessor) ReadNode(i uint64, out *[64]byte) {
	off := i * a.spec.NodeSize
	copy(out[:], a.buf[off:off+a.spec.NodeSize])
}

func testSpec() Spec {
	return Spec{DAGSizeBytes: 64 * 16, NodeSize: NodeSize, ReadsPerHash: ReadsPerHash, EpochBlocks: EpochBlocks}
}

func TestGenerateDAGDeterministic(t *testing.T) {
	spec := testSpec()
	seed := []byte("0123456789abcdef0123456789abcdef")
	left := make([]byte, spec.DAGSizeBytes)
	right := make([]byte, spec.DAGSizeBytes)

	if err := GenerateDAG(spec, left, seed, 2); err != nil {
		t.Fatalf("GenerateDAG left: %v", err)
	}
	if err := GenerateDAG(spec, right, seed, 3); err != nil {
		t.Fatalf("GenerateDAG right: %v", err)
	}
	if !bytes.Equal(left, right) {
		t.Fatal("expected identical DAG output for the same seed")
	}
}

func TestLatticeHashDeterministic(t *testing.T) {
	spec := testSpec()
	seed := []byte("0123456789abcdef0123456789abcdef")
	dag := make([]byte, spec.DAGSizeBytes)
	if err := GenerateDAG(spec, dag, seed, 2); err != nil {
		t.Fatalf("GenerateDAG: %v", err)
	}
	accessor := sliceAccessor{spec: spec, buf: dag}
	header := []byte("header")
	nonce := uint64(42)

	first := LatticeHash(header, nonce, accessor, nil)
	second := LatticeHash(header, nonce, accessor, nil)
	if first != second {
		t.Fatalf("expected deterministic lattice hash; first=%x second=%x", first.Pow256, second.Pow256)
	}

	third := LatticeHash(header, nonce+1, accessor, nil)
	if first == third {
		t.Fatal("expected nonce change to alter lattice hash")
	}
}

func TestLessOrEqualBETargetComparison(t *testing.T) {
	target, err := ParseTargetHex("00ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
	if err != nil {
		t.Fatalf("ParseTargetHex: %v", err)
	}
	var lower [32]byte
	var equal [32]byte
	copy(equal[:], target[:])
	var higher [32]byte
	copy(higher[:], target[:])
	lower[31] = 1
	higher[0] = 0x01

	if !LessOrEqualBE(lower, target) {
		t.Fatal("expected lower digest to satisfy target")
	}
	if !LessOrEqualBE(equal, target) {
		t.Fatal("expected equal digest to satisfy target")
	}
	if LessOrEqualBE(higher, target) {
		t.Fatal("expected higher digest to fail target comparison")
	}
}
