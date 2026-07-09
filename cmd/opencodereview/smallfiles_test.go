package main

import (
	"runtime"
	"strings"
	"testing"
)

func TestPrintVersion_Dev(t *testing.T) {
	origVersion := Version
	origCommit := GitCommit
	origDate := BuildDate
	defer func() {
		Version = origVersion
		GitCommit = origCommit
		BuildDate = origDate
	}()

	Version = "dev"
	GitCommit = ""
	BuildDate = ""

	got := captureStdout(t, func() {
		printVersion()
	})
	if !strings.Contains(got, "open-code-review dev") {
		t.Errorf("expected 'open-code-review dev', got %q", got)
	}
	if !strings.Contains(got, runtime.GOOS+"/"+runtime.GOARCH) {
		t.Errorf("expected OS/ARCH, got %q", got)
	}
}

func TestPrintVersion_WithCommitAndDate(t *testing.T) {
	origVersion := Version
	origCommit := GitCommit
	origDate := BuildDate
	defer func() {
		Version = origVersion
		GitCommit = origCommit
		BuildDate = origDate
	}()

	Version = "1.2.3"
	GitCommit = "abc1234"
	BuildDate = "2026-01-01"

	got := captureStdout(t, func() {
		printVersion()
	})
	if !strings.Contains(got, "1.2.3") {
		t.Errorf("expected version, got %q", got)
	}
	if !strings.Contains(got, "abc1234") {
		t.Errorf("expected commit, got %q", got)
	}
	if !strings.Contains(got, "2026-01-01") {
		t.Errorf("expected build date, got %q", got)
	}
}

func TestParseViewerFlags_Defaults(t *testing.T) {
	opts, err := parseViewerFlags(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.addr != "localhost:5483" {
		t.Errorf("addr = %q, want localhost:5483", opts.addr)
	}
	if opts.showHelp {
		t.Error("showHelp should be false")
	}
}

func TestParseViewerFlags_CustomAddr(t *testing.T) {
	opts, err := parseViewerFlags([]string{"--addr", ":3000"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.addr != ":3000" {
		t.Errorf("addr = %q, want :3000", opts.addr)
	}
}

func TestParseViewerFlags_Help(t *testing.T) {
	opts, err := parseViewerFlags([]string{"-h"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.showHelp {
		t.Error("expected showHelp=true")
	}
}

func TestRunLLM_NoArgs(t *testing.T) {
	got := captureStdout(t, func() {
		err := runLLM(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if !strings.Contains(got, "LLM utility") {
		t.Errorf("expected usage text, got %q", got)
	}
}

func TestRunLLM_UnknownSubcommand(t *testing.T) {
	err := runLLM([]string{"bogus"})
	if err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
	if !strings.Contains(err.Error(), "unknown llm sub-command") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunRules_NoArgs(t *testing.T) {
	got := captureStdout(t, func() {
		err := runRules(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if !strings.Contains(got, "ocr rules") {
		t.Errorf("expected usage text, got %q", got)
	}
}

func TestRunRules_Help(t *testing.T) {
	got := captureStdout(t, func() {
		err := runRules([]string{"-h"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if !strings.Contains(got, "ocr rules") {
		t.Errorf("expected usage text, got %q", got)
	}
}

func TestRunRules_UnknownSubcommand(t *testing.T) {
	err := runRules([]string{"bogus"})
	if err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
}

func TestRunRules_HelpAltFlag(t *testing.T) {
	got := captureStdout(t, func() {
		err := runRules([]string{"--help"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if !strings.Contains(got, "ocr rules") {
		t.Errorf("expected usage text, got %q", got)
	}
}

func TestRunRulesCheck_Help(t *testing.T) {
	got := captureStdout(t, func() {
		err := runRulesCheck([]string{"-h"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if !strings.Contains(got, "ocr rules check") {
		t.Errorf("expected usage text, got %q", got)
	}
}

func TestRunRulesCheck_NoArgs(t *testing.T) {
	got := captureStdout(t, func() {
		err := runRulesCheck(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if !strings.Contains(got, "ocr rules check") {
		t.Errorf("expected usage text, got %q", got)
	}
}

func TestRunLLMProviders(t *testing.T) {
	got := captureStdout(t, func() {
		runLLMProviders()
	})
	if !strings.Contains(got, "Built-in providers") {
		t.Errorf("expected provider listing, got %q", got)
	}
}

func TestRunViewer_Help(t *testing.T) {
	got := captureStdout(t, func() {
		err := runViewer([]string{"-h"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if !strings.Contains(got, "Session history") {
		t.Errorf("expected usage text, got %q", got)
	}
}

func TestPrintReviewUsage(t *testing.T) {
	got := captureStdout(t, func() {
		printReviewUsage()
	})
	if !strings.Contains(got, "ocr review") {
		t.Errorf("expected usage text, got %q", got)
	}
}

func TestPrintTopLevelUsage(t *testing.T) {
	got := captureStdout(t, func() {
		printTopLevelUsage()
	})
	if !strings.Contains(got, "OpenCodeReview") {
		t.Errorf("expected usage text, got %q", got)
	}
}

func TestPrintViewerUsage(t *testing.T) {
	got := captureStdout(t, func() {
		printViewerUsage()
	})
	if !strings.Contains(got, "Session history") {
		t.Errorf("expected viewer usage text, got %q", got)
	}
}

func TestPrintRulesCheckUsage(t *testing.T) {
	got := captureStdout(t, func() {
		printRulesCheckUsage()
	})
	if !strings.Contains(got, "ocr rules check") {
		t.Errorf("expected usage text, got %q", got)
	}
}

func TestPrintScanUsage(t *testing.T) {
	got := captureStdout(t, func() {
		printScanUsage()
	})
	if !strings.Contains(got, "ocr scan") {
		t.Errorf("expected usage text, got %q", got)
	}
}
