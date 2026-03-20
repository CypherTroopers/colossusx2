package consensus

import (
	"testing"
	"time"

	cx "colossusx/colossusx"
	"colossusx/pkg/chain"
	"colossusx/pkg/miner"
	"colossusx/pkg/mining"
	"colossusx/pkg/types"
)

func testConfig(t *testing.T) (types.ChainConfig, types.GenesisConfig) {
	t.Helper()
	spec := cx.ResearchSpec(1024*1024, 8, 8)
	target, err := cx.ParseTargetHex("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
	if err != nil {
		t.Fatal(err)
	}
	chainCfg := types.ChainConfig{NetworkID: "test", Spec: spec}
	genesis := types.GenesisConfig{ChainID: "test", Message: "test", Timestamp: time.Now().Unix() - 1, Bits: target, Spec: spec}
	return chainCfg, genesis
}

func TestValidatorInsertBlock(t *testing.T) {
	chainCfg, genesisCfg := testConfig(t)
	v, err := NewValidator(chainCfg, CPUBackend{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	store := chain.NewMemoryStore()
	minerSvc, err := miner.NewService(chainCfg.Spec, 1, &mining.CPUBackend{}, mining.GoHeapMemory{})
	if err != nil {
		t.Fatal(err)
	}
	defer minerSvc.Close()
	genesis, _, err := minerSvc.SealBlock(types.NewGenesisBlock(genesisCfg), 10)
	if err != nil {
		t.Fatal(err)
	}
	work, becameTip, err := v.InsertBlock(store, genesis)
	if err != nil {
		t.Fatal(err)
	}
	if !becameTip || work.Sign() <= 0 {
		t.Fatalf("expected genesis to become tip")
	}
	next := types.Block{Header: types.BlockHeader{Version: 1, Height: 1, ParentHash: genesis.BlockHash(), Timestamp: genesis.Header.Timestamp + 1, Target: genesisCfg.Bits, EpochSeed: types.EpochSeedForHeight(chainCfg.Spec, 1), DAGSizeBytes: chainCfg.Spec.DAGSizeBytes}}
	sealed, _, err := minerSvc.SealBlock(next, 10)
	if err != nil {
		t.Fatal(err)
	}
	if _, becameTip, err := v.InsertBlock(store, sealed); err != nil {
		t.Fatal(err)
	} else if !becameTip {
		t.Fatalf("expected child to become tip")
	}
}
