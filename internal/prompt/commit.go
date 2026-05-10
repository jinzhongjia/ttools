package prompt

import (
	"fmt"
	"strings"

	gitx "ttools/internal/git"
)

type FileSummary struct {
	Path    string
	Status  gitx.Status
	Summary string
}

type CommitPromptInput struct {
	TotalFiles    int
	Additions     int
	Deletions     int
	FileSummaries []FileSummary
}

func BuildFileSummaryPrompt(fd gitx.FileDiff) string {
	return fmt.Sprintf(`Summarize this staged file change in 1-2 concise sentences.
Do not generate a commit message.

File: %s
Status: %s
Stats: +%d -%d
Flags: binary=%t generated=%t lockfile=%t test=%t docs=%t config=%t
Patch:
%s`, fd.Path, fd.Status, fd.Additions, fd.Deletions, fd.Binary, fd.Generated, fd.Lockfile, fd.TestFile, fd.DocFile, fd.ConfigFile, fd.Patch)
}

func BuildCommitMessagePrompt(input CommitPromptInput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Generate a git commit message using Conventional Commits.\n")
	fmt.Fprintf(&b, "Keep the subject under 72 characters and use imperative mood.\n\n")
	fmt.Fprintf(&b, "Summary: %d files, +%d -%d\n\n", input.TotalFiles, input.Additions, input.Deletions)
	fmt.Fprintf(&b, "File summaries:\n")
	for _, fs := range input.FileSummaries {
		fmt.Fprintf(&b, "- %s: %s\n", fs.Path, fs.Summary)
	}
	return b.String()
}
