package main

type BatchHashBackend interface {
	HashBatch(header []byte, startNonce uint64, count uint64, dag *DAG) ([]HashResult, error)
}
