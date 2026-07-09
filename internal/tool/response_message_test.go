package tool

import "testing"

func TestComplete(t *testing.T) {
	cp := Complete()
	if !cp.Completed {
		t.Error("Complete() should set Completed=true")
	}
	if cp.Data != "" {
		t.Errorf("Complete() Data = %q, want empty", cp.Data)
	}
}

func TestOf(t *testing.T) {
	cp := Of("hello")
	if cp.Completed {
		t.Error("Of() should set Completed=false")
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
