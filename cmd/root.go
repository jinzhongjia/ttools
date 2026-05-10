package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"ttools/internal/ai"
	"ttools/internal/auth"
	"ttools/internal/config"
	gitx "ttools/internal/git"
	"ttools/internal/provider"
)

type GitService interface {
	Open(path string) (*gitx.Repository, error)
	HasStagedChanges(repo *gitx.Repository) (bool, error)
	GetStagedDiffs(repo *gitx.Repository) ([]gitx.FileDiff, error)
	Commit(repo *gitx.Repository, msg string) (string, error)
}

type AIService interface {
	GenerateCommitMessage(ctx context.Context, diffs []gitx.FileDiff) (string, error)
}

type Deps struct {
	Git GitService
	AI  AIService
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
				return errors.New("no staged changes found; run `git add <file>` first")
			}
			diffs, err := gitSvc.GetStagedDiffs(repo)
			if err != nil {
				return err
			}
			msg, err := aiSvc.GenerateCommitMessage(cmd.Context(), diffs)
			if err != nil {
				return err
			}
			if msg == "" {
				return errors.New("AI returned an empty commit message")
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Suggested commit message:\n\n%s\n", msg)
			if dryRun {
				return nil
			}
			if !yes && !confirm(cmd.InOrStdin(), out) {
				fmt.Fprintln(out, "Commit cancelled.")
				return nil
			}
			hash, err := gitSvc.Commit(repo, msg)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "Committed successfully: %s\n", hash)
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "generate message without committing")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "commit without confirmation")
	return cmd
}

func confirm(in io.Reader, out io.Writer) bool {
	fmt.Fprint(out, "\nCommit with this message? [Y/n] ")
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
