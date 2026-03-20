package node

import (
	"testing"
	"time"

	cx "colossusx/colossusx"
	"colossusx/pkg/chain"
	"colossusx/pkg/consensus"
	"colossusx/pkg/miner"
	"colossusx/pkg/mining"
	"colossusx/pkg/types"
)

func nodeTestConfig(t *testing.T) (Config, types.ChainConfig, types.GenesisConfig) {
	t.Helper()
	spec := cx.ResearchSpec(1024*1024, 8, 8)
	target, err := cx.ParseTargetHex("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
	if err != nil {
		t.Fatal(err)
	}
	chainCfg := types.ChainConfig{NetworkID: "testnet", Spec: spec}
	genesisCfg := types.GenesisConfig{ChainID: "testnet", Message: "genesis", Timestamp: time.Now().Unix(), Bits: target, Spec: spec}
	return Config{Chain: chainCfg, Genesis: genesisCfg, Mine: true, MaxNonces: 32, BlockTime: time.Millisecond}, chainCfg, genesisCfg
}

func TestNodeMiningUsesMinerServiceBackend(t *testing.T) {
	cfg, chainCfg, _ := nodeTestConfig(t)
	validator, err := consensus.NewValidator(chainCfg, consensus.CPUBackend{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer validator.Close()

	backend, err := mining.NewBackend("unified")
	if err != nil {
		t.Fatal(err)
	}
	minerSvc, err := miner.NewService(chainCfg.Spec, 1, backend, mining.GoHeapMemory{})
	if err != nil {
		t.Fatal(err)
	}
	defer minerSvc.Close()

	n, err := New(cfg, validator, minerSvc, chain.NewMemoryStore())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := n.InitGenesis(); err != nil {
		t.Fatal(err)
	}
	_, res, err := n.mineNextBlock()
	if err != nil {
		t.Fatal(err)
	}
	if res.Backend != cx.BackendUnified {
		t.Fatalf("expected node mining to use miner service backend unified, got %s", res.Backend)
	}
}

func TestValidatorRemainsIndependentFromMinerBackend(t *testing.T) {
	cfg, chainCfg, _ := nodeTestConfig(t)
	validator, err := consensus.NewValidator(chainCfg, consensus.CPUBackend{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer validator.Close()

	backend, err := mining.NewBackend("unified")
	if err != nil {
		t.Fatal(err)
	}
	minerSvc, err := miner.NewService(chainCfg.Spec, 1, backend, mining.GoHeapMemory{})
	if err != nil {
		t.Fatal(err)
	}
	defer minerSvc.Close()

	n, err := New(cfg, validator, minerSvc, chain.NewMemoryStore())
	if err != nil {
		t.Fatal(err)
	}
	genesis, err := n.InitGenesis()
	if err != nil {
		t.Fatal(err)
	}
	block, _, err := n.mineNextBlock()
	if err != nil {
		t.Fatal(err)
	}
	if err := validator.ValidateBlock(n.Store(), block); err != nil {
		t.Fatalf("validator should validate block mined with independent miner backend: %v", err)
	}
	if _, becameTip, err := validator.InsertBlock(n.Store(), block); err != nil {
		t.Fatal(err)
	} else if !becameTip {
		t.Fatalf("expected mined block after genesis %s to become tip", genesis.BlockHash())
	}
}
