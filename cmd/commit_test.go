package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	gitx "ttools/internal/git"
)

type fakeGitService struct {
	diffs     []gitx.FileDiff
	committed string
}

func (f *fakeGitService) Open(path string) (*gitx.Repository, error) { return &gitx.Repository{}, nil }
func (f *fakeGitService) HasStagedChanges(repo *gitx.Repository) (bool, error) {
	return len(f.diffs) > 0, nil
}
func (f *fakeGitService) GetStagedDiffs(repo *gitx.Repository) ([]gitx.FileDiff, error) {
	return f.diffs, nil
}
func (f *fakeGitService) Commit(repo *gitx.Repository, msg string) (string, error) {
	f.committed = msg
	return "abc123", nil
}

type fakeCommitAI struct{}

func (fakeCommitAI) GenerateCommitMessage(ctx context.Context, diffs []gitx.FileDiff) (string, error) {
	return "feat: test commit", nil
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
