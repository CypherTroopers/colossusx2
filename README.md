`colossusx` is a minimal COLOSSUS-X research miner written in Go. It runs as a single binary, generates its DAG at startup, and then executes either a benchmark or a mining loop across selectable `unified`, `cpu`, and `gpu` backends.

This README was written by inspecting the current codebase so that a new user can go from a fresh environment to a verified local run.

## Prerequisites

- OS: macOS, Linux, or WSL (any environment that can run Go)
- Go: **1.23 or newer**
  - `go.mod` specifies `go 1.23.0`
- Network access for the first dependency download

## Setup

### 1. Clone the repository

```bash
git clone https://github.com/CypherTroopers/colossusx.git
cd colossusx
```

> If you already have the repository checked out locally, you can skip this step.

### 2. Verify your Go version

```bash
go version
```

You should see `go1.23.x` or newer.

### 3. Download dependencies

In most cases, `go run` and `go build` will download modules automatically. If you want to do it explicitly:

```bash
go mod download
```

## Project layout

This repository is intentionally small. The main files are:

- `main.go`: CLI entrypoint, DAG generation, benchmark loop, and mining loop
- `go.mod`: module definition and Go version
- `go.sum`: dependency checksums
- `Makefile`: convenience targets for the most common local workflows

## Quick start

### Option A: use the Makefile

The easiest way to verify the project is with the provided Make targets:

```bash
make help
make bench-small
make bench-cpu
make mine-easy
```

### Option B: run commands directly

#### Show CLI help

```bash
go run . -h
```

Main flags:

- `-bench`: run the hash-loop benchmark only
- `-backend`: select `unified`, `cpu`, or `gpu` mining backend
- `-dag-mib`: DAG size in MiB
- `-reads`: random DAG reads per hash
- `-workers`: worker count
- `-epoch-blocks`: blocks per epoch
- `-epoch-seed`: hex seed used for DAG generation
- `-header`: input header as hex
- `-target`: 32-byte big-endian target
- `-start-nonce`: starting nonce
- `-max-nonces`: nonce attempts to try; `0` means unbounded


### Mining backends

The binary now exposes three backend modes:

- `-backend unified`: the original unified-memory-oriented contiguous DAG layout and CPU hash path
- `-backend cpu`: explicit CPU mining mode using the same DAG and hash function
- `-backend gpu`: available in `-tags opencl` builds as an experimental GPU-ready mode; the default build returns a clear error, and the OpenCL build currently reuses the CPU hash path until a dedicated kernel is wired in

For most users today, `unified` and `cpu` are the working modes in this repository.

## Fastest verified local run

The quickest way to confirm the program works is to run a small benchmark with a 1 MiB DAG:

```bash
go run . -bench -backend unified -dag-mib 1 -max-nonces 1000 -workers 2
```

Expected flow:

1. The miner configuration is printed.
2. The DAG is generated.
3. `benchmark complete` is printed.
4. `hashes`, `elapsed`, and `hashrate` are shown.

The equivalent Make target is:

```bash
make bench-small
```

## Verified mining run

With the default target, you may not find a solution quickly. For a simple execution check, use an intentionally easy target:

```bash
go run . \
  -dag-mib 1 \
  -workers 2 \
  -max-nonces 10 \
  -target ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff
```

This makes it easy to confirm output such as:

- `solution found`
- `nonce`
- `hash256`
- `hash512`
- `elapsed`
- `hashes`
- `hashrate`

The equivalent Make target is:

```bash
make mine-easy
```

## More realistic examples

A more research-like benchmark run:

```bash
go run . -bench -dag-mib 8192 -reads 64 -workers 4 -max-nonces 200000
go run . -bench -dag-mib 16384 -reads 64 -workers 4 -max-nonces 200000
go run . -bench -dag-mib 32768 -reads 64 -workers 4 -max-nonces 200000
```

Or regular mining:

```bash
go run . -dag-mib 8192 -reads 64 -workers 4 -max-nonces 200000
go run . -dag-mib 16384 -reads 64 -workers 4 -max-nonces 200000
go run . -dag-mib 32768 -reads 64 -workers 4 -max-nonces 200000
```

## Build a binary

If you prefer a compiled binary over `go run`:

```bash
go build -o bin/colossusx .
./bin/colossusx -bench -dag-mib 1 -max-nonces 1000 -workers 2
```

Or use the Make target:

```bash
make build
./bin/colossusx -bench -dag-mib 1 -max-nonces 1000 -workers 2
```

## Code flow overview

At a high level, the program does the following:

1. Parse CLI flags.
2. Build a `Spec` and validate it.
3. Decode `header`, `epoch-seed`, and `target` from hex.
4. Allocate the DAG buffer.
5. Generate the DAG with `GenerateDAG`.
6. Run `Benchmark` if `-bench` is enabled.
7. Otherwise run `Mine` and search for a valid hash.

## Common pitfalls

### Go version is too old

If your Go toolchain is older than the version specified in `go.mod`, the build may fail.

### DAG size is too large for your machine

`-dag-mib` directly affects memory usage. For larger research runs, prefer `8192` (8 GiB), `16384` (16 GiB), or `32768` (32 GiB).

### `header`, `epoch-seed`, and `target` must be hex

Invalid hex input will cause startup errors. In particular, `-target` must be exactly **32 bytes / 64 hex characters**.

### A default mining run may not find a solution

In mining mode, the program exits with `no solution found in range` if no solution exists within the specified nonce range. For a simple smoke test, use the easy target shown above.

## Make targets

```bash
make help
```

Available targets:

- `make help`: list available targets
- `make deps`: download Go module dependencies
- `make build`: build `bin/colossusx`
- `make run-help`: print the CLI help
- `make bench-small`: run the verified small benchmark
- `make mine-easy`: run the verified easy-target mining example
- `make clean`: remove build artifacts

## Minimum verification checklist

### Show help

```bash
go run . -h
```

### Run the benchmark smoke test

```bash
go run . -bench -backend unified -dag-mib 1 -max-nonces 1000 -workers 2
```

### Run the easy-target mining smoke test

```bash
go run . -dag-mib 1 -workers 2 -max-nonces 10 -target ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff
```
