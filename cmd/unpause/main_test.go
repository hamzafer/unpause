package main

import (
	"os/exec"
	"strings"
	"testing"
)

// buildBinary compiles the real CLI once per test run; main.go wires cobra flags
// directly onto closures, so exercising it as a subprocess is simpler than unit-testing internals.
func buildBinary(t *testing.T) string {
	t.Helper()
	bin := t.TempDir() + "/unpause"
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	return bin
}

func TestOpenHelpDocumentsJSONFlag(t *testing.T) {
	bin := buildBinary(t)
	out, err := exec.Command(bin, "open", "--help").CombinedOutput()
	if err != nil {
		t.Fatalf("--help failed: %v\n%s", err, out)
	}
	for _, want := range []string{"--print", "--json", "--fork", "--open"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("open --help missing %q\n%s", want, out)
		}
	}
}

func TestOpenUnknownSessionErrorsCleanly(t *testing.T) {
	bin := buildBinary(t)
	out, err := exec.Command(bin, "open", "definitely-not-a-real-session-id-xyz", "--print", "--json").CombinedOutput()
	if err == nil {
		t.Fatalf("expected a non-zero exit for an unmatched id, got success: %s", out)
	}
	if !strings.Contains(string(out), "no session matches") {
		t.Errorf("expected a clear error, got: %s", out)
	}
}
