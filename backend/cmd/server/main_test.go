package main

import (
	"context"
	"errors"
	"testing"
)

type fakeNamespaceEnsurer struct {
	calls int
	err   error
}

func (f *fakeNamespaceEnsurer) EnsureNamespace(context.Context) error {
	f.calls++
	return f.err
}

func TestEnsureSandboxNamespaceIfRequiredSkipsWhenNotRequired(t *testing.T) {
	ensurer := &fakeNamespaceEnsurer{}

	if err := ensureSandboxNamespaceIfRequired(context.Background(), ensurer, false); err != nil {
		t.Fatalf("ensureSandboxNamespaceIfRequired returned error: %v", err)
	}
	if ensurer.calls != 0 {
		t.Fatalf("EnsureNamespace called %d times, want 0", ensurer.calls)
	}
}

func TestEnsureSandboxNamespaceIfRequiredCallsEnsurer(t *testing.T) {
	ensurer := &fakeNamespaceEnsurer{}

	if err := ensureSandboxNamespaceIfRequired(context.Background(), ensurer, true); err != nil {
		t.Fatalf("ensureSandboxNamespaceIfRequired returned error: %v", err)
	}
	if ensurer.calls != 1 {
		t.Fatalf("EnsureNamespace called %d times, want 1", ensurer.calls)
	}
}

func TestEnsureSandboxNamespaceIfRequiredReturnsEnsurerError(t *testing.T) {
	ensurer := &fakeNamespaceEnsurer{err: errors.New("forbidden")}

	err := ensureSandboxNamespaceIfRequired(context.Background(), ensurer, true)
	if err == nil || err.Error() != "forbidden" {
		t.Fatalf("ensureSandboxNamespaceIfRequired error = %v, want forbidden", err)
	}
	if ensurer.calls != 1 {
		t.Fatalf("EnsureNamespace called %d times, want 1", ensurer.calls)
	}
}
