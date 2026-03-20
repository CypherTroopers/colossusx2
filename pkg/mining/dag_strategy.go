package mining

import cx "colossusx/colossusx"

func ResolveDAGStrategy(mode cx.BackendMode, dagAlloc string, runtime RuntimeState) (cx.Allocator, error) {
	return dagStrategyResolver{backend: mode, runtime: runtime}.Resolve(dagAlloc)
}
