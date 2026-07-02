package tool

import (
	"context"
	"testing"
)

func TestOfName(t *testing.T) {
	tests := []struct {
		name string
		want Tool
	}{
		{"code_comment", CodeComment},
		{"file_read", FileRead},
		{"file_find", FileFind},
		{"file_read_diff", FileReadDiff},
		{"code_search", CodeSearch},
		{"task_done", TaskDone},
		{"nonexistent", Unknown},
		{"", Unknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := OfName(tt.name)
			if got != tt.want {
				t.Errorf("OfName(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestTool_Name(t *testing.T) {
	if CodeComment.Name() != "code_comment" {
		t.Errorf("CodeComment.Name() = %q", CodeComment.Name())
	}
}

func TestTool_IsKnown(t *testing.T) {
	if !CodeComment.IsKnown() {
		t.Error("CodeComment should be known")
	}
	if Unknown.IsKnown() {
		t.Error("Unknown should not be known")
	}
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	reg := NewRegistry()
	stub := NewStub(CodeComment)
	reg.Register(stub)

	got, ok := reg.Get("code_comment")
	if !ok {
		t.Fatal("expected to find code_comment")
	}
	if got.Tool() != CodeComment {
		t.Errorf("got tool %v, want CodeComment", got.Tool())
	}

	_, ok = reg.Get("nonexistent")
	if ok {
		t.Error("should not find nonexistent tool")
	}
}

func TestRegistry_Freeze_PanicsOnRegister(t *testing.T) {
	reg := NewRegistry()
	reg.Freeze()

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on Register after Freeze")
		}
	}()
	reg.Register(NewStub(FileRead))
}

func TestRegistry_GetAfterFreeze(t *testing.T) {
	reg := NewRegistry()
	reg.Register(NewStub(FileRead))
	reg.Freeze()

	_, ok := reg.Get("file_read")
	if !ok {
		t.Error("should still find tools after Freeze")
	}
}

func TestIsReserved(t *testing.T) {
	reserved := []string{"unknown", "task_done", "code_comment", "file_read", "file_find", "file_read_diff", "code_search"}
	for _, name := range reserved {
		if !IsReserved(name) {
			t.Errorf("IsReserved(%q) = false, want true", name)
		}
	}

	nonReserved := []string{"custom_tool", "my_tool", "search", ""}
	for _, name := range nonReserved {
		if IsReserved(name) {
			t.Errorf("IsReserved(%q) = true, want false", name)
		}
	}
}

func TestDynamic(t *testing.T) {
	tool := Dynamic("my_custom_tool")
	if tool.Name() != "my_custom_tool" {
		t.Errorf("Dynamic().Name() = %q, want %q", tool.Name(), "my_custom_tool")
	}
}

func TestDynamic_PanicsOnEmpty(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for empty name")
		}
	}()
	Dynamic("")
}

func TestDynamic_PanicsOnReserved(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for reserved name")
		}
	}()
	Dynamic("file_read")
}

type dummyProvider struct{}

func (d *dummyProvider) Tool() Tool { return CodeSearch }
func (d *dummyProvider) Execute(_ context.Context, _ map[string]any) (string, error) {
	return "result", nil
}

func TestRegistry_ProviderInterface(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&dummyProvider{})
	reg.Freeze()

	p, ok := reg.Get("code_search")
	if !ok {
		t.Fatal("code_search not found")
	}
	result, err := p.Execute(context.Background(), nil)
	if err != nil || result != "result" {
		t.Errorf("Execute() = %q, %v", result, err)
	}
}
