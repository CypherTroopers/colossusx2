//go:build !cuda || !cgo

package mining

import cx "colossusx/colossusx"

func allocPinnedHost(size uint64) (cx.Allocation, error) {
	return &sliceAllocation{name: "pinned-host", buf: make([]byte, size)}, nil
}
