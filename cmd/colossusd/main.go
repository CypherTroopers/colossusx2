package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	cx "colossusx/colossusx"
	"colossusx/pkg/chain"
	"colossusx/pkg/consensus"
	"colossusx/pkg/miner"
	"colossusx/pkg/mining"
	"colossusx/pkg/node"
	"colossusx/pkg/types"
)

func main() {
	cfg, err := parseFlags()
	if err != nil {
		log.Fatal(err)
	}

	validatorBackend, err := mining.NewBackend(cfg.ValidatorBackend)
	if err != nil {
		log.Fatal(err)
	}
	validator, err := consensus.NewValidator(cfg.Chain, validatorBackend, cfg.Workers)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := validator.Close(); err != nil {
			log.Printf("validator close: %v", err)
		}
	}()

	minerBackend, err := mining.NewBackend(cfg.MinerBackend)
	if err != nil {
		log.Fatal(err)
	}
	minerRuntime, err := mining.InitializeBackendRuntime(minerBackend)
	if err != nil {
		log.Fatal(err)
	}
	minerMode, err := mining.ParseBackendMode(cfg.MinerBackend)
	if err != nil {
		log.Fatal(err)
	}
	allocator, err := mining.ResolveDAGStrategy(minerMode, cfg.MinerDAGAlloc, minerRuntime)
	if err != nil {
		log.Fatal(err)
	}
	minerSvc, err := miner.NewService(cfg.Chain.Spec, cfg.Workers, minerBackend, allocator)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := minerSvc.Close(); err != nil {
			log.Printf("miner close: %v", err)
		}
	}()

	store, err := chain.NewDiskStore(cfg.DataDir)
	if err != nil {
		log.Fatal(err)
	}

	n, err := node.New(node.Config{
		Chain:      cfg.Chain,
		Genesis:    cfg.Genesis,
		Mine:       cfg.Mine,
		MaxNonces:  cfg.MaxNonces,
		BlockTime:  cfg.BlockTime,
		NodeID:     cfg.NodeID,
		ListenAddr: cfg.ListenAddr,
		Bootnodes:  cfg.Bootnodes,
	}, validator, minerSvc, store)
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Printf("colossusd starting network=%s mode=%s dag=%dMiB workers=%d mine=%t datadir=%s listen=%s bootnodes=%d node_id=%s validator_backend=%s miner_backend=%s miner_dag_alloc=%s resolved_alloc=%s runtime_initialized=%t\n", cfg.Chain.NetworkID, cfg.Chain.Spec.Mode, cfg.Chain.Spec.DAGSizeBytes/(1024*1024), cfg.Workers, cfg.Mine, cfg.DataDir, cfg.ListenAddr, len(cfg.Bootnodes), cfg.NodeID, cfg.ValidatorBackend, cfg.MinerBackend, cfg.MinerDAGAlloc, allocator.Name(), minerRuntime != nil)
	if err := n.Run(ctx); err != nil && err != context.Canceled {
		log.Fatal(err)
	}
}

type config struct {
	Chain             types.ChainConfig
	Genesis           types.GenesisConfig
	Mine              bool
	Workers           int
	MaxNonces         uint64
	BlockTime         time.Duration
	DataDir           string
	ListenAddr        string
	Bootnodes         []string
	NodeID            string
	MinerBackend      string
	MinerDAGAlloc     string
	ValidatorBackend  string
	ValidatorDAGAlloc string
}

func parseFlags() (config, error) {
	fs := flag.NewFlagSet("colossusd", flag.ContinueOnError)
	modeName := fs.String("mode", string(cx.ModeResearch), "chain mode: strict or research")
	networkID := fs.String("network", "devnet", "network identifier")
	dagMiB := fs.Uint64("dag-mib", 8, "DAG size in MiB for research mode")
	reads := fs.Uint64("reads", 32, "DAG reads per hash for research mode")
	epochBlocks := fs.Uint64("epoch-blocks", 32, "blocks per epoch for research mode")
	mine := fs.Bool("mine", true, "enable local mining loop")
	noMine := fs.Bool("no-mine", false, "disable local mining loop")
	workers := fs.Int("workers", runtime.NumCPU(), "mining workers")
	maxNonces := fs.Uint64("max-nonces", 500000, "maximum nonce range per block template")
	blockTime := fs.Duration("block-time", 500*time.Millisecond, "delay between mined blocks")
	genesisMessage := fs.String("genesis-message", "colossusx devnet genesis", "genesis message")
	dataDir := fs.String("datadir", filepath.Join(".", "data"), "node data directory")
	listenAddr := fs.String("listen", ":30333", "tcp listen address")
	bootnodes := fs.String("bootnodes", "", "comma-separated bootnode addresses")
	nodeID := fs.String("node-id", "", "stable node identifier")
	targetHex := fs.String("target", "0fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "mining target in hex")
	minerBackend := fs.String("miner-backend", string(cx.BackendCPU), "mining backend: unified, cpu, or gpu")
	minerDAGAlloc := fs.String("miner-dag-alloc", "auto", "mining dag allocation: auto, go-heap, pinned-host, cuda-managed, opencl-svm")
	validatorBackend := fs.String("validator-backend", string(cx.BackendCPU), "validation backend: cpu by default")
	validatorDAGAlloc := fs.String("validator-dag-alloc", "go-heap", "validation dag allocation (reserved for future wiring)")
	rpcListen := fs.String("rpc-listen", "", "reserved for future RPC listener")
	_ = rpcListen
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
	target, err := cx.ParseTargetHex(*targetHex)
	if err != nil {
		return config{}, err
	}
	if *noMine {
		*mine = false
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
	return config{Chain: chainCfg, Genesis: genesis, Mine: *mine, Workers: *workers, MaxNonces: *maxNonces, BlockTime: *blockTime, DataDir: *dataDir, ListenAddr: *listenAddr, Bootnodes: node.ParseBootnodes(*bootnodes), NodeID: *nodeID, MinerBackend: *minerBackend, MinerDAGAlloc: *minerDAGAlloc, ValidatorBackend: *validatorBackend, ValidatorDAGAlloc: *validatorDAGAlloc}, nil
}
