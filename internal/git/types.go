package git

import gogit "github.com/go-git/go-git/v5"

type Status string

const (
	StatusAdded    Status = "added"
	StatusModified Status = "modified"
	StatusDeleted  Status = "deleted"
	StatusRenamed  Status = "renamed"
)

type FileDiff struct {
	Path       string
	OldPath    string
	Status     Status
	Additions  int
	Deletions  int
	Patch      string
	Binary     bool
	Generated  bool
	Lockfile   bool
	TestFile   bool
	DocFile    bool
	ConfigFile bool
}

type WorktreeChange struct {
	Path      string
	Status    Status
	Untracked bool
}

type Repository struct {
	Git  *gogit.Repository
	Path string
}
