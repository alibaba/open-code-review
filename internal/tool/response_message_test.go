// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import "testing"

func TestComplete(t *testing.T) {
	cp := Complete()
	if !cp.Completed {
		t.Error("Complete() should set Completed=true")
	}
	if cp.Failed {
		t.Error("Complete() should set Failed=false")
	}
	if cp.Data != "" {
		t.Errorf("Complete() Data = %q, want empty", cp.Data)
	}
}

func TestFail(t *testing.T) {
	cp := Fail("task failed")
	if cp.Completed {
		t.Error("Fail() should set Completed=false")
	}
	if !cp.Failed {
		t.Error("Fail() should set Failed=true")
	}
	if cp.Data != "task failed" {
		t.Errorf("Fail() Data = %q, want %q", cp.Data, "task failed")
	}
}

func TestOf(t *testing.T) {
	cp := Of("hello")
	if cp.Completed {
		t.Error("Of() should set Completed=false")
	}
	if cp.Failed {
		t.Error("Of() should set Failed=false")
	}
	if cp.Data != "hello" {
		t.Errorf("Of() Data = %q, want %q", cp.Data, "hello")
	}
}

func TestOf_Empty(t *testing.T) {
	cp := Of("")
	if cp.Completed {
		t.Error("Of(\"\") should set Completed=false")
	}
	if cp.Data != "" {
		t.Errorf("Of(\"\") Data = %q, want empty", cp.Data)
	}
}
