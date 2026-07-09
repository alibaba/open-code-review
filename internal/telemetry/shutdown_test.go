package telemetry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestShutdown_NoFuncs(t *testing.T) {
	shutdownFuncs = nil
	err := Shutdown(context.Background())
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestShutdown_Success(t *testing.T) {
	called := false
	shutdownFuncs = []func(context.Context) error{
		func(ctx context.Context) error {
			called = true
			return nil
		},
	}
	defer func() { shutdownFuncs = nil }()

	err := Shutdown(context.Background())
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if !called {
		t.Error("expected shutdown function to be called")
	}
	if shutdownFuncs != nil {
		t.Error("expected shutdownFuncs to be cleared after shutdown")
	}
}

func TestShutdown_WithErrors(t *testing.T) {
	shutdownFuncs = []func(context.Context) error{
		func(ctx context.Context) error { return nil },
		func(ctx context.Context) error { return errors.New("fail1") },
		func(ctx context.Context) error { return errors.New("fail2") },
	}
	defer func() { shutdownFuncs = nil }()

	err := Shutdown(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if shutdownFuncs != nil {
		t.Error("expected shutdownFuncs to be cleared even on error")
	}
}

func TestShutdownWithTimeout(t *testing.T) {
	called := false
	shutdownFuncs = []func(context.Context) error{
		func(ctx context.Context) error {
			called = true
			return nil
		},
	}
	defer func() { shutdownFuncs = nil }()

	ShutdownWithTimeout(context.Background(), 5*time.Second)
	if !called {
		t.Error("expected shutdown function to be called via ShutdownWithTimeout")
	}
}

func TestShutdownWithTimeout_Error(t *testing.T) {
	shutdownFuncs = []func(context.Context) error{
		func(ctx context.Context) error { return errors.New("shutdown fail") },
	}
	defer func() { shutdownFuncs = nil }()

	ShutdownWithTimeout(context.Background(), 5*time.Second)
}
