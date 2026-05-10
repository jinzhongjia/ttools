package ai

import (
	"context"
	"strings"
	"testing"

	gitx "ttools/internal/git"
)

type fakeClient struct {
	fileCalls int
	finalIn   FinalInput
	seen      []gitx.FileDiff
}

func (f *fakeClient) SummarizeFileDiff(ctx context.Context, fd gitx.FileDiff) (string, error) {
	f.fileCalls++
	f.seen = append(f.seen, fd)
	return "summary for " + fd.Path, nil
}

func (f *fakeClient) GenerateCommitMessage(ctx context.Context, input FinalInput) (string, error) {
	f.finalIn = input
	return "feat: add ai commit command", nil
}

func TestGenerateCommitMessageTwoStage(t *testing.T) {
	c := &fakeClient{}
	msg, err := GenerateTwoStage(context.Background(), c, []gitx.FileDiff{{Path: "a.go", Additions: 1}, {Path: "b.go", Additions: 2, Deletions: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if msg != "feat: add ai commit command" {
		t.Fatalf("message = %q", msg)
	}
	if c.fileCalls != 2 {
		t.Fatalf("file calls = %d", c.fileCalls)
	}
	if c.finalIn.TotalFiles != 2 || c.finalIn.Additions != 3 || c.finalIn.Deletions != 1 {
		t.Fatalf("bad final input: %+v", c.finalIn)
	}
}

func TestGenerateTwoStageTruncatesLargePatchAndSkipsLowSignalFiles(t *testing.T) {
	c := &fakeClient{}
	largePatch := strings.Repeat("a", MaxFilePatchBytes+100)
	_, err := GenerateTwoStage(context.Background(), c, []gitx.FileDiff{
		{Path: "main.go", Patch: largePatch, Additions: 1},
		{Path: "go.sum", Patch: "large lock diff", Lockfile: true},
		{Path: "asset.bin", Patch: "binary", Binary: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.fileCalls != 3 {
		t.Fatalf("file calls = %d", c.fileCalls)
	}
	if len(c.seen[0].Patch) > MaxFilePatchBytes+200 {
		t.Fatalf("patch was not truncated: %d", len(c.seen[0].Patch))
	}
	if c.seen[1].Patch != "" || c.seen[2].Patch != "" {
		t.Fatalf("expected low signal patches to be omitted: %+v", c.seen)
	}
}
