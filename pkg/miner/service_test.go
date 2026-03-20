package miner

import (
	"testing"
	"time"

	cx "colossusx/colossusx"
	"colossusx/pkg/mining"
	"colossusx/pkg/types"
)

func testSpec() cx.Spec {
	return cx.ResearchSpec(1024*1024, 8, 8)
}

func testTarget(t *testing.T) cx.Target {
	t.Helper()
	target, err := cx.ParseTargetHex("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
	if err != nil {
		t.Fatal(err)
	}
	return target
}

func testBlock(spec cx.Spec, target cx.Target, height uint64) types.Block {
	return types.Block{Header: types.BlockHeader{
		Version:      1,
		Height:       height,
		Timestamp:    time.Now().Unix(),
		Target:       target,
		EpochSeed:    types.EpochSeedForHeight(spec, height),
		DAGSizeBytes: spec.DAGSizeBytes,
	}}
}

func TestServiceSealBlockUsesSelectedBackend(t *testing.T) {
	backend, err := mining.NewBackend("unified")
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(testSpec(), 1, backend, mining.GoHeapMemory{})
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	sealed, res, err := svc.SealBlock(testBlock(testSpec(), testTarget(t), 0), 32)
	if err != nil {
		t.Fatal(err)
	}
	if sealed.Header.Nonce == 0 && res.Hashes == 0 {
		t.Fatalf("expected mining result")
	}
	if res.Backend != cx.BackendUnified {
		t.Fatalf("expected unified backend, got %s", res.Backend)
	}
}

func TestServiceCachesDAGByEpochSeedAndSize(t *testing.T) {
	backend, err := mining.NewBackend("cpu")
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(testSpec(), 1, backend, mining.GoHeapMemory{})
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	headerA := testBlock(testSpec(), testTarget(t), 0).Header
	dag1, err := svc.dagForHeader(headerA)
	if err != nil {
		t.Fatal(err)
	}
	dag2, err := svc.dagForHeader(headerA)
	if err != nil {
		t.Fatal(err)
	}
	if dag1 != dag2 {
		t.Fatal("expected identical seed+size to reuse dag")
	}

	headerB := headerA
	headerB.DAGSizeBytes += headerB.DAGSizeBytes
	dag3, err := svc.dagForHeader(headerB)
	if err != nil {
		t.Fatal(err)
	}
	if dag3 == dag1 {
		t.Fatal("expected size change to create a distinct dag")
	}

	headerC := headerA
	headerC.EpochSeed = types.EpochSeedForHeight(testSpec(), testSpec().EpochBlocks)
	dag4, err := svc.dagForHeader(headerC)
	if err != nil {
		t.Fatal(err)
	}
	if dag4 == dag1 {
		t.Fatal("expected seed change to create a distinct dag")
	}
}
