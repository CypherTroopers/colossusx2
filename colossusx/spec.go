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

	StrictV2TileSizeBytes     uint64 = 4096
	StrictV2MatDim            uint32 = 16
	StrictV2ComputeRounds     uint32 = 64
	StrictV2RoundCommitPeriod uint32 = 8
)

type Mode string

type ComputePrecision string

type MemoryModel string

const (
	ModeStrict   Mode = "strict"
	ModeResearch Mode = "research"

	ComputePrecisionInt8 ComputePrecision = "int8"
	ComputePrecisionFP16 ComputePrecision = "fp16"

	MemoryModelAny           MemoryModel = "any"
	MemoryModelUnifiedShared MemoryModel = "unified-shared"
)

type Spec struct {
	Mode                Mode
	DAGSizeBytes        uint64
	NodeSize            uint64
	ReadsPerHash        uint64
	EpochBlocks         uint64
	TileSizeBytes       uint64
	MatDim              uint32
	ComputeRounds       uint32
	ComputePrecision    ComputePrecision
	MemoryModelRequired MemoryModel
	DeviceExecutionOnly bool
	RoundCommitInterval uint32
	AlgorithmVersion    uint32
}

func StrictSpec() Spec {
	return Spec{
		Mode:                ModeStrict,
		DAGSizeBytes:        StrictDAGSizeBytes,
		NodeSize:            StrictNodeSize,
		ReadsPerHash:        StrictReadsPerHash,
		EpochBlocks:         StrictEpochBlocks,
		TileSizeBytes:       StrictV2TileSizeBytes,
		MatDim:              StrictV2MatDim,
		ComputeRounds:       StrictV2ComputeRounds,
		ComputePrecision:    ComputePrecisionInt8,
		MemoryModelRequired: MemoryModelUnifiedShared,
		DeviceExecutionOnly: true,
		RoundCommitInterval: StrictV2RoundCommitPeriod,
		AlgorithmVersion:    2,
	}
}

func ResearchSpec(dagSizeBytes, readsPerHash, epochBlocks uint64) Spec {
	s := StrictSpec()
	s.Mode = ModeResearch
	s.TileSizeBytes = 0
	s.MatDim = 0
	s.ComputeRounds = 0
	s.ComputePrecision = ""
	s.MemoryModelRequired = MemoryModelAny
	s.DeviceExecutionOnly = false
	s.RoundCommitInterval = 0
	s.AlgorithmVersion = 1
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
	switch s.Mode {
	case ModeStrict:
		if !s.IsStrictLocked() {
			return fmt.Errorf("strict COLOSSUS-X mode requires DAG_SIZE=%d NODE_SIZE=%d READS_PER_H=%d EPOCH_BLOCKS=%d", StrictDAGSizeBytes, StrictNodeSize, StrictReadsPerHash, StrictEpochBlocks)
		}
		if s.AlgorithmVersion != 2 {
			return fmt.Errorf("strict mode requires algorithm version 2")
		}
		if s.MemoryModelRequired != MemoryModelUnifiedShared {
			return fmt.Errorf("strict mode requires unified-shared memory model")
		}
		if !s.DeviceExecutionOnly {
			return fmt.Errorf("strict mode requires device execution")
		}
		if s.TileSizeBytes == 0 || s.MatDim == 0 || s.ComputeRounds == 0 || s.RoundCommitInterval == 0 {
			return fmt.Errorf("strict mode requires strict-v2 tensor parameters")
		}
	case ModeResearch:
		if s.AlgorithmVersion == 0 {
			s.AlgorithmVersion = 1
		}
	default:
		return fmt.Errorf("unsupported mode %q", s.Mode)
	}
	return nil
}

func (s Spec) NodeCount() uint64 { return s.DAGSizeBytes / s.NodeSize }
func (s Spec) IsStrictLocked() bool {
	return s.DAGSizeBytes == StrictDAGSizeBytes && s.NodeSize == StrictNodeSize && s.ReadsPerHash == StrictReadsPerHash && s.EpochBlocks == StrictEpochBlocks
}
