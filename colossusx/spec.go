package colossusx

import (
	"errors"
	"fmt"
)

const (
	StrictDAGSizeBytes uint64 = 80 * 1024 * 1024 * 1024
	StrictNodeSize     uint64 = 64
	StrictReadsPerHash uint64 = 512
	StrictEpochBlocks  uint64 = 8000
)

type Mode string

const (
	ModeStrict   Mode = "strict"
	ModeResearch Mode = "research"
)

type Spec struct {
	Mode         Mode
	DAGSizeBytes uint64
	NodeSize     uint64
	ReadsPerHash uint64
	EpochBlocks  uint64
}

func StrictSpec() Spec {
	return Spec{
		Mode:         ModeStrict,
		DAGSizeBytes: StrictDAGSizeBytes,
		NodeSize:     StrictNodeSize,
		ReadsPerHash: StrictReadsPerHash,
		EpochBlocks:  StrictEpochBlocks,
	}
}

func ResearchSpec(dagSizeBytes, readsPerHash, epochBlocks uint64) Spec {
	s := StrictSpec()
	s.Mode = ModeResearch
	if dagSizeBytes != 0 {
		s.DAGSizeBytes = dagSizeBytes
	}
	if readsPerHash != 0 {
		s.ReadsPerHash = readsPerHash
	}
	if epochBlocks != 0 {
		s.EpochBlocks = epochBlocks
	}
	return s
}

func (s Spec) Validate() error {
	if s.Mode == "" {
		s.Mode = ModeStrict
	}
	if s.DAGSizeBytes == 0 {
		return errors.New("dag size must be > 0")
	}
	if s.NodeSize != StrictNodeSize {
		return fmt.Errorf("COLOSSUS-X requires %d-byte nodes", StrictNodeSize)
	}
	if s.ReadsPerHash == 0 {
		return errors.New("reads/hash must be > 0")
	}
	if s.EpochBlocks == 0 {
		return errors.New("epoch blocks must be > 0")
	}
	if s.DAGSizeBytes%s.NodeSize != 0 {
		return fmt.Errorf("dag size must be multiple of node size (%d)", s.NodeSize)
	}
	if s.Mode == ModeStrict && !s.IsStrictLocked() {
		return fmt.Errorf("strict COLOSSUS-X mode requires DAG_SIZE=%d NODE_SIZE=%d READS_PER_H=%d EPOCH_BLOCKS=%d", StrictDAGSizeBytes, StrictNodeSize, StrictReadsPerHash, StrictEpochBlocks)
	}
	return nil
}

func (s Spec) NodeCount() uint64 { return s.DAGSizeBytes / s.NodeSize }

func (s Spec) IsStrictLocked() bool {
	return s.DAGSizeBytes == StrictDAGSizeBytes && s.NodeSize == StrictNodeSize && s.ReadsPerHash == StrictReadsPerHash && s.EpochBlocks == StrictEpochBlocks
}
