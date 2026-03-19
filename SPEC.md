# COLOSSUS-X Specification

## Formal Name

**COLOSSUS-X**  
**Colossal Memory Optimized Lattice Operation & Storage System for Unified eXascale**

## Version

**1.0.0**

## Tagline

**Designed for 96–128 GB VRAM Architectures**

---

## Overview

COLOSSUS-X is a mining algorithm designed with a **memory-bandwidth-first** philosophy.

Unlike traditional GPU mining algorithms optimized for 8–24 GB VRAM devices, COLOSSUS-X is built for **large-capacity unified-memory GPU architectures**, such as AMD AI Max+ 395 and Nvidia GB10.

The algorithm uses a massive **Directed Acyclic Graph (DAG)** that cannot be practically resident on ordinary consumer GPUs. Mining is based on **graph-traversal-driven memory-hard computation**, where sustained random-access bandwidth is the primary bottleneck rather than raw arithmetic throughput.

---

## Architecture

### 1. Titan DAG — 80 GB Working Set

**Parameters**
- DAG Size: **80 GB**
- Node Size: **64 bytes**
- Node Count: **~1.28 billion**
- Epoch Length: **8,000 blocks**

**Properties**
- The full 80 GB DAG is intended to remain resident in VRAM / unified memory.
- Hashing performs random 64-byte reads across the entire DAG.
- External memory substitution or practical CPU-side solving is intentionally discouraged by the size and bandwidth requirements.
- The DAG is regenerated every 8,000 blocks.

---

### 2. Bandwidth Storm — 3.2 TB/s Demand

**Parameters**
- Reads per Hash: **512**
- Target Throughput: **50 MH/s**
- Required Memory Bandwidth: **~3.2 TB/s**
- Target Memory Types: **HBM3e / LPDDR5X**

**Properties**
- Each hash performs 512 random DAG accesses.
- At the target throughput, the memory subsystem becomes the dominant performance constraint.
- The algorithm is intentionally designed so that memory bandwidth matters more than compute throughput.

---

### 3. LatticeHash Core — SHA3 + Blake3 Hybrid

**Parameters**
- Hash Functions: **SHA3-512 + Blake3**
- Nonce Width: **256-bit target design**
- Output: **512-bit digest**
- Difficulty Model: **Adaptive, 120-second blocks**

**Properties**
- SHA3-512 provides strong cryptographic mixing and final compression.
- Blake3 is used in the iterative mix-update path to exploit efficient parallel-friendly hashing behavior.
- DAG access is interleaved with the hash rounds so memory operations and hash mixing are tightly coupled.
- The design target is a 256-bit nonce space; implementations may stage migration work through narrower internal nonce types so long as the hashing boundary is abstracted for a later 256-bit upgrade.

---

### 4. Zero-Copy Unified Memory Pipeline

**Parameters**
- PCIe Transfer Overhead: **0 bytes**
- Copy Operations: **None**
- CPU Validation: **Shared-pointer DAG**
- Beneficial Architectures: **AMD/Nvidia UMA only**

**Properties**
- CPU and GPU share the same physical memory pool.
- PCIe copy overhead is eliminated.
- The CPU validation path can directly access the same DAG memory region used by the GPU.

---

### 5. Adaptive Work Partitioner

**Parameters**
- Partitions: **16 × 5 GB**
- Ownership: **Per CU / SM cluster**
- Rebalance Trigger: **Thermal + Power telemetry**
- Cache Strategy: **L2 locality-aware**

**Properties**
- The 80 GB DAG is partitioned into sixteen 5 GB regions.
- Each CU / SM cluster is intended to primarily operate on an assigned region.
- Work can be dynamically rebalanced according to thermal and power telemetry.
- The scheduler should prefer locality where possible.

---

### 6. ASIC / CPU Resistance Layer

**Parameters**
- Minimum Required VRAM: **88 GB**
- Typical CPU DDR5 Bandwidth: **~100 GB/s**
- ASIC Practicality: **Low**
- Typical Discrete GPU Viability: **No**

**Properties**
- The 80 GB DAG makes large on-die memory integration economically prohibitive for ASICs.
- CPU memory bandwidth is far below the intended operating envelope.
- Typical 16–32 GB discrete GPUs cannot hold the DAG and are therefore not target hardware.

---

## Design Principles

### Memory-Bandwidth First
The primary bottleneck is intended to be **memory bandwidth**, not raw arithmetic throughput.

### Unified Memory Exploitation
The 80 GB DAG is intended to be **shared with zero-copy semantics** between CPU and GPU on suitable architectures.

### Egalitarian by Design
The algorithm intentionally targets only systems with sufficiently large unified-memory pools and bandwidth.

### Long Epoch Stability
An 8,000-block epoch reduces DAG regeneration overhead and keeps the memory image stable for a long interval.

---

## Normative Constants

```text
CONST DAG_SIZE     = 80 * 1024^3
CONST NODE_SIZE    = 64
CONST READS_PER_H  = 512
CONST EPOCH_BLOCKS = 8000
```

## Normative Algorithm

### DAG Generation

For node index `i`, using `i` encoded as an unsigned 64-bit little-endian integer in the current implementation profile:

```text
dag[i] = keccak512(epoch_seed ++ i)
```

### LatticeHash

```text
seed = sha3_512(header ++ nonce_bytes)
mix  = seed[0:32]
for round in 0..READS_PER_H-1:
    node_idx = fnv1a64(mix ++ le64(round)) mod NODE_COUNT
    node     = dag[node_idx]                  // 64 bytes
    blake_in = (mix XOR node[0:32]) ++ (mix XOR node[32:64])
    mix      = blake3_256(blake_in)           // 32 bytes
result = sha3_512(seed ++ mix)
```

`mix` is always 32 bytes. `node` is always 64 bytes. The Blake3 round input is therefore the deterministic 64-byte concatenation of the mix XORed with each 32-byte half of the node.

The current codebase introduces a nonce abstraction at the hashing boundary to make a later 256-bit nonce upgrade straightforward, but the active mining loop still steps through a uint64-backed nonce range today.

