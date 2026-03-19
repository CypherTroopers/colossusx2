package main

import (
	"strings"
	"testing"
)

func TestGPUBackendFailsExplicitlyWhenDispatchIsStub(t *testing.T) {
	backend, err := NewGPUBackend()
	if err == nil {
		t.Fatal("expected NewGPUBackend to fail while GPU dispatch is disabled")
	}
	if backend != nil {
		t.Fatal("expected no backend when GPU path is disabled")
	}
	if !strings.Contains(err.Error(), "hash-equivalent") {
		t.Fatalf("expected explicit GPU disablement error, got %v", err)
	}
}
