package mining

import cx "colossusx/colossusx"

type Spec = cx.Spec
type Target = cx.Target
type HashResult = cx.HashResult
type DAG = cx.DAG
type Miner = cx.Miner
type MineResult = cx.MineResult
type HashBackend = cx.HashBackend
type BackendMode = cx.BackendMode

const (
	BackendUnified = cx.BackendUnified
	BackendCPU     = cx.BackendCPU
	BackendGPU     = cx.BackendGPU
)
