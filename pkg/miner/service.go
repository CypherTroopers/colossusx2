package miner

import (
	"errors"
	"fmt"
	"runtime"
	"sync"

	cx "colossusx/colossusx"
	"colossusx/pkg/types"
)

type Service struct {
	spec      cx.Spec
	workers   int
	backend   cx.HashBackend
	allocator cx.Allocator

	mu   sync.Mutex
	dags map[string]*cx.DAG
}

type dagKey struct {
	seed string
	size uint64
}

func NewService(spec cx.Spec, workers int, backend cx.HashBackend, allocator cx.Allocator) (*Service, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	if backend == nil {
		return nil, errors.New("miner backend is required")
	}
	if allocator == nil {
		return nil, errors.New("dag allocator is required")
	}
	return &Service{spec: spec, workers: workers, backend: backend, allocator: allocator, dags: make(map[string]*cx.DAG)}, nil
}

func (s *Service) SealBlock(block types.Block, maxNonces uint64) (types.Block, cx.MineResult, error) {
	dag, err := s.dagForHeader(block.Header)
	if err != nil {
		return types.Block{}, cx.MineResult{}, err
	}
	miner, err := cx.NewMiner(s.spec, dag, s.workers, s.backend)
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

func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, dag := range s.dags {
		_ = dag.Close()
		delete(s.dags, key)
	}
	return nil
}

func (s *Service) dagForHeader(header types.BlockHeader) (*cx.DAG, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := dagKey{seed: header.EpochSeed.String(), size: header.DAGSizeBytes}
	encoded := fmt.Sprintf("%s/%d", key.seed, key.size)
	if dag, ok := s.dags[encoded]; ok {
		return dag, nil
	}
	spec := s.spec
	spec.DAGSizeBytes = header.DAGSizeBytes
	dag, err := cx.NewDAGWithAllocator(spec, s.allocator)
	if err != nil {
		return nil, err
	}
	if err := cx.PopulateDAG(dag, header.EpochSeed[:], s.workers); err != nil {
		_ = dag.Close()
		return nil, err
	}
	s.dags[encoded] = dag
	return dag, nil
}
