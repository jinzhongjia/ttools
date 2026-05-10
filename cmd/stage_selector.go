package cmd

import (
	"fmt"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	gitx "github.com/jinzhongjia/ttools/internal/git"
)

type StageSelector interface {
	Select(changes []gitx.WorktreeChange) ([]string, error)
}

type BubbleStageSelector struct {
	in  io.Reader
	out io.Writer
}

func NewBubbleStageSelector(in io.Reader, out io.Writer) BubbleStageSelector {
	return BubbleStageSelector{in: in, out: out}
}

func (s BubbleStageSelector) Select(changes []gitx.WorktreeChange) ([]string, error) {
	model := newStageSelectorModel(changes)
	program := tea.NewProgram(model, tea.WithInput(s.in), tea.WithOutput(s.out))
	result, err := program.Run()
	if err != nil {
		return nil, err
	}
	finalModel, ok := result.(stageSelectorModel)
	if !ok || finalModel.cancelled {
		return nil, nil
	}
	return finalModel.selectedPaths(), nil
}

type stageSelectorModel struct {
	changes   []gitx.WorktreeChange
	cursor    int
	selected  map[int]bool
	done      bool
	cancelled bool
}

func newStageSelectorModel(changes []gitx.WorktreeChange) stageSelectorModel {
	return stageSelectorModel{changes: changes, selected: map[int]bool{}}
}

func (m stageSelectorModel) Init() tea.Cmd { return nil }

func (m stageSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.cancelled = true
			m.done = true
			return m, tea.Quit
		case tea.KeyEnter:
			m.done = true
			return m, tea.Quit
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyDown:
			if m.cursor < len(m.changes)-1 {
				m.cursor++
			}
		case tea.KeySpace:
			m.selected[m.cursor] = !m.selected[m.cursor]
		}
	}
	return m, nil
}

func (m stageSelectorModel) View() string {
	if m.done {
		return ""
	}
	var b strings.Builder
	b.WriteString("No staged changes found.\n\n")
	b.WriteString("Select files to stage:\n")
	for i, change := range m.changes {
		cursor := " "
		if i == m.cursor {
			cursor = ">"
		}
		checked := " "
		if m.selected[i] {
			checked = "x"
		}
		untracked := ""
		if change.Untracked {
			untracked = " untracked"
		}
		_, _ = fmt.Fprintf(&b, "%s [%s] %s (%s%s)\n", cursor, checked, change.Path, change.Status, untracked)
	}
	b.WriteString("\nSpace to select, Enter to confirm, Esc to cancel\n")
	return b.String()
}

func (m stageSelectorModel) selectedPaths() []string {
	paths := make([]string, 0, len(m.selected))
	for i, change := range m.changes {
		if m.selected[i] {
			paths = append(paths, change.Path)
		}
	}
	return paths
}
