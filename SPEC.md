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

While the strict production-oriented profile targets an **80 GB DAG**, implementations may also expose **32 GB, 16 GB, and 8 GB research profiles** for development, validation, benchmarking, and staged implementation work. These smaller profiles are explicitly **non-strict** and must not be confused with the primary COLOSSUS-X deployment target.

---

## Architecture

### 1. Titan DAG — Primary and Research Working Sets

#### Primary Strict Profile

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

#### Research / Development Profiles

**Parameters**
- DAG Size: **32 GB**
- DAG Size: **16 GB**
- DAG Size: **8 GB**
- Node Size: **64 bytes**
- Epoch Length: **8,000 blocks**

**Approximate Node Counts**
- **32 GB**: ~536.9 million nodes
- **16 GB**: ~268.4 million nodes
- **8 GB**: ~134.2 million nodes

**Properties**
- These smaller DAG profiles exist for research, staged implementation, testing, and benchmarking.
- They preserve the same node size and overall lattice-hash structure.
- They are not equivalent to the strict 80 GB resistance profile.
- They must be treated as **non-strict / research-only** execution modes.

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
- Partitions: **16 × 5 GB** in the 80 GB strict profile
- Ownership: **Per CU / SM cluster**
- Rebalance Trigger: **Thermal + Power telemetry**
- Cache Strategy: **L2 locality-aware**

**Properties**
- In the strict profile, the 80 GB DAG is partitioned into sixteen 5 GB regions.
- Each CU / SM cluster is intended to primarily operate on an assigned region.
- Work can be dynamically rebalanced according to thermal and power telemetry.
- The scheduler should prefer locality where possible.

**Research-profile note**
- Smaller research profiles may use proportionally smaller partition layouts while preserving the same scheduling model.
- Such layouts are implementation profiles, not changes to the strict 80 GB algorithm target.

---

### 6. ASIC / CPU Resistance Layer

**Parameters**
- Minimum Required VRAM for strict profile: **88 GB**
- Typical CPU DDR5 Bandwidth: **~100 GB/s**
- ASIC Practicality: **Low**
- Typical Discrete GPU Viability: **No** for the strict profile

**Properties**
- The 80 GB DAG makes large on-die memory integration economically prohibitive for ASICs.
- CPU memory bandwidth is far below the intended operating envelope.
- Typical 16–32 GB discrete GPUs cannot hold the strict DAG and are therefore not target hardware.

**Research-profile note**
- 32 GB, 16 GB, and 8 GB modes may be executable on smaller systems for development purposes.
- Those profiles intentionally relax the strict memory-residency requirement and therefore do not provide the same resistance properties as the 80 GB strict profile.

---

## Design Principles

### Memory-Bandwidth First
The primary bottleneck is intended to be **memory bandwidth**, not raw arithmetic throughput.

### Unified Memory Exploitation
The 80 GB DAG is intended to be **shared with zero-copy semantics** between CPU and GPU on suitable architectures.

### Egalitarian by Design
The algorithm intentionally targets only systems with sufficiently large unified-memory pools and bandwidth in strict mode.

### Long Epoch Stability
An 8,000-block epoch reduces DAG regeneration overhead and keeps the memory image stable for a long interval.

---

## Normative Constants

```text
CONST DAG_SIZE     = 80 * 1024^3
CONST NODE_SIZE    = 64
CONST READS_PER_H  = 512
CONST EPOCH_BLOCKS = 8000
