// Package ui contains the Bubble Tea model for branch-clean.
package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"branch-clean/internal/git"
	"branch-clean/internal/plan"
)

type state int

const (
	stateLoading state = iota
	stateSelect
	stateDeleting
	stateFinished // deletion complete, results shown until the user exits
	stateDone     // quitting
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true)
	cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	warnStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

type item struct {
	plan.Candidate
	selected bool
}

type result struct {
	Branch string
	Err    error
}

type loadedMsg struct {
	candidates []plan.Candidate
	err        error
}

type stepMsg struct{ result result }

type Model struct {
	repo    git.Repo
	state   state
	items   []item
	cursor  int
	height  int
	width   int
	err     error
	queue   []plan.Candidate
	results []result
	offset  int
}

func New(repo git.Repo) Model {
	return Model{repo: repo, height: 24, width: 80}
}

func (m Model) Err() error { return m.err }

func (m Model) Init() tea.Cmd {
	return load(m.repo)
}

func load(repo git.Repo) tea.Cmd {
	return func() tea.Msg {
		cands, err := Collect(repo)
		return loadedMsg{candidates: cands, err: err}
	}
}

// Collect fetches from the remotes and computes the cleanup candidates.
func Collect(repo git.Repo) ([]plan.Candidate, error) {
	remotes, err := repo.Remotes()
	if err != nil {
		return nil, err
	}
	if len(remotes) == 0 {
		return nil, fmt.Errorf("no remotes configured; refusing to treat every branch as remote-less")
	}
	if err := repo.Fetch(); err != nil {
		return nil, err
	}
	if err := repo.PruneWorktrees(); err != nil {
		return nil, err
	}
	branches, err := repo.LocalBranches()
	if err != nil {
		return nil, err
	}
	remoteBranches, err := repo.RemoteBranches()
	if err != nil {
		return nil, err
	}
	worktrees, err := repo.Worktrees()
	if err != nil {
		return nil, err
	}
	top, err := repo.Toplevel()
	if err != nil {
		return nil, err
	}
	return plan.Build(plan.Input{
		Branches:       branches,
		RemoteBranches: remoteBranches,
		Remotes:        remotes,
		Worktrees:      worktrees,
		DefaultBranch:  repo.DefaultBranch(),
		CurrentPath:    top,
		IsDirty:        repo.IsDirty,
	}), nil
}

// Delete removes the worktree (if any) and then the branch for each candidate.
func Delete(repo git.Repo, cands []plan.Candidate) []result {
	results := make([]result, 0, len(cands))
	for _, c := range cands {
		results = append(results, deleteOne(repo, c))
	}
	return results
}

func deleteOne(repo git.Repo, c plan.Candidate) result {
	if c.Worktree != "" {
		if err := repo.RemoveWorktree(c.Worktree); err != nil {
			return result{c.Branch, err}
		}
	}
	return result{c.Branch, repo.DeleteBranch(c.Branch)}
}

func deleteStep(repo git.Repo, c plan.Candidate) tea.Cmd {
	return func() tea.Msg { return stepMsg{result: deleteOne(repo, c)} }
}

func (m Model) selectedCandidates() []plan.Candidate {
	var out []plan.Candidate
	for _, it := range m.items {
		if it.selected {
			out = append(out, it.Candidate)
		}
	}
	return out
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height, m.width = msg.Height, msg.Width
	case loadedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.state = stateDone
			return m, tea.Quit
		}
		for _, c := range msg.candidates {
			m.items = append(m.items, item{Candidate: c, selected: true})
		}
		if len(m.items) == 0 {
			m.state = stateDone
			return m, tea.Quit
		}
		m.state = stateSelect
	case stepMsg:
		m.results = append(m.results, msg.result)
		if len(m.results) < len(m.queue) {
			return m, deleteStep(m.repo, m.queue[len(m.results)])
		}
		m.state = stateFinished
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		return m, tea.Quit
	}
	switch m.state {
	case stateSelect:
		return m.handleSelectKey(key)
	case stateFinished:
		return m.handleFinishedKey(key)
	}
	return m, nil
}

func (m Model) handleFinishedKey(key string) (tea.Model, tea.Cmd) {
	page := m.visibleRows()
	switch key {
	case "q", "esc", "enter":
		m.state = stateDone
		return m, tea.Quit
	case "up", "k":
		m.offset--
	case "down", "j":
		m.offset++
	case "pgup":
		m.offset -= page
	case "pgdown", " ":
		m.offset += page
	case "home", "g":
		m.offset = 0
	case "end", "G":
		m.offset = len(m.resultLines())
	}
	m.offset = max(0, min(m.offset, len(m.resultLines())-page))
	return m, nil
}

func (m Model) handleSelectKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "n", "N", "q", "esc":
		m.state = stateDone
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case " ", "x":
		m.items[m.cursor].selected = !m.items[m.cursor].selected
	case "a":
		all := true
		for _, it := range m.items {
			all = all && it.selected
		}
		for i := range m.items {
			m.items[i].selected = !all
		}
	case "y", "Y":
		sel := m.selectedCandidates()
		if len(sel) == 0 {
			return m, nil
		}
		m.state = stateDeleting
		m.queue = sel
		return m, deleteStep(m.repo, sel[0])
	}
	return m, nil
}

func (m Model) View() string {
	var b strings.Builder
	switch m.state {
	case stateLoading:
		b.WriteString("Fetching remotes and scanning branches…\n")
	case stateSelect:
		m.viewSelect(&b)
	case stateDeleting, stateFinished:
		m.viewResults(&b)
	case stateDone:
		m.viewDone(&b)
	}
	return b.String()
}

func (m Model) viewSelect(b *strings.Builder) {
	b.WriteString(titleStyle.Render("Local branches with no remote equivalent") + "\n\n")

	visible := m.height - 7
	if visible < 3 {
		visible = 3
	}
	start := 0
	if m.cursor >= visible {
		start = m.cursor - visible + 1
	}
	end := min(start+visible, len(m.items))

	for i := start; i < end; i++ {
		it := m.items[i]
		cursor, box := "  ", "[ ]"
		if i == m.cursor {
			cursor = cursorStyle.Render("> ")
		}
		if it.selected {
			box = okStyle.Render("[x]")
		}
		line := fmt.Sprintf("%s%s %s", cursor, box, it.Branch)
		if it.Worktree != "" {
			line += dimStyle.Render("  worktree: " + it.Worktree)
		}
		if it.Dirty {
			line += warnStyle.Render("  ⚠ uncommitted changes (removal will fail)")
		}
		line += dimStyle.Render("  (" + it.Reason + ")")
		b.WriteString(line + "\n")
	}
	if len(m.items) > visible {
		b.WriteString(dimStyle.Render(fmt.Sprintf("  … %d-%d of %d", start+1, end, len(m.items))) + "\n")
	}
	fmt.Fprintf(b, "\n%d of %d selected\n", len(m.selectedCandidates()), len(m.items))
	b.WriteString(dimStyle.Render("↑/↓ move · space toggle · a all/none") + "\n")
	b.WriteString("Delete selected? " + okStyle.Render("y") + " yes / " + errStyle.Render("n") + " no\n")
}

func (m Model) failedCount() int {
	n := 0
	for _, r := range m.results {
		if r.Err != nil {
			n++
		}
	}
	return n
}

func (m Model) visibleRows() int {
	return max(m.height-6, 3)
}

// resultLines renders one line per result row. While deleting they are in
// deletion order; once finished, failures come first so they are not missed.
func (m Model) resultLines() []string {
	var ok, bad []string
	errWidth := max(m.width-6, 20)
	for _, r := range m.results {
		if r.Err == nil {
			ok = append(ok, okStyle.Render("✓ "+r.Branch))
			continue
		}
		msg := strings.TrimSpace(lipgloss.NewStyle().Width(errWidth).Render(r.Err.Error()))
		bad = append(bad, errStyle.Render("✗ "+r.Branch)+"\n    "+strings.ReplaceAll(msg, "\n", "\n    "))
	}
	if m.state == stateDeleting {
		var lines []string
		for _, r := range m.results {
			if r.Err == nil {
				lines = append(lines, okStyle.Render("✓ "+r.Branch))
			} else {
				lines = append(lines, errStyle.Render("✗ "+r.Branch)+dimStyle.Render("  failed, see details at the end"))
			}
		}
		return lines
	}
	var lines []string
	for _, l := range append(bad, ok...) {
		lines = append(lines, strings.Split(l, "\n")...)
	}
	return lines
}

func (m Model) viewResults(b *strings.Builder) {
	total, done, failed := len(m.queue), len(m.results), m.failedCount()
	if m.state == stateDeleting {
		fmt.Fprintf(b, "%s\n", titleStyle.Render(fmt.Sprintf("Deleting %d/%d", done, total)))
		fmt.Fprintf(b, "%s\n\n", dimStyle.Render(bar(done, total, 30)+fmt.Sprintf("  %d failed", failed)))
	} else {
		summary := fmt.Sprintf("Deleted %d, failed %d", done-failed, failed)
		fmt.Fprintf(b, "%s\n\n", titleStyle.Render(summary))
	}

	lines := m.resultLines()
	visible := m.visibleRows()
	start := m.offset
	if m.state == stateDeleting {
		start = max(0, len(lines)-visible)
	}
	end := min(start+visible, len(lines))
	for _, l := range lines[start:end] {
		b.WriteString(l + "\n")
	}
	if m.state == stateDeleting {
		if done < total {
			fmt.Fprintf(b, "\n%s\n", dimStyle.Render("Removing "+m.queue[done].Branch+"…"))
		}
		return
	}
	if len(lines) > visible {
		fmt.Fprintf(b, "%s\n", dimStyle.Render(fmt.Sprintf("  … lines %d-%d of %d", start+1, end, len(lines))))
	}
	b.WriteString("\n" + dimStyle.Render("↑/↓ scroll · q/enter exit") + "\n")
}

func bar(done, total, width int) string {
	if total == 0 {
		return ""
	}
	n := done * width / total
	return strings.Repeat("█", n) + strings.Repeat("░", width-n)
}

func (m Model) viewDone(b *strings.Builder) {
	switch {
	case m.err != nil:
	case len(m.results) > 0:
		fmt.Fprintf(b, "Deleted %d, failed %d.\n", len(m.results)-m.failedCount(), m.failedCount())
	case len(m.items) == 0:
		b.WriteString("Nothing to clean up.\n")
	default:
		b.WriteString("Cancelled, nothing deleted.\n")
	}
}
