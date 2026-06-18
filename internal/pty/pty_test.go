package pty

import (
	"os"
	"testing"
)

func TestExecPipeBasic(t *testing.T) {
	// Temporarily replace stdin to ensure IsTerminal returns false
	orig := os.Stdin
	os.Stdin, _ = os.Open("/dev/null")
	defer func() { os.Stdin = orig }()

	rc := ExecPipe("/bin/bash", "echo hello")
	if rc != 0 {
		t.Errorf("expected exit code 0, got %d", rc)
	}
}

func TestExecPipeTrue(t *testing.T) {
	orig := os.Stdin
	os.Stdin, _ = os.Open("/dev/null")
	defer func() { os.Stdin = orig }()

	rc := ExecPipe("/bin/bash", "true")
	if rc != 0 {
		t.Errorf("expected exit code 0 for true, got %d", rc)
	}
}

func TestExecPipeFalse(t *testing.T) {
	orig := os.Stdin
	os.Stdin, _ = os.Open("/dev/null")
	defer func() { os.Stdin = orig }()

	rc := ExecPipe("/bin/bash", "false")
	if rc != 1 {
		t.Errorf("expected exit code 1 for false, got %d", rc)
	}
}

func TestExecPipeWithOutput(t *testing.T) {
	orig := os.Stdin
	os.Stdin, _ = os.Open("/dev/null")
	defer func() { os.Stdin = orig }()

	rc := ExecPipe("/bin/bash", "echo -n test123")
	if rc != 0 {
		t.Errorf("expected exit code 0, got %d", rc)
	}
}
