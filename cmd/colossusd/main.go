package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	cx "colossusx/colossusx"
	"colossusx/pkg/chain"
	"colossusx/pkg/consensus"
	"colossusx/pkg/node"
	"colossusx/pkg/types"
)

func main() {
	cfg, err := parseFlags()
	if err != nil {
		log.Fatal(err)
	}

	validator, err := consensus.NewValidator(cfg.Chain, consensus.CPUBackend{}, cfg.Workers)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := validator.Close(); err != nil {
			log.Printf("validator close: %v", err)
		}
	}()

	n, err := node.New(node.Config{
		Chain:     cfg.Chain,
		Genesis:   cfg.Genesis,
		Mine:      cfg.Mine,
		MaxNonces: cfg.MaxNonces,
		BlockTime: cfg.BlockTime,
	}, validator, chain.NewMemoryStore())
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Printf("colossusd starting network=%s mode=%s dag=%dMiB workers=%d mine=%t\n", cfg.Chain.NetworkID, cfg.Chain.Spec.Mode, cfg.Chain.Spec.DAGSizeBytes/(1024*1024), cfg.Workers, cfg.Mine)
	if err := n.Run(ctx); err != nil && err != context.Canceled {
		log.Fatal(err)
	}
}

type config struct {
	Chain     types.ChainConfig
	Genesis   types.GenesisConfig
	Mine      bool
	Workers   int
	MaxNonces uint64
	BlockTime time.Duration
}

func parseFlags() (config, error) {
	fs := flag.NewFlagSet("colossusd", flag.ContinueOnError)
	modeName := fs.String("mode", string(cx.ModeResearch), "chain mode: strict or research")
	networkID := fs.String("network", "devnet", "network identifier")
	dagMiB := fs.Uint64("dag-mib", 8, "DAG size in MiB for research mode")
	reads := fs.Uint64("reads", 32, "DAG reads per hash for research mode")
	epochBlocks := fs.Uint64("epoch-blocks", 32, "blocks per epoch for research mode")
	mine := fs.Bool("mine", true, "enable local mining loop")
	workers := fs.Int("workers", runtime.NumCPU(), "mining workers")
	maxNonces := fs.Uint64("max-nonces", 500000, "maximum nonce range per block template")
	blockTime := fs.Duration("block-time", 500*time.Millisecond, "delay between mined blocks")
	genesisMessage := fs.String("genesis-message", "colossusx devnet genesis", "genesis message")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return config{}, err
	}

	mode := cx.Mode(*modeName)
	var spec cx.Spec
	switch mode {
	case cx.ModeStrict:
		spec = cx.StrictSpec()
	case cx.ModeResearch:
		spec = cx.ResearchSpec((*dagMiB)*1024*1024, *reads, *epochBlocks)
	default:
		return config{}, fmt.Errorf("unsupported mode %q", *modeName)
	}
	if err := spec.Validate(); err != nil {
		return config{}, err
	}
	target, err := cx.ParseTargetHex("0fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
	if err != nil {
		return config{}, err
	}
	chainCfg := types.ChainConfig{NetworkID: *networkID, Spec: spec}
	genesis := types.GenesisConfig{
		ChainID:   *networkID,
		Message:   *genesisMessage,
		Timestamp: time.Now().Unix(),
		Bits:      target,
		Spec:      spec,
		ExtraData: fmt.Sprintf("mode=%s", spec.Mode),
	}
	return config{Chain: chainCfg, Genesis: genesis, Mine: *mine, Workers: *workers, MaxNonces: *maxNonces, BlockTime: *blockTime}, nil
}
