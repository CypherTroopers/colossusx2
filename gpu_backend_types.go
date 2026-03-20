package main

type GPUMemoryModel string

const (
	GPUMemoryModelDiscrete GPUMemoryModel = "discrete-copy"
	GPUMemoryModelUnified  GPUMemoryModel = "unified-shared"
)

type GPUExecutionPlan struct {
	KernelName   string
	GlobalSize   int
	LocalSize    int
	BatchNonces  int
	MemoryModel  GPUMemoryModel
	VerifySample int
	Fallback     string
	UsedFallback bool
	CopiedDAG    bool
}
