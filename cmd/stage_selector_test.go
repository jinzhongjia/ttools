package cmd

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	gitx "github.com/jinzhongjia/ttools/internal/git"
)

func TestStageSelectorModelTogglesAndSelects(t *testing.T) {
	model := newStageSelectorModel([]gitx.WorktreeChange{
		{Path: "cmd/root.go", Status: gitx.StatusModified},
		{Path: "README.md", Status: gitx.StatusAdded, Untracked: true},
	})

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(stageSelectorModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(stageSelectorModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(stageSelectorModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(stageSelectorModel)

	if strings.Join(model.selectedPaths(), ",") != "cmd/root.go,README.md" {
		t.Fatalf("selected = %+v", model.selectedPaths())
	}
	if !model.done {
		t.Fatal("expected done")
	}
}
