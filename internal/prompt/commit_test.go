package prompt

import (
	"strings"
	"testing"

	gitx "github.com/jinzhongjia/ttools/internal/git"
)

func TestBuildFileSummaryPrompt(t *testing.T) {
	fd := gitx.FileDiff{Path: "internal/git/diff.go", Status: gitx.StatusModified, Additions: 3, Deletions: 1, Patch: "@@\n+new line"}
	out := BuildFileSummaryPrompt(fd)

	for _, want := range []string{"Summarize this staged file change", "internal/git/diff.go", "modified", "+3 -1", "+new line"} {
		if !strings.Contains(out, want) {
			t.Fatalf("prompt missing %q:\n%s", want, out)
		}
	}
}

func TestBuildCommitMessagePrompt(t *testing.T) {
	input := CommitPromptInput{
		TotalFiles:    2,
		Additions:     10,
		Deletions:     2,
		FileSummaries: []FileSummary{{Path: "cmd/commit.go", Summary: "Adds commit command."}},
	}
	out := BuildCommitMessagePrompt(input)

	for _, want := range []string{"Conventional Commits", "2 files", "+10 -2", "cmd/commit.go", "Adds commit command"} {
		if !strings.Contains(out, want) {
			t.Fatalf("prompt missing %q:\n%s", want, out)
		}
	}
}
