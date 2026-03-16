package main

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zeebo/blake3"
	"golang.org/x/crypto/sha3"
)

const (
	NodeSize = 64
)

type Config struct {
	DAGSizeBytes uint64
	ReadsPerHash uint64
	Workers      int
}

type Result struct {
	Nonce      uint64
	Hash256Hex string
	Hash512Hex string
	Elapsed    time.Duration
	Hashes     uint64
}

func main() {
	var (
		dagMiB       = flag.Uint64("dag-mib", 256, "DAG size in MiB")
		reads        = flag.Uint64("reads", 64, "random DAG reads per hash")
		workers      = flag.Int("workers", runtime.NumCPU(), "number of mining workers")
		headerHex    = flag.String("header", "434f4c4f535355532d582d544553542d4845414445522d303031", "block header bytes in hex")
		epochSeedHex = flag.String("epoch-seed", "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff", "epoch seed in hex")
		targetHex    = flag.String("target", "0000ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "32-byte big-endian target hex")
		startNonce   = flag.Uint64("start-nonce", 0, "start nonce")
		maxNonces    = flag.Uint64("max-nonces", 0, "0 = unbounded search")
	)
	flag.Parse()

	if *dagMiB == 0 {
		log.Fatal("dag-mib must be > 0")
	}
	if *reads == 0 {
		log.Fatal("reads must be > 0")
	}
	if *workers <= 0 {
		log.Fatal("workers must be > 0")
	}

	header, err := hex.DecodeString(*headerHex)
	if err != nil {
		log.Fatalf("invalid header hex: %v", err)
	}
	epochSeed, err := hex.DecodeString(*epochSeedHex)
	if err != nil {
		log.Fatalf("invalid epoch-seed hex: %v", err)
	}
	target, err := parseTarget(*targetHex)
	if err != nil {
		log.Fatalf("invalid target hex: %v", err)
	}

	cfg := Config{
		DAGSizeBytes: (*dagMiB) * 1024 * 1024,
		ReadsPerHash: *reads,
		Workers:      *workers,
	}

	if cfg.DAGSizeBytes%NodeSize != 0 {
		log.Fatalf("dag size must be multiple of %d bytes", NodeSize)
	}

	fmt.Printf("COLOSSUS-X research miner\n")
	fmt.Printf("dag: %d MiB\n", cfg.DAGSizeBytes/(1024*1024))
	fmt.Printf("node size: %d bytes\n", NodeSize)
	fmt.Printf("reads/hash: %d\n", cfg.ReadsPerHash)
	fmt.Printf("workers: %d\n", cfg.Workers)
	fmt.Printf("header: %x\n", header)
	fmt.Printf("epoch seed: %x\n", epochSeed)
	fmt.Printf("target: %x\n", target[:])

	start := time.Now()
	dag := make([]byte, cfg.DAGSizeBytes)
	generateDAG(dag, epochSeed, cfg.Workers)
	fmt.Printf("dag generated in %s\n", time.Since(start).Round(time.Millisecond))

	res, ok := mine(header, target, dag, cfg, *startNonce, *maxNonces)
	if !ok {
		fmt.Println("no solution found in search range")
		os.Exit(1)
	}

	fmt.Println("solution found")
	fmt.Printf("nonce: %d\n", res.Nonce)
	fmt.Printf("hash256: %s\n", res.Hash256Hex)
	fmt.Printf("hash512: %s\n", res.Hash512Hex)
	fmt.Printf("elapsed: %s\n", res.Elapsed.Round(time.Millisecond))
	if res.Elapsed > 0 {
		fmt.Printf("hashrate: %.2f H/s\n", float64(res.Hashes)/res.Elapsed.Seconds())
	}
}

func parseTarget(s string) ([32]byte, error) {
	var out [32]byte
	b, err := hex.DecodeString(s)
	if err != nil {
		return out, err
	}
	if len(b) != 32 {
		return out, fmt.Errorf("target must be exactly 32 bytes, got %d", len(b))
	}
	copy(out[:], b)
	return out, nil
}

func generateDAG(dag []byte, epochSeed []byte, workers int) {
	nodeCount := uint64(len(dag) / NodeSize)

	var wg sync.WaitGroup
	chunk := nodeCount / uint64(workers)
	if chunk == 0 {
		chunk = 1
	}

	for w := 0; w < workers; w++ {
		start := uint64(w) * chunk
		end := start + chunk
		if w == workers-1 || end > nodeCount {
			end = nodeCount
		}
		if start >= nodeCount {
			break
		}

		wg.Add(1)
		go func(from, to uint64) {
			defer wg.Done()

			buf := make([]byte, len(epochSeed)+8)
			copy(buf, epochSeed)

			for i := from; i < to; i++ {
				binary.LittleEndian.PutUint64(buf[len(epochSeed):], i)
				sum := sha3.Sum512(buf)
				copy(dag[i*NodeSize:(i+1)*NodeSize], sum[:])
			}
		}(start, end)
	}

	wg.Wait()
}

func mine(header []byte, target [32]byte, dag []byte, cfg Config, startNonce uint64, maxNonces uint64) (Result, bool) {
	start := time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var totalHashes atomic.Uint64
	var found atomic.Bool

	type foundResult struct {
		nonce   uint64
		hash256 [32]byte
		hash512 [64]byte
	}
	resultCh := make(chan foundResult, 1)

	var wg sync.WaitGroup

	for wid := 0; wid < cfg.Workers; wid++ {
		wg.Add(1)

		go func(workerID int) {
			defer wg.Done()

			step := uint64(cfg.Workers)
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

				hash256, hash512 := latticeHash(header, nonce, dag, cfg)
				totalHashes.Add(1)

				if lessOrEqualBE(hash256, target) {
					if found.CompareAndSwap(false, true) {
						resultCh <- foundResult{
							nonce:   nonce,
							hash256: hash256,
							hash512: hash512,
						}
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

	r, ok := <-resultCh
	if !ok {
		return Result{}, false
	}

	return Result{
		Nonce:      r.nonce,
		Hash256Hex: hex.EncodeToString(r.hash256[:]),
		Hash512Hex: hex.EncodeToString(r.hash512[:]),
		Elapsed:    time.Since(start),
		Hashes:     totalHashes.Load(),
	}, true
}

func latticeHash(header []byte, nonce uint64, dag []byte, cfg Config) ([32]byte, [64]byte) {
	var zero32 [32]byte
	var zero64 [64]byte

	nodeCount := uint64(len(dag) / NodeSize)
	if nodeCount == 0 {
		return zero32, zero64
	}

	seedInput := make([]byte, len(header)+8)
	copy(seedInput, header)
	binary.LittleEndian.PutUint64(seedInput[len(header):], nonce)

	seed512 := sha3.Sum512(seedInput)

	var mix [32]byte
	copy(mix[:], seed512[:32])

	var fnvInput [40]byte
	var blakeInput [64]byte

	for r := uint64(0); r < cfg.ReadsPerHash; r++ {
		copy(fnvInput[:32], mix[:])
		binary.LittleEndian.PutUint64(fnvInput[32:], r)

		idx := fnv1a64(fnvInput[:]) % nodeCount
		node := dag[idx*NodeSize : (idx+1)*NodeSize]

		for i := 0; i < 32; i++ {
			blakeInput[i] = mix[i] ^ node[i]
			blakeInput[32+i] = node[32+i]
		}

		sum := blake3.Sum256(blakeInput[:])
		copy(mix[:], sum[:])
	}

	finalInput := make([]byte, 64+32)
	copy(finalInput[:64], seed512[:])
	copy(finalInput[64:], mix[:])

	final512 := sha3.Sum512(finalInput)

	var hash256 [32]byte
	copy(hash256[:], final512[:32])

	return hash256, final512
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

func lessOrEqualBE(a [32]byte, b [32]byte) bool {
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
