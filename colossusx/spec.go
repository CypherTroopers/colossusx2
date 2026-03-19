package colossusx

import (
	"errors"
	"fmt"
)

const (
	DAGSizeBytes uint64 = 80 * 1024 * 1024 * 1024
	NodeSize     uint64 = 64
	ReadsPerHash uint64 = 512
	EpochBlocks  uint64 = 8000
)

type Spec struct {
	DAGSizeBytes uint64
	NodeSize     uint64
	ReadsPerHash uint64
	EpochBlocks  uint64
}

func DefaultSpec() Spec {
	return Spec{
		DAGSizeBytes: DAGSizeBytes,
		NodeSize:     NodeSize,
		ReadsPerHash: ReadsPerHash,
		EpochBlocks:  EpochBlocks,
	}
}

func (s Spec) Validate() error {
	if s.DAGSizeBytes == 0 {
		return errors.New("dag size must be > 0")
	}
	if s.NodeSize != NodeSize {
		return fmt.Errorf("COLOSSUS-X requires %d-byte nodes", NodeSize)
	}
	if s.ReadsPerHash == 0 {
		return errors.New("reads/hash must be > 0")
	}
	if s.DAGSizeBytes%s.NodeSize != 0 {
		return fmt.Errorf("dag size must be multiple of node size (%d)", s.NodeSize)
	}
	return nil
}

func (s Spec) NodeCount() uint64 { return s.DAGSizeBytes / s.NodeSize }

func (s Spec) IsSpecLocked() bool {
	return s.DAGSizeBytes == DAGSizeBytes && s.NodeSize == NodeSize && s.ReadsPerHash == ReadsPerHash && s.EpochBlocks == EpochBlocks
}
