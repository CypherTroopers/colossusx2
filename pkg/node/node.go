package node

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"time"

	cx "colossusx/colossusx"
	"colossusx/pkg/chain"
	"colossusx/pkg/consensus"
	"colossusx/pkg/types"
)

type Config struct {
	Chain     types.ChainConfig
	Genesis   types.GenesisConfig
	Mine      bool
	MaxNonces uint64
	BlockTime time.Duration
	Logf      func(string, ...any)
}

type Node struct {
	cfg       Config
	store     *chain.MemoryStore
	validator *consensus.Validator
}

func New(cfg Config, validator *consensus.Validator, store *chain.MemoryStore) (*Node, error) {
	if store == nil {
		store = chain.NewMemoryStore()
	}
	if validator == nil {
		return nil, fmt.Errorf("validator is required")
	}
	if cfg.BlockTime <= 0 {
		cfg.BlockTime = time.Second
	}
	if cfg.Logf == nil {
		cfg.Logf = log.Printf
	}
	return &Node{cfg: cfg, validator: validator, store: store}, nil
}

func (n *Node) Store() *chain.MemoryStore { return n.store }

func (n *Node) InitGenesis() (types.Block, error) {
	if tip, _, err := n.store.CurrentTip(); err == nil {
		return tip, nil
	}
	genesis := types.NewGenesisBlock(n.cfg.Genesis)
	sealed, res, err := n.validator.SealBlock(genesis, n.cfg.MaxNonces)
	if err != nil {
		return types.Block{}, err
	}
	work := consensus.CalcBlockWork(sealed.Header.Target)
	if err := n.store.StoreBlock(sealed, work); err != nil {
		return types.Block{}, err
	}
	if err := n.store.SetCurrentTip(sealed.BlockHash()); err != nil {
		return types.Block{}, err
	}
	n.cfg.Logf("genesis initialized hash=%s nonce=%d hashes=%d", sealed.BlockHash().String(), sealed.Header.Nonce, res.Hashes)
	return sealed, nil
}

func (n *Node) Run(ctx context.Context) error {
	if _, err := n.InitGenesis(); err != nil {
		return err
	}
	if !n.cfg.Mine {
		<-ctx.Done()
		return ctx.Err()
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		block, res, err := n.mineNextBlock()
		if err != nil {
			return err
		}
		_, becameTip, err := n.validator.InsertBlock(n.store, block)
		if err != nil {
			return err
		}
		n.cfg.Logf("mined block height=%d hash=%s nonce=%d hashes=%d hashrate=%.2fH/s became_tip=%t", block.Header.Height, block.BlockHash().String(), block.Header.Nonce, res.Hashes, res.HashRate, becameTip)
		timer := time.NewTimer(n.cfg.BlockTime)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (n *Node) mineNextBlock() (types.Block, cx.MineResult, error) {
	tip, _, err := n.store.CurrentTip()
	if err != nil {
		return types.Block{}, cx.MineResult{}, err
	}
	header := types.BlockHeader{
		Version:      1,
		Height:       tip.Header.Height + 1,
		ParentHash:   tip.BlockHash(),
		Timestamp:    max(time.Now().Unix(), tip.Header.Timestamp+1),
		Target:       n.cfg.Genesis.Bits,
		EpochSeed:    types.EpochSeedForHeight(n.cfg.Chain.Spec, tip.Header.Height+1),
		DAGSizeBytes: n.cfg.Chain.Spec.DAGSizeBytes,
		TxRoot:       sha256.Sum256([]byte(fmt.Sprintf("height:%d", tip.Header.Height+1))),
		StateRoot:    sha256.Sum256([]byte(tip.BlockHash().String())),
	}
	block := types.Block{Header: header}
	sealed, res, err := n.validator.SealBlock(block, n.cfg.MaxNonces)
	if err != nil {
		return types.Block{}, cx.MineResult{}, err
	}
	return sealed, res, nil
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
