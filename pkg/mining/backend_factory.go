package mining

import (
	"fmt"
	"strings"

	cx "colossusx/colossusx"
)

func ParseBackendMode(s string) (cx.BackendMode, error) {
	switch mode := cx.BackendMode(strings.ToLower(strings.TrimSpace(s))); mode {
	case cx.BackendUnified, cx.BackendCPU, cx.BackendGPU:
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported backend %q (expected one of: %s, %s, %s)", s, cx.BackendUnified, cx.BackendCPU, cx.BackendGPU)
	}
}

func NewBackend(mode string) (cx.HashBackend, error) {
	parsed, err := ParseBackendMode(mode)
	if err != nil {
		return nil, err
	}
	switch parsed {
	case cx.BackendUnified:
		return &UnifiedBackend{}, nil
	case cx.BackendCPU:
		return &CPUBackend{}, nil
	case cx.BackendGPU:
		return NewGPUBackend()
	default:
		return nil, fmt.Errorf("unsupported backend %q", mode)
	}
}
