// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// ocr-stub is a cross-platform stand-in for the OCR CLI used by ACP tests.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type config struct {
	VersionText   string   `json:"version_text"`
	VersionPrefix string   `json:"version_prefix"`
	VersionSuffix string   `json:"version_suffix"`
	VersionBytes  int      `json:"version_bytes"`
	SleepMS       int      `json:"sleep_ms"`
	ExitCode      int      `json:"exit_code"`
	HoldPIDFile   string   `json:"hold_pid_file"`
	HoldInherit   bool     `json:"hold_inherit"`
	HoldMS        int      `json:"hold_ms"`
	FlushMS       int      `json:"flush_ms"`
	FlushStdout   string   `json:"flush_stdout"`
	FlushStderr   string   `json:"flush_stderr"`
	MarkerFile    string   `json:"marker_file"`
	StderrLines   []string `json:"stderr_lines"`
	Stdout        string   `json:"stdout"`
	Block         bool     `json:"block"`
	CleanupFile   string   `json:"cleanup_file"`
	HugeMessage   int      `json:"huge_message"`
}

func main() {
	if os.Getenv("OCR_STUB_CHILD") == "1" {
		runChild()
		return
	}
	cfg := loadConfig()
	if versionRequested() {
		runVersion(cfg)
		return
	}
	if cfg.MarkerFile != "" {
		if err := os.WriteFile(cfg.MarkerFile, []byte("started"), 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "ocr-stub: marker: %v\n", err)
			os.Exit(1)
		}
	}
	command, mode := commandAndMode()
	if command != "review" && command != "scan" && mode == "" && !cfg.hasCommandOverride() {
		fmt.Fprintf(os.Stderr, "ocr-stub: expected review or scan, got %q\n", strings.Join(os.Args[1:], " "))
		os.Exit(2)
	}
	exit := runCommand(command, mode, cfg)
	os.Exit(exit)
}

func (c config) hasCommandOverride() bool {
	return len(c.StderrLines) > 0 || c.Stdout != "" || c.Block || c.HoldPIDFile != "" || c.FlushMS > 0 || c.CleanupFile != "" || c.HugeMessage > 0
}

func loadConfig() config {
	self, err := os.Executable()
	if err != nil {
		return config{}
	}
	data, err := os.ReadFile(self + ".json")
	if err != nil {
		return config{}
	}
	var cfg config
	if err := json.Unmarshal(data, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "ocr-stub: config: %v\n", err)
		os.Exit(1)
	}
	return cfg
}

func versionRequested() bool {
	for _, arg := range os.Args[1:] {
		if arg == "--version" {
			return true
		}
	}
	return false
}

func commandAndMode() (string, string) {
	if len(os.Args) < 2 {
		return "", ""
	}
	command := os.Args[1]
	if len(os.Args) < 3 || strings.HasPrefix(os.Args[2], "-") {
		return command, ""
	}
	return command, os.Args[2]
}

func runVersion(cfg config) {
	if cfg.SleepMS > 0 {
		time.Sleep(time.Duration(cfg.SleepMS) * time.Millisecond)
	}
	if cfg.VersionBytes > 0 {
		_, _ = os.Stdout.Write([]byte(strings.Repeat("x", cfg.VersionBytes)))
		return
	}
	if cfg.HoldPIDFile != "" {
		if _, err := spawnChild(cfg.HoldInherit, holdDuration(cfg), "", ""); err != nil {
			fmt.Fprintf(os.Stderr, "ocr-stub: spawn: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(cfg.HoldPIDFile, []byte(strconv.Itoa(lastChildPID)+"\n"), 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "ocr-stub: pid file: %v\n", err)
			os.Exit(1)
		}
	}
	if cfg.FlushMS > 0 {
		fmt.Print(cfg.VersionPrefix)
		if _, err := spawnChild(true, time.Duration(cfg.FlushMS)*time.Millisecond, cfg.VersionSuffix, ""); err != nil {
			fmt.Fprintf(os.Stderr, "ocr-stub: flush spawn: %v\n", err)
			os.Exit(1)
		}
		os.Exit(cfg.ExitCode)
		return
	}
	text := cfg.VersionText
	if text == "" {
		text = "ocr-stub 0.0.0"
	}
	fmt.Println(text)
	os.Exit(cfg.ExitCode)
}

func runCommand(command, mode string, cfg config) int {
	for _, line := range cfg.StderrLines {
		fmt.Fprintln(os.Stderr, line)
	}
	if cfg.HugeMessage > 0 {
		fmt.Printf("{\"status\":\"success\",\"comments\":[],\"message\":\"%s\"}\n", strings.Repeat("x", cfg.HugeMessage))
		return cfg.ExitCode
	}
	switch mode {
	case "nonzero":
		fmt.Println(`{"status":"complete","comments":[]}`)
		return 7
	case "invalid":
		fmt.Println("{invalid")
		return 0
	case "multiple":
		fmt.Println(`{"status":"complete","comments":[]}`)
		fmt.Println(`{"status":"complete","comments":[]}`)
		return 0
	case "block":
		return runBlock(cfg, false)
	case "cleanup":
		return runBlock(cfg, true)
	case "large":
		for i := 0; i < 20; i++ {
			fmt.Fprint(os.Stderr, "xxxxxxxxxx")
		}
		fmt.Fprintln(os.Stderr)
		for i := 0; i < 30; i++ {
			fmt.Fprint(os.Stdout, "xxxxxxxxxx")
		}
		fmt.Fprintln(os.Stdout)
		return 0
	case "huge":
		fmt.Printf("{\"status\":\"success\",\"comments\":[],\"message\":\"%s\"}\n", strings.Repeat("x", 1<<20))
		return 0
	}
	if cfg.HoldPIDFile != "" {
		if _, err := spawnChild(cfg.HoldInherit, holdDuration(cfg), "", ""); err != nil {
			fmt.Fprintf(os.Stderr, "ocr-stub: spawn: %v\n", err)
			return 1
		}
		if err := os.WriteFile(cfg.HoldPIDFile, []byte(strconv.Itoa(lastChildPID)+"\n"), 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "ocr-stub: pid file: %v\n", err)
			return 1
		}
	}
	if cfg.FlushMS > 0 {
		if cfg.Stdout != "" {
			fmt.Print(cfg.Stdout)
		}
		if _, err := spawnChild(true, time.Duration(cfg.FlushMS)*time.Millisecond, cfg.FlushStdout, cfg.FlushStderr); err != nil {
			fmt.Fprintf(os.Stderr, "ocr-stub: flush spawn: %v\n", err)
			return 1
		}
		if cfg.ExitCode != 0 {
			return cfg.ExitCode
		}
		return 0
	}
	if cfg.Block {
		return runBlock(cfg, cfg.CleanupFile != "")
	}
	if cfg.Stdout != "" {
		fmt.Print(cfg.Stdout)
		if !strings.HasSuffix(cfg.Stdout, "\n") {
			fmt.Println()
		}
		return cfg.ExitCode
	}
	if cfg.SleepMS > 0 {
		time.Sleep(time.Duration(cfg.SleepMS) * time.Millisecond)
	}
	if command == "scan" {
		fmt.Println(`{"status":"success","comments":[]}`)
	} else {
		fmt.Fprintln(os.Stderr, "[ocr] running")
		fmt.Println(`{"status":"complete","comments":[],"manifest":{"terminal_state":"complete"}}`)
	}
	return cfg.ExitCode
}

func runBlock(cfg config, cleanup bool) int {
	fmt.Fprintln(os.Stderr, "READY")
	waitForInterrupt()
	if cleanup || cfg.CleanupFile != "" {
		path := cfg.CleanupFile
		if path == "" {
			path = "cleaned"
		}
		_ = os.WriteFile(path, []byte("cleaned\n"), 0o600)
		return 0
	}
	fmt.Println(`{"status":"failed","comments":[]}`)
	return 1
}

func holdDuration(cfg config) time.Duration {
	if cfg.HoldMS > 0 {
		return time.Duration(cfg.HoldMS) * time.Millisecond
	}
	return 30 * time.Second
}

var lastChildPID int

func spawnChild(inherit bool, delay time.Duration, stdout, stderr string) (int, error) {
	self, err := os.Executable()
	if err != nil {
		return 0, err
	}
	cmd := exec.Command(self, "--stub-child")
	cmd.Env = append(os.Environ(),
		"OCR_STUB_CHILD=1",
		"OCR_STUB_CHILD_SLEEP_MS="+strconv.Itoa(int(delay/time.Millisecond)),
		"OCR_STUB_CHILD_STDOUT="+stdout,
		"OCR_STUB_CHILD_STDERR="+stderr,
	)
	if inherit {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else {
		null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
		if err != nil {
			return 0, err
		}
		defer null.Close()
		cmd.Stdin = null
		cmd.Stdout = null
		cmd.Stderr = null
	}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	lastChildPID = cmd.Process.Pid
	return lastChildPID, nil
}

func runChild() {
	ms, _ := strconv.Atoi(os.Getenv("OCR_STUB_CHILD_SLEEP_MS"))
	if ms > 0 {
		time.Sleep(time.Duration(ms) * time.Millisecond)
	}
	if text := os.Getenv("OCR_STUB_CHILD_STDOUT"); text != "" {
		fmt.Print(text)
	}
	if text := os.Getenv("OCR_STUB_CHILD_STDERR"); text != "" {
		fmt.Fprint(os.Stderr, text)
	}
}
