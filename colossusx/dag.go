package colossusx

import (
	"encoding/binary"
	"errors"
	"runtime"
	"sync"

	"golang.org/x/crypto/sha3"
)

func GenerateDAG(spec Spec, dag []byte, epochSeed []byte, workers int) error {
	if err := spec.Validate(); err != nil {
		return err
	}
	if len(epochSeed) == 0 {
		return errors.New("epoch seed cannot be empty")
	}
	if uint64(len(dag)) < spec.DAGSizeBytes {
		return errors.New("managed allocation is smaller than the DAG")
	}
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	nodeCount := spec.NodeCount()
	chunk := nodeCount / uint64(workers)
	if chunk == 0 {
		chunk = 1
	}

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		from := uint64(w) * chunk
		to := from + chunk
		if w == workers-1 || to > nodeCount {
			to = nodeCount
		}
		if from >= nodeCount {
			break
		}
		wg.Add(1)
		go func(from, to uint64) {
			defer wg.Done()
			tmp := make([]byte, len(epochSeed)+8)
			copy(tmp, epochSeed)
			for i := from; i < to; i++ {
				binary.LittleEndian.PutUint64(tmp[len(epochSeed):], i)
				sum := sha3.Sum512(tmp)
				off := i * spec.NodeSize
				copy(dag[off:off+spec.NodeSize], sum[:])
			}
		}(from, to)
	}
	wg.Wait()
	return nil
}
