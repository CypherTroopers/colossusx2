package types

import (
	"testing"

	cx "colossusx/colossusx"
)

func TestHeaderEncodingDeterministic(t *testing.T) {
	target, err := cx.ParseTargetHex("0fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
	if err != nil {
		t.Fatal(err)
	}
	h := BlockHeader{Version: 1, Height: 7, Timestamp: 42, Target: target, Nonce: 9, EpochSeed: EpochSeedForHeight(cx.ResearchSpec(8*1024*1024, 8, 16), 7), DAGSizeBytes: 8 * 1024 * 1024}
	if got, want := string(h.Encode()), string(h.Encode()); got != want {
		t.Fatalf("encoding not stable")
	}
	if h.HeaderHash() != h.HeaderHash() {
		t.Fatalf("header hash not stable")
	}
}
