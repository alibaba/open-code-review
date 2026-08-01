package tool

import (
	"context"
	"testing"
)

func TestStubProvider_Tool(t *testing.T) {
	s := NewStub(FileRead)
	if s.Tool() != FileRead {
		t.Errorf("Tool() = %v, want FileRead", s.Tool())
	}
}

func TestStubProvider_Execute(t *testing.T) {
	s := NewStub(FileRead)
	got, err := s.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != NotAvailableMsg {
		t.Errorf("Execute() = %q, want %q", got, NotAvailableMsg)
	}
}

func TestBuiltinToolProvider(t *testing.T) {
	called := false
	fn := func(_ context.Context, args map[string]any) (string, error) {
		called = true
		return "result", nil
	}

	b := NewBuiltin(TaskDone, fn)
	if b.Tool() != TaskDone {
		t.Errorf("Tool() = %v, want TaskDone", b.Tool())
	}

	got, err := b.Execute(context.Background(), map[string]any{"key": "val"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected fn to be called")
	}
	if got != "result" {
		t.Errorf("Execute() = %q, want %q", got, "result")
	}
}
