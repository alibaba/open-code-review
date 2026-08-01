package diff

import (
	"testing"
)

func TestParseHunks_SingleHunk(t *testing.T) {
	raw := `diff --git a/pkg/example/handler.go b/pkg/example/handler.go
--- a/pkg/example/handler.go
+++ b/pkg/example/handler.go
@@ -10,7 +10,7 @@ func HandleRequest(w http.ResponseWriter, r *http.Request) {
     ctx := r.Context()
-    log.Print("handling request")
+    log.Printf("handling request: %s", r.URL.Path)
     err := process(ctx)`

	hunks := ParseHunks(raw)

	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}
	h := hunks[0]
	if h.OldStart != 10 || h.OldCount != 7 {
		t.Errorf("OldStart/OldCount: expected 10,7 got %d,%d", h.OldStart, h.OldCount)
	}
	if h.NewStart != 10 || h.NewCount != 7 {
		t.Errorf("NewStart/NewCount: expected 10,7 got %d,%d", h.NewStart, h.NewCount)
	}
	if len(h.Lines) != 4 {
		t.Fatalf("expected 4 lines, got %d", len(h.Lines))
	}

	// Check line types in order
	expected := []HunkLineType{HunkContext, HunkDeleted, HunkAdded, HunkContext}
	for i, lt := range expected {
		if h.Lines[i].Type != lt {
			t.Errorf("line[%d]: type %d expected %d", i, h.Lines[i].Type, lt)
		}
	}
}

func TestParseHunks_MultipleHunks(t *testing.T) {
	raw := `diff --git a/pkg/example/handler.go b/pkg/example/handler.go
--- a/pkg/example/handler.go
+++ b/pkg/example/handler.go
@@ -10,3 +10,3 @@ func foo() {
     a := 1
-    b := 2
+    b := 3
     c := 4
@@ -25,6 +25,8 @@ func bar() {
     if err != nil {
         return err
     }
+    log.Print("ok")
+    log.Print("done")
     return nil`

	hunks := ParseHunks(raw)
	if len(hunks) != 2 {
		t.Fatalf("expected 2 hunks, got %d", len(hunks))
	}

	h1 := hunks[0]
	if h1.OldStart != 10 || h1.NewStart != 10 {
		t.Errorf("hunk 0: OldStart=%d NewStart=%d", h1.OldStart, h1.NewStart)
	}

	h2 := hunks[1]
	if h2.OldStart != 25 || h2.NewStart != 25 {
		t.Errorf("hunk 1: OldStart=%d NewStart=%d", h2.OldStart, h2.NewStart)
	}
	if h2.OldCount != 6 || h2.NewCount != 8 {
		t.Errorf("hunk 1 counts: OldCount=%d NewCount=%d", h2.OldCount, h2.NewCount)
	}
}

func TestParseHunks_NoNewlineMarker(t *testing.T) {
	raw := `@@ -1,2 +1,2 @@
-    old line
\ No newline at end of file
+    new line`

	hunks := ParseHunks(raw)
	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}
	if len(hunks[0].Lines) != 2 {
		t.Errorf("expected 2 lines (excluding no-newline marker), got %d", len(hunks[0].Lines))
	}
}

func TestParseHunks_EmptyInput(t *testing.T) {
	hunks := ParseHunks("")
	if len(hunks) != 0 {
		t.Errorf("expected 0 hunks, got %d", len(hunks))
	}
}

func TestParseHunks_NewFileAllAdditions(t *testing.T) {
	raw := `diff --git a/pkg/new.go b/pkg/new.go
new file mode 100644
--- /dev/null
+++ b/pkg/new.go
@@ -0,0 +1,3 @@
+package pkg
+
+func New() {}`

	hunks := ParseHunks(raw)
	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}
	h := hunks[0]
	for _, l := range h.Lines {
		if l.Type != HunkAdded {
			t.Errorf("expected all lines to be HunkAdded, got %d", l.Type)
		}
	}
}
