package git

import (
	"errors"
	"io"
	"path/filepath"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	formatindex "github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/pmezard/go-difflib/difflib"
)

func OpenRepository(path string) (*Repository, error) {
	repo, err := gogit.PlainOpenWithOptions(path, &gogit.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return nil, err
	}
	return &Repository{Git: repo, Path: path}, nil
}

func IsGitRepo(path string) bool {
	_, err := OpenRepository(path)
	return err == nil
}

func HasStagedChanges(repo *Repository) (bool, error) {
	wt, err := repo.Git.Worktree()
	if err != nil {
		return false, err
	}
	status, err := wt.Status()
	if err != nil {
		return false, err
	}
	for _, s := range status {
		if isStagedStatus(s.Staging) {
			return true, nil
		}
	}
	return false, nil
}

func GetStagedDiffs(repo *Repository) ([]FileDiff, error) {
	wt, err := repo.Git.Worktree()
	if err != nil {
		return nil, err
	}
	status, err := wt.Status()
	if err != nil {
		return nil, err
	}

	idx, err := repo.Git.Storer.Index()
	if err != nil {
		return nil, err
	}
	headTree, _ := headTree(repo)

	var diffs []FileDiff
	for path, st := range status {
		if !isStagedStatus(st.Staging) {
			continue
		}
		fd := FileDiff{Path: path, Status: mapStatus(st.Staging)}
		fd.TestFile = isTestFile(path)
		fd.DocFile = isDocFile(path)
		fd.ConfigFile = isConfigFile(path)
		fd.Lockfile = isLockfile(path)

		oldContent := ""
		if headTree != nil && fd.Status != StatusAdded {
			oldContent, _ = treeFileContent(headTree, path)
		}
		newContent := ""
		if fd.Status != StatusDeleted {
			newContent, _ = indexFileContent(repo, idx, path)
		}
		fd.Binary = isBinaryContent(oldContent) || isBinaryContent(newContent)
		fd.Generated = isGeneratedContent(oldContent) || isGeneratedContent(newContent)
		if fd.Binary {
			diffs = append(diffs, fd)
			continue
		}
		fd.Patch, fd.Additions, fd.Deletions = unifiedPatch(path, oldContent, newContent)
		diffs = append(diffs, fd)
	}
	return diffs, nil
}

func isStagedStatus(code gogit.StatusCode) bool {
	return code != gogit.Unmodified && code != gogit.Untracked
}

func GetWorktreeChanges(repo *Repository) ([]WorktreeChange, error) {
	wt, err := repo.Git.Worktree()
	if err != nil {
		return nil, err
	}
	status, err := wt.Status()
	if err != nil {
		return nil, err
	}

	changes := make([]WorktreeChange, 0, len(status))
	for path, st := range status {
		if isStagedStatus(st.Staging) {
			continue
		}
		if st.Worktree == gogit.Unmodified && st.Staging != gogit.Untracked {
			continue
		}
		code := st.Worktree
		untracked := st.Staging == gogit.Untracked || st.Worktree == gogit.Untracked
		if untracked {
			code = gogit.Added
		}
		changes = append(changes, WorktreeChange{Path: path, Status: mapStatus(code), Untracked: untracked})
	}
	return changes, nil
}

func StageFiles(repo *Repository, paths []string) error {
	wt, err := repo.Git.Worktree()
	if err != nil {
		return err
	}
	status, err := wt.Status()
	if err != nil {
		return err
	}
	for _, path := range paths {
		fileStatus := status.File(path)
		if fileStatus.Worktree == gogit.Deleted {
			if _, err := wt.Remove(path); err != nil {
				return err
			}
			continue
		}
		if _, err := wt.Add(path); err != nil {
			return err
		}
	}
	return nil
}

func Commit(repo *Repository, message string) (plumbing.Hash, error) {
	wt, err := repo.Git.Worktree()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	cfg, _ := repo.Git.Config()
	author := &object.Signature{Name: "ttools", Email: "ttools@example.invalid", When: time.Now()}
	if cfg != nil && cfg.User.Name != "" && cfg.User.Email != "" {
		author.Name = cfg.User.Name
		author.Email = cfg.User.Email
	}
	return wt.Commit(message, &gogit.CommitOptions{Author: author})
}

func headTree(repo *Repository) (*object.Tree, error) {
	head, err := repo.Git.Head()
	if err != nil {
		return nil, err
	}
	commit, err := repo.Git.CommitObject(head.Hash())
	if err != nil {
		return nil, err
	}
	return commit.Tree()
}

func treeFileContent(tree *object.Tree, path string) (string, error) {
	file, err := tree.File(path)
	if err != nil {
		return "", err
	}
	return file.Contents()
}

func indexFileContent(repo *Repository, idx *formatindex.Index, path string) (string, error) {
	for _, entry := range idx.Entries {
		if entry.Name != path {
			continue
		}
		blob, err := repo.Git.BlobObject(entry.Hash)
		if err != nil {
			return "", err
		}
		reader, err := blob.Reader()
		if err != nil {
			return "", err
		}
		defer func() { _ = reader.Close() }()
		buf := new(strings.Builder)
		if _, err := io.Copy(buf, reader); err != nil {
			return "", err
		}
		return buf.String(), nil
	}
	return "", errors.New("index entry not found")
}

func mapStatus(code gogit.StatusCode) Status {
	switch code {
	case gogit.Added:
		return StatusAdded
	case gogit.Deleted:
		return StatusDeleted
	case gogit.Renamed:
		return StatusRenamed
	default:
		return StatusModified
	}
}

func unifiedPatch(path, oldContent, newContent string) (string, int, int) {
	diff := difflib.UnifiedDiff{
		A:        difflib.SplitLines(oldContent),
		B:        difflib.SplitLines(newContent),
		FromFile: "a/" + path,
		ToFile:   "b/" + path,
		Context:  3,
	}
	patch, _ := difflib.GetUnifiedDiffString(diff)
	var additions, deletions int
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") {
			continue
		}
		if strings.HasPrefix(line, "+") {
			additions++
		}
		if strings.HasPrefix(line, "-") {
			deletions++
		}
	}
	return patch, additions, deletions
}

func isTestFile(path string) bool {
	return strings.HasSuffix(path, "_test.go") || strings.Contains(path, "/test/") || strings.Contains(path, "/tests/")
}

func isDocFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".mdx" || strings.HasPrefix(strings.ToLower(path), "docs/")
}

func isConfigFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".toml", ".yaml", ".yml", ".json":
		return true
	default:
		return false
	}
}

func isLockfile(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return strings.HasSuffix(base, ".lock") || base == "go.sum" || base == "package-lock.json" || base == "yarn.lock" || base == "pnpm-lock.yaml"
}

func isBinaryContent(content string) bool {
	return strings.ContainsRune(content, '\x00')
}

func isGeneratedContent(content string) bool {
	head := content
	if len(head) > 4096 {
		head = head[:4096]
	}
	head = strings.ToLower(head)
	return strings.Contains(head, "code generated") || strings.Contains(head, "do not edit") || strings.Contains(head, "generated by")
}
