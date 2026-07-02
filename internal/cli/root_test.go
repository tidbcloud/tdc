package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/Icemap/tdc/internal/apperr"
	"github.com/Icemap/tdc/internal/version"
	"github.com/spf13/cobra"
)

func TestHelpCommands(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "root help",
			args: []string{"help"},
			want: "Commands:",
		},
		{
			name: "db help",
			args: []string{"db", "help"},
			want: "create-db-cluster",
		},
		{
			name: "fs help",
			args: []string{"fs", "help"},
			want: "mount-file-system",
		},
		{
			name: "subcommand help",
			args: []string{"fs", "mount-file-system", "help"},
			want: "Mount a tdc fs resource locally.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, _, err := executeForTest(tt.args...)
			if err != nil {
				t.Fatalf("expected help to succeed, got %v", err)
			}
			if !strings.Contains(stdout, tt.want) {
				t.Fatalf("expected output to contain %q, got:\n%s", tt.want, stdout)
			}
		})
	}
}

func TestVersionWorksAtEveryLevel(t *testing.T) {
	tests := [][]string{
		{"--version"},
		{"db", "--version"},
		{"fs", "mount-file-system", "--version"},
	}

	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, _, err := executeForTest(args...)
			if err != nil {
				t.Fatalf("expected version to succeed, got %v", err)
			}
			if got := strings.TrimSpace(stdout); got != testVersion().String() {
				t.Fatalf("expected %q, got %q", testVersion().String(), got)
			}
		})
	}
}

func TestUnknownCommandReturnsActionableError(t *testing.T) {
	_, _, err := executeForTest("db", "missing-command")
	if err == nil {
		t.Fatal("expected unknown command to fail")
	}
	if apperr.ExitCodeFor(err) != 2 {
		t.Fatalf("expected exit code 2, got %d", apperr.ExitCodeFor(err))
	}
	if got := apperr.MessageFor(err); !strings.Contains(got, "unknown command") {
		t.Fatalf("expected unknown command message, got %q", got)
	}
}

func TestContextCanceledIsRenderedAsInterrupted(t *testing.T) {
	err := normalizeError(context.Canceled)
	if err == nil {
		t.Fatal("expected interrupted error")
	}
	if got := apperr.ExitCodeFor(err); got != 130 {
		t.Fatalf("expected exit code 130, got %d", got)
	}
	if got := apperr.MessageFor(err); got != "interrupted" {
		t.Fatalf("expected interrupted message, got %q", got)
	}
}

func TestShortHelpFlagIsRejected(t *testing.T) {
	stdout, _, err := executeForTest("-h")
	if err == nil {
		t.Fatal("expected -h to fail")
	}
	if stdout != "" {
		t.Fatalf("expected no stdout, got %q", stdout)
	}
	if apperr.ExitCodeFor(err) != 2 {
		t.Fatalf("expected exit code 2, got %d", apperr.ExitCodeFor(err))
	}
}

func TestHelpOutputDoesNotExposeShortFlags(t *testing.T) {
	stdout, _, err := executeForTest("--help")
	if err != nil {
		t.Fatalf("expected --help to succeed, got %v", err)
	}
	if strings.Contains(stdout, "-h,") || strings.Contains(stdout, "-v,") {
		t.Fatalf("help output exposes a short flag:\n%s", stdout)
	}
}

func TestNoCommandDefinesShortFlags(t *testing.T) {
	root := NewRootCommand(testVersion())
	visitCommands(root, func(cmd *cobra.Command) {
		cmd.InitDefaultHelpFlag()
		cmd.InitDefaultVersionFlag()
	})
	if HasShorthand(root) {
		t.Fatal("expected command tree to avoid short flags")
	}
}

func TestPlaceholderCommandReturnsNotImplemented(t *testing.T) {
	_, _, err := executeForTest("organization", "list-projects")
	if err == nil {
		t.Fatal("expected placeholder command to fail")
	}
	if got := apperr.MessageFor(err); got != "tdc organization list-projects is not implemented yet" {
		t.Fatalf("unexpected message: %q", got)
	}
}

func executeForTest(args ...string) (string, string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root := NewRootCommand(testVersion())
	err := Execute(context.Background(), root, args, &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}

func testVersion() version.Info {
	return version.Info{
		Version: "0.0.0-test",
		Commit:  "testcommit",
		Date:    "2026-07-02",
		OS:      "testos",
		Arch:    "testarch",
	}
}
