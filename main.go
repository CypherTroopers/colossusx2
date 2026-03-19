package main

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/sha3"
)

const (
	DefaultDAGMiB      = 256
	DefaultReadsPerH   = 64
	DefaultNodeSize    = 64
	DefaultEpochBlocks = 8000
)

type BackendMode string

const (
	BackendUnified BackendMode = "unified"
	BackendCPU     BackendMode = "cpu"
	BackendGPU     BackendMode = "gpu"
)

type Spec struct {
	DAGSizeBytes uint64
	NodeSize     uint64
	ReadsPerHash uint64
	EpochBlocks  uint64
}

func (s Spec) Validate() error {
	if s.DAGSizeBytes == 0 {
		return errors.New("dag size must be > 0")
	}
	if s.NodeSize != 64 {
		return errors.New("this research spec currently requires 64-byte nodes")
	}
	if s.ReadsPerHash == 0 {
		return errors.New("reads/hash must be > 0")
	}
	if s.DAGSizeBytes%s.NodeSize != 0 {
		return fmt.Errorf("dag size must be multiple of node size (%d)", s.NodeSize)
	}
	return nil
}

func (s Spec) NodeCount() uint64 {
	return s.DAGSizeBytes / s.NodeSize
}

type DAG struct {
	spec      Spec
	alloc     managedAllocation
	ownership bool
}

func NewDAGWithAllocation(spec Spec, alloc managedAllocation, ownership bool) (*DAG, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	if alloc == nil {
		return nil, errors.New("dag allocation cannot be nil")
	}
	if uint64(len(alloc.Bytes())) < spec.DAGSizeBytes {
		if ownership {
			_ = alloc.Free()
		}
		return nil, errors.New("managed allocation is smaller than the DAG")
	}
	return &DAG{spec: spec, alloc: alloc, ownership: ownership}, nil
}

func NewDAGWithStrategy(spec Spec, strategy MemoryStrategy) (*DAG, error) {
	if strategy == nil {
		strategy = GoHeapMemory{}
	}
	alloc, err := strategy.Alloc(spec.DAGSizeBytes)
	if err != nil {
		return nil, err
	}
	return NewDAGWithAllocation(spec, alloc, true)
}

func NewDAG(spec Spec) (*DAG, error) {
	return NewDAGWithStrategy(spec, GoHeapMemory{})
}

func (d *DAG) NodeCount() uint64 {
	return d.spec.NodeCount()
}

func (d *DAG) Node(i uint64) []byte {
	off := i * d.spec.NodeSize
	buf := d.Bytes()
	return buf[off : off+d.spec.NodeSize]
}

func (d *DAG) Bytes() []byte {
	if d == nil || d.alloc == nil {
		return nil
	}
	buf := d.alloc.Bytes()
	if uint64(len(buf)) > d.spec.DAGSizeBytes {
		buf = buf[:d.spec.DAGSizeBytes]
	}
	return buf
}

func (d *DAG) Close() error {
	if d == nil || d.alloc == nil || !d.ownership {
		return nil
	}
	err := d.alloc.Free()
	d.alloc = nil
	d.ownership = false
	return err
}

type Target [32]byte

func ParseTargetHex(s string) (Target, error) {
	var t Target
	b, err := hex.DecodeString(s)
	if err != nil {
		return t, err
	}
	if len(b) != 32 {
		return t, fmt.Errorf("target must be exactly 32 bytes, got %d", len(b))
	}
	copy(t[:], b)
	return t, nil
}

func (t Target) String() string {
	return hex.EncodeToString(t[:])
}

type HashResult struct {
	Pow256  [32]byte
	Full512 [64]byte
}

type MineResult struct {
	Nonce      uint64
	Hashes     uint64
	Elapsed    time.Duration
	HashRate   float64
	Hash256Hex string
	Hash512Hex string
	Backend    BackendMode
}

type HashBackend interface {
	Mode() BackendMode
	Description() string
	Prepare(*DAG) error
	Hash(header []byte, nonce uint64, dag *DAG) HashResult
}

type Miner struct {
	spec    Spec
	dag     *DAG
	workers int
	backend HashBackend
}

func NewMiner(spec Spec, dag *DAG, workers int, backend HashBackend) (*Miner, error) {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	if backend == nil {
		backend = &UnifiedBackend{}
	}
	if err := backend.Prepare(dag); err != nil {
		return nil, err
	}
	return &Miner{
		spec:    spec,
		dag:     dag,
		workers: workers,
		backend: backend,
	}, nil
}

func parseBackendMode(s string) (BackendMode, error) {
	switch BackendMode(s) {
	case BackendUnified, BackendCPU, BackendGPU:
		return BackendMode(s), nil
	default:
		return "", fmt.Errorf("unsupported backend %q (expected one of: %s, %s, %s)", s, BackendUnified, BackendCPU, BackendGPU)
	}
}

func newBackend(mode BackendMode) (HashBackend, error) {
	switch mode {
	case BackendUnified:
		return &UnifiedBackend{}, nil
	case BackendCPU:
		return &CPUBackend{}, nil
	case BackendGPU:
		return NewGPUBackend()
	default:
		return nil, fmt.Errorf("unsupported backend %q", mode)
	}
}

func main() {
	var (
		backendName  = flag.String("backend", string(BackendUnified), "mining backend: unified, cpu, or gpu")
		dagAlloc     = flag.String("dag-alloc", "auto", "dag allocation strategy: auto, go-heap, pinned-host, cuda-managed, opencl-svm")
		dagMiB       = flag.Uint64("dag-mib", DefaultDAGMiB, "DAG size in MiB")
		reads        = flag.Uint64("reads", DefaultReadsPerH, "random DAG reads per hash")
		workers      = flag.Int("workers", runtime.NumCPU(), "mining worker count")
		epochBlocks  = flag.Uint64("epoch-blocks", DefaultEpochBlocks, "blocks per epoch")
		headerHex    = flag.String("header", "434f4c4f535355532d582d544553542d4845414445522d303031", "header bytes in hex")
		epochSeedHex = flag.String("epoch-seed", "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff", "epoch seed in hex")
		targetHex    = flag.String("target", "00ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "32-byte big-endian target hex")
		startNonce   = flag.Uint64("start-nonce", 0, "starting nonce")
		maxNonces    = flag.Uint64("max-nonces", 200000, "0 = unbounded")
		benchOnly    = flag.Bool("bench", false, "benchmark hash loop only")
	)
	flag.Parse()

	mode, err := parseBackendMode(*backendName)
	if err != nil {
		log.Fatal(err)
	}
	backend, err := newBackend(mode)
	if err != nil {
		log.Fatal(err)
	}

	spec := Spec{
		DAGSizeBytes: (*dagMiB) * 1024 * 1024,
		NodeSize:     DefaultNodeSize,
		ReadsPerHash: *reads,
		EpochBlocks:  *epochBlocks,
	}
	if err := spec.Validate(); err != nil {
		log.Fatal(err)
	}

	header, err := hex.DecodeString(*headerHex)
	if err != nil {
		log.Fatalf("invalid header hex: %v", err)
	}
	epochSeed, err := hex.DecodeString(*epochSeedHex)
	if err != nil {
		log.Fatalf("invalid epoch-seed hex: %v", err)
	}
	target, err := ParseTargetHex(*targetHex)
	if err != nil {
		log.Fatalf("invalid target: %v", err)
	}

	fmt.Println("COLOSSUS-X research miner")
	fmt.Printf("backend: %s (%s)\n", backend.Mode(), backend.Description())
	fmt.Printf("dag: %d MiB\n", spec.DAGSizeBytes/(1024*1024))
	fmt.Printf("node size: %d bytes\n", spec.NodeSize)
	fmt.Printf("node count: %d\n", spec.NodeCount())
	fmt.Printf("reads/hash: %d\n", spec.ReadsPerHash)
	fmt.Printf("epoch blocks: %d\n", spec.EpochBlocks)
	fmt.Printf("workers: %d\n", *workers)
	fmt.Printf("target: %s\n", target.String())

	strategy, err := selectDAGStrategy(mode, *dagAlloc)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("dag allocation: %s\n", strategy.Name())

	dag, err := NewDAGWithStrategy(spec, strategy)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := dag.Close(); err != nil {
			log.Printf("warning: dag close failed: %v", err)
		}
	}()

	genStart := time.Now()
	if err := GenerateDAG(dag, epochSeed, *workers); err != nil {
		log.Fatalf("generate dag: %v", err)
	}
	fmt.Printf("dag generated in %s\n", time.Since(genStart).Round(time.Millisecond))

	miner, err := NewMiner(spec, dag, *workers, backend)
	if err != nil {
		log.Fatal(err)
	}

	if *benchOnly {
		Benchmark(miner, header, *startNonce, *maxNonces)
		return
	}

	res, ok := miner.Mine(header, target, *startNonce, *maxNonces)
	if !ok {
		fmt.Println("no solution found in range")
		os.Exit(1)
	}

	fmt.Println("solution found")
	fmt.Printf("nonce: %d\n", res.Nonce)
	fmt.Printf("hash256: %s\n", res.Hash256Hex)
	fmt.Printf("hash512: %s\n", res.Hash512Hex)
	fmt.Printf("elapsed: %s\n", res.Elapsed.Round(time.Millisecond))
	fmt.Printf("hashes: %d\n", res.Hashes)
	fmt.Printf("hashrate: %.2f H/s\n", res.HashRate)
}

func GenerateDAG(dag *DAG, epochSeed []byte, workers int) error {
	if len(epochSeed) == 0 {
		return errors.New("epoch seed cannot be empty")
	}
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	nodeCount := dag.NodeCount()
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
				copy(dag.Node(i), sum[:])
			}
		}(from, to)
	}

	wg.Wait()
	return nil
}

func (m *Miner) Mine(header []byte, target Target, startNonce, maxNonces uint64) (MineResult, bool) {
	if batchBackend, ok := m.backend.(BatchHashBackend); ok {
		return m.mineBatch(header, target, startNonce, maxNonces, batchBackend)
	}
	start := time.Now()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var totalHashes atomic.Uint64
	var found atomic.Bool

	type foundMsg struct {
		nonce uint64
		hash  HashResult
	}
	resultCh := make(chan foundMsg, 1)

	var wg sync.WaitGroup

	for wid := 0; wid < m.workers; wid++ {
		wg.Add(1)

		go func(workerID int) {
			defer wg.Done()

			step := uint64(m.workers)
			nonce := startNonce + uint64(workerID)

			for {
				if ctx.Err() != nil || found.Load() {
					return
				}

				if maxNonces > 0 {
					offset := nonce - startNonce
					if offset >= maxNonces {
						return
					}
				}

				h := m.backend.Hash(header, nonce, m.dag)
				totalHashes.Add(1)

				if LessOrEqualBE(h.Pow256, target) {
					if found.CompareAndSwap(false, true) {
						resultCh <- foundMsg{nonce: nonce, hash: h}
						cancel()
					}
					return
				}

				if math.MaxUint64-nonce < step {
					return
				}
				nonce += step
			}
		}(wid)
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	msg, ok := <-resultCh
	if !ok {
		return MineResult{}, false
	}

	elapsed := time.Since(start)
	hashes := totalHashes.Load()
	var hashrate float64
	if elapsed > 0 {
		hashrate = float64(hashes) / elapsed.Seconds()
	}

	return MineResult{
		Nonce:      msg.nonce,
		Hashes:     hashes,
		Elapsed:    elapsed,
		HashRate:   hashrate,
		Hash256Hex: hex.EncodeToString(msg.hash.Pow256[:]),
		Hash512Hex: hex.EncodeToString(msg.hash.Full512[:]),
		Backend:    m.backend.Mode(),
	}, true
}

func (m *Miner) mineBatch(header []byte, target Target, startNonce, maxNonces uint64, batchBackend BatchHashBackend) (MineResult, bool) {
	start := time.Now()
	if maxNonces == 0 {
		maxNonces = 100000
	}
	results, err := batchBackend.HashBatch(header, startNonce, maxNonces, m.dag)
	if err != nil {
		return MineResult{}, false
	}
	for i, h := range results {
		if LessOrEqualBE(h.Pow256, target) {
			elapsed := time.Since(start)
			hashes := uint64(i + 1)
			return MineResult{
				Nonce:      startNonce + uint64(i),
				Hashes:     hashes,
				Elapsed:    elapsed,
				HashRate:   float64(hashes) / elapsed.Seconds(),
				Hash256Hex: hex.EncodeToString(h.Pow256[:]),
				Hash512Hex: hex.EncodeToString(h.Full512[:]),
				Backend:    m.backend.Mode(),
			}, true
		}
	}
	return MineResult{}, false
}

func Benchmark(m *Miner, header []byte, startNonce, maxNonces uint64) {
	if maxNonces == 0 {
		maxNonces = 100000
	}

	start := time.Now()
	if batchBackend, ok := m.backend.(BatchHashBackend); ok {
		_, _ = batchBackend.HashBatch(header, startNonce, maxNonces, m.dag)
	} else {
		for i := uint64(0); i < maxNonces; i++ {
			_ = m.backend.Hash(header, startNonce+i, m.dag)
		}
	}
	elapsed := time.Since(start)

	fmt.Println("benchmark complete")
	fmt.Printf("backend: %s\n", m.backend.Mode())
	fmt.Printf("hashes: %d\n", maxNonces)
	fmt.Printf("elapsed: %s\n", elapsed.Round(time.Millisecond))
	fmt.Printf("hashrate: %.2f H/s\n", float64(maxNonces)/elapsed.Seconds())
}

func fnv1a64(data []byte) uint64 {
	const (
		offset64 = 14695981039346656037
		prime64  = 1099511628211
	)
	var h uint64 = offset64
	for _, b := range data {
		h ^= uint64(b)
		h *= prime64
	}
	return h
}

func LessOrEqualBE(a [32]byte, b Target) bool {
	for i := 0; i < 32; i++ {
		if a[i] < b[i] {
			return true
		}
		if a[i] > b[i] {
			return false
		}
	}
	return true
}
