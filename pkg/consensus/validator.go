package consensus

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"runtime"
	"sync"
	"time"

	cx "colossusx/colossusx"
	"colossusx/pkg/chain"
	"colossusx/pkg/types"
)

var (
	ErrInvalidParent    = errors.New("invalid parent linkage")
	ErrInvalidTimestamp = errors.New("invalid timestamp")
	ErrInvalidTarget    = errors.New("invalid target")
	ErrInvalidPoW       = errors.New("invalid proof of work")
	ErrInvalidEpoch     = errors.New("invalid epoch parameters")
)

type Validator struct {
	config    types.ChainConfig
	backend   cx.HashBackend
	workers   int
	now       func() time.Time
	mu        sync.Mutex
	dags      map[string]*cx.DAG
	allocator cx.Allocator
}

type dagKey struct {
	seed string
	size uint64
}

type sliceAllocation struct{ buf []byte }

func (a *sliceAllocation) Bytes() []byte { return a.buf }
func (a *sliceAllocation) Free() error   { a.buf = nil; return nil }
func (a *sliceAllocation) Name() string  { return "go-slice" }

type sliceAllocator struct{}

func (sliceAllocator) Alloc(size uint64) (cx.Allocation, error) {
	return &sliceAllocation{buf: make([]byte, size)}, nil
}
func (sliceAllocator) Name() string { return "go-slice" }

type CPUBackend struct{}

func (CPUBackend) Mode() cx.BackendMode  { return cx.BackendCPU }
func (CPUBackend) Description() string   { return "consensus cpu backend" }
func (CPUBackend) Prepare(*cx.DAG) error { return nil }
func (CPUBackend) Hash(header []byte, nonce cx.Nonce, dag *cx.DAG) cx.HashResult {
	return cx.LatticeHash(dag.Spec(), header, nonce, dag, nil)
}

func NewValidator(cfg types.ChainConfig, backend cx.HashBackend, workers int) (*Validator, error) {
	if err := cfg.Spec.Validate(); err != nil {
		return nil, err
	}
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	if backend == nil {
		backend = CPUBackend{}
	}
	return &Validator{
		config:    cfg,
		backend:   backend,
		workers:   workers,
		now:       time.Now,
		dags:      make(map[string]*cx.DAG),
		allocator: sliceAllocator{},
	}, nil
}

func (v *Validator) ValidateHeader(store chain.Store, header types.BlockHeader) error {
	if header.DAGSizeBytes != v.config.Spec.DAGSizeBytes {
		return fmt.Errorf("%w: dag size mismatch", ErrInvalidEpoch)
	}
	expectedSeed := types.EpochSeedForHeight(v.config.Spec, header.Height)
	if expectedSeed != header.EpochSeed {
		return fmt.Errorf("%w: epoch seed mismatch", ErrInvalidEpoch)
	}
	if header.Target == (cx.Target{}) {
		return ErrInvalidTarget
	}
	if header.Height == 0 {
		if header.ParentHash != (types.Hash{}) {
			return fmt.Errorf("%w: genesis parent must be zero", ErrInvalidParent)
		}
	} else {
		parent, err := store.GetHeader(header.ParentHash)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidParent, err)
		}
		if parent.Height+1 != header.Height {
			return fmt.Errorf("%w: expected height %d got %d", ErrInvalidParent, parent.Height+1, header.Height)
		}
		if header.Timestamp <= parent.Timestamp {
			return fmt.Errorf("%w: child timestamp %d <= parent %d", ErrInvalidTimestamp, header.Timestamp, parent.Timestamp)
		}
	}
	now := v.now().Unix() + 2*60*60
	if header.Timestamp > now {
		return fmt.Errorf("%w: timestamp %d too far ahead of %d", ErrInvalidTimestamp, header.Timestamp, now)
	}
	return v.validatePoW(header)
}

func (v *Validator) ValidateBlock(store chain.Store, block types.Block) error {
	return v.ValidateHeader(store, block.Header)
}

func CalcBlockWork(target cx.Target) *big.Int {
	max := new(big.Int).Lsh(big.NewInt(1), 256)
	targetInt := new(big.Int).SetBytes(target[:])
	if targetInt.Sign() == 0 {
		return big.NewInt(0)
	}
	return max.Div(max, targetInt.Add(targetInt, big.NewInt(1)))
}

func SelectBestChainByTotalWork(currentHash types.Hash, currentWork *big.Int, candidateHash types.Hash, candidateWork *big.Int) types.Hash {
	cmp := candidateWork.Cmp(currentWork)
	if cmp > 0 {
		return candidateHash
	}
	if cmp < 0 {
		return currentHash
	}
	if candidateHash.String() < currentHash.String() {
		return candidateHash
	}
	return currentHash
}

func (v *Validator) InsertBlock(store chain.Store, block types.Block) (*big.Int, bool, error) {
	if err := v.ValidateBlock(store, block); err != nil {
		return nil, false, err
	}
	blockHash := block.BlockHash()
	if store.HasBlock(blockHash) {
		work, err := store.TotalWork(blockHash)
		return work, false, err
	}
	blockWork := CalcBlockWork(block.Header.Target)
	totalWork := new(big.Int).Set(blockWork)
	if block.Header.Height > 0 {
		parentWork, err := store.TotalWork(block.Header.ParentHash)
		if err != nil {
			return nil, false, err
		}
		totalWork.Add(totalWork, parentWork)
	}
	if err := store.StoreBlock(block, totalWork); err != nil {
		return nil, false, err
	}
	current, currentWork, err := store.CurrentTip()
	if err != nil {
		if err := store.SetCurrentTip(blockHash); err != nil {
			return nil, false, err
		}
		return totalWork, true, nil
	}
	best := SelectBestChainByTotalWork(current.BlockHash(), currentWork, blockHash, totalWork)
	if best == blockHash {
		if err := store.SetCurrentTip(blockHash); err != nil {
			return nil, false, err
		}
		return totalWork, true, nil
	}
	return totalWork, false, nil
}

func (v *Validator) SealBlock(block types.Block, maxNonces uint64) (types.Block, cx.MineResult, error) {
	dag, err := v.dagForHeader(block.Header)
	if err != nil {
		return types.Block{}, cx.MineResult{}, err
	}
	miner, err := cx.NewMiner(v.config.Spec, dag, v.workers, v.backend)
	if err != nil {
		return types.Block{}, cx.MineResult{}, err
	}
	res, ok := miner.Mine(block.Header.EncodeForMining(), block.Header.Target, cx.NewUint64Nonce(0), maxNonces)
	if !ok {
		return types.Block{}, cx.MineResult{}, fmt.Errorf("no solution found in %d nonces", maxNonces)
	}
	nonce, ok := res.Nonce.(cx.Uint64Nonce)
	if !ok {
		return types.Block{}, cx.MineResult{}, errors.New("unexpected nonce type")
	}
	block.Header.Nonce = nonce.Uint64()
	return block, res, nil
}

func (v *Validator) Close() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	for key, dag := range v.dags {
		_ = dag.Close()
		delete(v.dags, key)
	}
	return nil
}

func (v *Validator) validatePoW(header types.BlockHeader) error {
	dag, err := v.dagForHeader(header)
	if err != nil {
		return err
	}
	hash := v.backend.Hash(header.EncodeForMining(), cx.NewUint64Nonce(header.Nonce), dag)
	if !cx.LessOrEqualBE(hash.Pow256, header.Target) {
		return fmt.Errorf("%w: pow=%s target=%s", ErrInvalidPoW, hex.EncodeToString(hash.Pow256[:]), header.Target.String())
	}
	return nil
}

func (v *Validator) dagForHeader(header types.BlockHeader) (*cx.DAG, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	key := dagKey{seed: header.EpochSeed.String(), size: header.DAGSizeBytes}
	encoded := fmt.Sprintf("%s/%d", key.seed, key.size)
	if dag, ok := v.dags[encoded]; ok {
		return dag, nil
	}
	spec := v.config.Spec
	spec.DAGSizeBytes = header.DAGSizeBytes
	dag, err := cx.NewDAGWithAllocator(spec, v.allocator)
	if err != nil {
		return nil, err
	}
	if err := cx.PopulateDAG(dag, header.EpochSeed[:], v.workers); err != nil {
		_ = dag.Close()
		return nil, err
	}
	v.dags[encoded] = dag
	return dag, nil
}
