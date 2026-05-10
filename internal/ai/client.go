package ai

import (
	"context"
	"fmt"
	"strings"

	gitx "ttools/internal/git"
)

const MaxFilePatchBytes = 12 * 1024

type Client interface {
	SummarizeFileDiff(ctx context.Context, fd gitx.FileDiff) (string, error)
	GenerateCommitMessage(ctx context.Context, input FinalInput) (string, error)
}

type FinalInput struct {
	TotalFiles int
	Additions  int
	Deletions  int
	Summaries  []FileSummary
}

type FileSummary struct {
	Path    string
	Status  gitx.Status
	Summary string
}

func GenerateTwoStage(ctx context.Context, client Client, diffs []gitx.FileDiff) (string, error) {
	input := FinalInput{TotalFiles: len(diffs)}
	for _, fd := range diffs {
		prepared := prepareFileDiff(fd)
		summary, err := client.SummarizeFileDiff(ctx, prepared)
		if err != nil {
			return "", err
		}
		input.Additions += fd.Additions
		input.Deletions += fd.Deletions
		input.Summaries = append(input.Summaries, FileSummary{Path: fd.Path, Status: fd.Status, Summary: summary})
	}
	msg, err := client.GenerateCommitMessage(ctx, input)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(msg), nil
}

func prepareFileDiff(fd gitx.FileDiff) gitx.FileDiff {
	if fd.Binary || fd.Lockfile || fd.Generated {
		fd.Patch = ""
		return fd
	}
	if len(fd.Patch) > MaxFilePatchBytes {
		fd.Patch = fd.Patch[:MaxFilePatchBytes] + fmt.Sprintf("\n\n[patch truncated: original %d bytes]", len(fd.Patch))
	}
	return fd
}
