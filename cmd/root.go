package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/jinzhongjia/ttools/internal/ai"
	"github.com/jinzhongjia/ttools/internal/auth"
	"github.com/jinzhongjia/ttools/internal/config"
	gitx "github.com/jinzhongjia/ttools/internal/git"
	"github.com/jinzhongjia/ttools/internal/provider"
)

type GitService interface {
	Open(path string) (*gitx.Repository, error)
	HasStagedChanges(repo *gitx.Repository) (bool, error)
	GetStagedDiffs(repo *gitx.Repository) ([]gitx.FileDiff, error)
	GetWorktreeChanges(repo *gitx.Repository) ([]gitx.WorktreeChange, error)
	StageFiles(repo *gitx.Repository, paths []string) error
	Commit(repo *gitx.Repository, msg string) (string, error)
}

type AIService interface {
	GenerateCommitMessage(ctx context.Context, diffs []gitx.FileDiff) (string, error)
}

type Deps struct {
	Git           GitService
	AI            AIService
	StageSelector StageSelector
}

type defaultGitService struct{}

func (defaultGitService) Open(path string) (*gitx.Repository, error) {
	return gitx.OpenRepository(path)
}
func (defaultGitService) HasStagedChanges(repo *gitx.Repository) (bool, error) {
	return gitx.HasStagedChanges(repo)
}
func (defaultGitService) GetStagedDiffs(repo *gitx.Repository) ([]gitx.FileDiff, error) {
	return gitx.GetStagedDiffs(repo)
}
func (defaultGitService) GetWorktreeChanges(repo *gitx.Repository) ([]gitx.WorktreeChange, error) {
	return gitx.GetWorktreeChanges(repo)
}
func (defaultGitService) StageFiles(repo *gitx.Repository, paths []string) error {
	return gitx.StageFiles(repo, paths)
}
func (defaultGitService) Commit(repo *gitx.Repository, msg string) (string, error) {
	h, err := gitx.Commit(repo, msg)
	return h.String(), err
}

type twoStageAIService struct{ client ai.Client }

func (s twoStageAIService) GenerateCommitMessage(ctx context.Context, diffs []gitx.FileDiff) (string, error) {
	return ai.GenerateTwoStage(ctx, s.client, diffs)
}

func NewRootCommand(deps Deps) *cobra.Command {
	var cfgOpts config.Options
	root := &cobra.Command{
		Use:           "ttools",
		Short:         "Developer tools",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&cfgOpts.ConfigFile, "config", "", "config file")
	root.PersistentFlags().StringVar(&cfgOpts.Provider, "provider", "", "LLM provider")
	root.PersistentFlags().StringVar(&cfgOpts.Model, "model", "", "LLM model")
	root.PersistentFlags().StringVar(&cfgOpts.APIKey, "api-key", "", "LLM API key or Copilot OAuth token")
	root.PersistentFlags().StringVar(&cfgOpts.BaseURL, "base-url", "", "OpenAI-compatible base URL")

	root.AddCommand(newCommitCommand(deps, &cfgOpts))
	return root
}

func newCommitCommand(deps Deps, cfgOpts *config.Options) *cobra.Command {
	var dryRun bool
	var yes bool
	cmd := &cobra.Command{
		Use:     "commit",
		Aliases: []string{"ac"},
		Short:   "Generate an AI commit message for staged changes and commit",
		RunE: func(cmd *cobra.Command, args []string) error {
			gitSvc := deps.Git
			if gitSvc == nil {
				gitSvc = defaultGitService{}
			}
			aiSvc := deps.AI
			if aiSvc == nil {
				cfg, err := config.Load(*cfgOpts)
				if err != nil {
					return err
				}
				resolver := auth.NewCopilotResolver(auth.ResolverOptions{})
				client, err := provider.NewFactory(resolver).New(cfg.LLM)
				if err != nil {
					return err
				}
				aiSvc = twoStageAIService{client: client}
			}

			repo, err := gitSvc.Open(".")
			if err != nil {
				return fmt.Errorf("open git repository: %w", err)
			}
			has, err := gitSvc.HasStagedChanges(repo)
			if err != nil {
				return err
			}
			if !has {
				if err := promptAndStageFiles(cmd, deps, gitSvc, repo); err != nil {
					return err
				}
				has, err = gitSvc.HasStagedChanges(repo)
				if err != nil {
					return err
				}
				if !has {
					return nil
				}
			}
			diffs, err := gitSvc.GetStagedDiffs(repo)
			if err != nil {
				return err
			}
			msg, err := generateWithIndicator(cmd.OutOrStdout(), func() (string, error) {
				return aiSvc.GenerateCommitMessage(cmd.Context(), diffs)
			})
			if err != nil {
				return err
			}
			if msg == "" {
				return errors.New("AI returned an empty commit message")
			}
			out := cmd.OutOrStdout()
			if err := printSuggestedCommitMessage(out, msg); err != nil {
				return err
			}
			if dryRun {
				return nil
			}
			if !yes && !confirm(cmd.InOrStdin(), out) {
				if _, err := fmt.Fprintln(out, "Commit cancelled."); err != nil {
					return err
				}
				return nil
			}
			hash, err := gitSvc.Commit(repo, msg)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "Committed successfully: %s\n", hash); err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "generate message without committing")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "commit without confirmation")
	return cmd
}

func promptAndStageFiles(cmd *cobra.Command, deps Deps, gitSvc GitService, repo *gitx.Repository) error {
	changes, err := gitSvc.GetWorktreeChanges(repo)
	if err != nil {
		return err
	}
	if len(changes) == 0 {
		return errors.New("no changes found")
	}
	selector := deps.StageSelector
	if selector == nil {
		selector = NewBubbleStageSelector(cmd.InOrStdin(), cmd.OutOrStdout())
	}
	paths, err := selector.Select(changes)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "No files selected; commit cancelled.")
		return err
	}
	if err := gitSvc.StageFiles(repo, paths); err != nil {
		return err
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Staged %d %s.\n", len(paths), pluralize("file", len(paths)))
	return err
}

func pluralize(word string, count int) string {
	if count == 1 {
		return word
	}
	return word + "s"
}

func generateWithIndicator(out io.Writer, generate func() (string, error)) (string, error) {
	if !isTerminalWriter(out) {
		return generate()
	}

	done := make(chan struct{})
	go func() {
		frames := []string{"Generating.  ", "Generating.. ", "Generating..."}
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for i := 0; ; i++ {
			_, _ = fmt.Fprintf(out, "\r%s", frames[i%len(frames)])
			select {
			case <-done:
				return
			case <-ticker.C:
			}
		}
	}()

	msg, err := generate()
	close(done)
	_, _ = fmt.Fprint(out, "\r\033[K")
	return msg, err
}

func printSuggestedCommitMessage(out io.Writer, msg string) error {
	if !isTerminalWriter(out) {
		_, err := fmt.Fprintf(out, "Suggested commit message:\n\n%s\n", msg)
		return err
	}
	return typewriterPrint(out, "Suggested commit message:\n\n"+msg+"\n", 10*time.Millisecond)
}

func typewriterPrint(out io.Writer, text string, delay time.Duration) error {
	for _, r := range text {
		if _, err := fmt.Fprint(out, string(r)); err != nil {
			return err
		}
		time.Sleep(delay)
	}
	return nil
}

func isTerminalWriter(w io.Writer) bool {
	file, ok := w.(interface{ Fd() uintptr })
	return ok && term.IsTerminal(int(file.Fd()))
}

func confirm(in io.Reader, out io.Writer) bool {
	_, _ = fmt.Fprint(out, "\nCommit with this message? [Y/n] ")
	var answer string
	_, _ = fmt.Fscanln(in, &answer)
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "" || answer == "y" || answer == "yes"
}

func Execute() error {
	cmd := NewRootCommand(Deps{})
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)
	cmd.SetIn(os.Stdin)
	return cmd.Execute()
}
