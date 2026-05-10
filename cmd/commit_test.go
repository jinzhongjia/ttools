package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	gitx "github.com/jinzhongjia/ttools/internal/git"
)

type fakeGitService struct {
	diffs   []gitx.FileDiff
	changes []gitx.WorktreeChange
	staged  []string

	committed string
}

func (f *fakeGitService) Open(path string) (*gitx.Repository, error) { return &gitx.Repository{}, nil }
func (f *fakeGitService) HasStagedChanges(repo *gitx.Repository) (bool, error) {
	return len(f.diffs) > 0, nil
}
func (f *fakeGitService) GetStagedDiffs(repo *gitx.Repository) ([]gitx.FileDiff, error) {
	return f.diffs, nil
}
func (f *fakeGitService) GetWorktreeChanges(repo *gitx.Repository) ([]gitx.WorktreeChange, error) {
	return f.changes, nil
}
func (f *fakeGitService) StageFiles(repo *gitx.Repository, paths []string) error {
	f.staged = append([]string(nil), paths...)
	for _, path := range paths {
		f.diffs = append(f.diffs, gitx.FileDiff{Path: path, Status: gitx.StatusModified, Patch: "+x"})
	}
	return nil
}
func (f *fakeGitService) Commit(repo *gitx.Repository, msg string) (string, error) {
	f.committed = msg
	return "abc123", nil
}

type fakeCommitAI struct{}

func (fakeCommitAI) GenerateCommitMessage(ctx context.Context, diffs []gitx.FileDiff) (string, error) {
	return "feat: test commit", nil
}

type fakeStageSelector struct {
	paths []string
}

func (s fakeStageSelector) Select(changes []gitx.WorktreeChange) ([]string, error) {
	return s.paths, nil
}

func TestCommitDryRunDoesNotCommit(t *testing.T) {
	gitSvc := &fakeGitService{diffs: []gitx.FileDiff{{Path: "a.go", Patch: "+x"}}}
	root := NewRootCommand(Deps{Git: gitSvc, AI: fakeCommitAI{}})
	root.SetArgs([]string{"commit", "--dry-run"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if gitSvc.committed != "" {
		t.Fatal("unexpected commit")
	}
	if !strings.Contains(out.String(), "feat: test commit") {
		t.Fatalf("output = %s", out.String())
	}
}

func TestCommitYesCommits(t *testing.T) {
	gitSvc := &fakeGitService{diffs: []gitx.FileDiff{{Path: "a.go", Patch: "+x"}}}
	root := NewRootCommand(Deps{Git: gitSvc, AI: fakeCommitAI{}})
	root.SetArgs([]string{"commit", "--yes"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if gitSvc.committed != "feat: test commit" {
		t.Fatalf("committed = %q", gitSvc.committed)
	}
}

func TestRootCommandExposesConfigFlags(t *testing.T) {
	root := NewRootCommand(Deps{Git: &fakeGitService{diffs: []gitx.FileDiff{{Path: "a.go"}}}, AI: fakeCommitAI{}})
	for _, name := range []string{"config", "provider", "model", "api-key", "base-url"} {
		if root.PersistentFlags().Lookup(name) == nil {
			t.Fatalf("missing flag %s", name)
		}
	}
}

func TestCommitPromptsToStageWhenNoStagedChanges(t *testing.T) {
	gitSvc := &fakeGitService{changes: []gitx.WorktreeChange{{Path: "cmd/root.go", Status: gitx.StatusModified}}}
	root := NewRootCommand(Deps{Git: gitSvc, AI: fakeCommitAI{}, StageSelector: fakeStageSelector{paths: []string{"cmd/root.go"}}})
	root.SetArgs([]string{"commit", "--yes"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Join(gitSvc.staged, ",") != "cmd/root.go" {
		t.Fatalf("staged = %+v", gitSvc.staged)
	}
	if gitSvc.committed != "feat: test commit" {
		t.Fatalf("committed = %q", gitSvc.committed)
	}
	if !strings.Contains(out.String(), "Staged 1 file") {
		t.Fatalf("output = %s", out.String())
	}
}

func TestCommitCancelsWhenStageSelectionIsEmpty(t *testing.T) {
	gitSvc := &fakeGitService{changes: []gitx.WorktreeChange{{Path: "cmd/root.go", Status: gitx.StatusModified}}}
	root := NewRootCommand(Deps{Git: gitSvc, AI: fakeCommitAI{}, StageSelector: fakeStageSelector{}})
	root.SetArgs([]string{"commit", "--yes"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if gitSvc.committed != "" {
		t.Fatal("unexpected commit")
	}
	if !strings.Contains(out.String(), "No files selected") {
		t.Fatalf("output = %s", out.String())
	}
}

func TestGenerateWithIndicatorSkipsSpinnerForNonTerminalWriter(t *testing.T) {
	var out bytes.Buffer
	msg, err := generateWithIndicator(&out, func() (string, error) {
		return "feat: test", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if msg != "feat: test" {
		t.Fatalf("msg = %q", msg)
	}
	if out.String() != "" {
		t.Fatalf("expected no spinner output for non-terminal writer, got %q", out.String())
	}
}

func TestPrintSuggestedCommitMessageUsesPlainOutputForNonTerminalWriter(t *testing.T) {
	var out bytes.Buffer
	if err := printSuggestedCommitMessage(&out, "feat: test"); err != nil {
		t.Fatal(err)
	}
	want := "Suggested commit message:\n\nfeat: test\n"
	if out.String() != want {
		t.Fatalf("output = %q, want %q", out.String(), want)
	}
}

func TestTypewriterPrintWritesAllText(t *testing.T) {
	var out bytes.Buffer
	if err := typewriterPrint(&out, "hello", 0); err != nil {
		t.Fatal(err)
	}
	if out.String() != "hello" {
		t.Fatalf("output = %q", out.String())
	}
}
