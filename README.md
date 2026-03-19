# COLOSSUS-X

This repository now separates a spec-locked `colossusx` core package from the CLI and execution backends.

## Architecture

- `main.go` is orchestration only: CLI parsing, config wiring, DAG allocation selection, miner startup, and result printing.
- `colossusx/` owns the COLOSSUS-X semantics: strict constants, DAG generation, lattice hashing, mining flow, and deterministic big-endian target comparison.
- `unified` and `cpu` backends are execution strategies over the same core algorithm.
- `gpu` remains explicitly disabled unless a future implementation is proven hash-equivalent; this repository does **not** claim working GPU acceleration today.

## Strict COLOSSUS-X spec

Strict mode centralizes and enforces these constants inside `colossusx/spec.go`:

- `DAG_SIZE = 80 * 1024^3`
- `NODE_SIZE = 64`
- `READS_PER_H = 512`
- `EPOCH_BLOCKS = 8000`

Strict mode rejects CLI attempts to override DAG size, reads per hash, or epoch blocks.

## Research / development compatibility

A compatibility path still exists for development and testing:

- use `-mode research`
- use smaller DAGs or alternate read counts for local tests
- keep the exact same core algorithm implementation
- prefer real unified-memory-capable allocations when available (CUDA managed memory / OpenCL SVM), while keeping Go heap as the fallback memory model

This compatibility mode does **not** change the strict path or silently weaken strict-mode validation.

The codebase now also carries a nonce abstraction at the hashing boundary so a future 256-bit nonce upgrade can be introduced without rewriting every backend at once; the active miner still iterates a uint64 nonce range today.

## Algorithm summary

### 1. DAG generation

For each node index `i`:

- allocate a unified-memory-compatible DAG buffer abstraction
- compute `dag[i] = keccak512(epoch_seed ++ i)`

### 2. Lattice hash

- `seed = sha3_512(header ++ nonce_bytes)`
- `mix = first 32 bytes of seed`
- repeat `READS_PER_H` rounds:
  - `node_idx = fnv1a(mix ++ round_index) % NODE_COUNT`
  - `node_data = dag[node_idx]`
  - `mix = blake3((mix XOR node_data[0:32]) ++ (mix XOR node_data[32:64]))`
- `result = sha3_512(seed ++ mix)`

### 3. Mining loop

- each backend calls the same core lattice hash implementation
- target comparison is deterministic and big-endian

## CLI modes

### Strict mode

Default mode:

```bash
go run . -mode strict -bench -backend unified
```

Because strict mode uses the real 80 GiB DAG constant, it is mainly intended for production-grade orchestration and spec verification.

### Research mode

For development and tests:

```bash
go run . -mode research -bench -backend unified -dag-mib 1 -reads 8 -max-nonces 1000
```

## Backends

- `unified`: primary long-term memory model; operates directly over the shared DAG allocation.
- `cpu`: prepares its own CPU-side node table but still calls the same `colossusx.LatticeHash` logic.
- `gpu`: intentionally unavailable in default builds and still described as disabled until hash parity is proven.

## Testing

Run:

```bash
go test ./... -count=1
```

The test suite covers:

- DAG determinism for identical seeds
- DAG divergence for different seeds
- lattice hash determinism
- strict-mode constant enforcement
- target comparison correctness
- backend parity between unified and cpu
- practical CLI parsing / mode regression checks
