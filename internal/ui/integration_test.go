package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"branch-clean/internal/git"
)

func sh(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func setup(t *testing.T) (repo git.Repo, root string) {
	t.Helper()
	root, _ = filepath.EvalSymlinks(t.TempDir())
	bare, work := filepath.Join(root, "remote.git"), filepath.Join(root, "work")
	sh(t, root, "init", "--bare", "-b", "main", bare)
	sh(t, root, "clone", bare, work)
	sh(t, work, "commit", "--allow-empty", "-m", "init")
	sh(t, work, "push", "origin", "main")
	// pushed branch (kept), pushed-then-deleted-remotely (gone), local only, local only with clean/dirty worktrees
	sh(t, work, "branch", "pushed")
	sh(t, work, "push", "-u", "origin", "pushed")
	sh(t, work, "branch", "gone")
	sh(t, work, "push", "-u", "origin", "gone")
	sh(t, root, "--git-dir", bare, "branch", "-D", "gone")
	sh(t, work, "branch", "local-only")
	sh(t, work, "worktree", "add", "-b", "wt-clean", filepath.Join(root, "wt-clean"))
	sh(t, work, "worktree", "add", "-b", "wt-dirty", filepath.Join(root, "wt-dirty"))
	if err := os.WriteFile(filepath.Join(root, "wt-dirty", "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	sh(t, work, "checkout", "-b", "current-local")
	return git.Repo{Dir: work}, root
}

func branchNames(t *testing.T, repo git.Repo) []string {
	bs, err := repo.LocalBranches()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, b := range bs {
		names = append(names, b.Name)
	}
	sort.Strings(names)
	return names
}

func TestCollectAndDelete(t *testing.T) {
	repo, root := setup(t)
	cands, err := Collect(repo)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range cands {
		got = append(got, c.Branch)
	}
	want := []string{"gone", "local-only", "wt-clean", "wt-dirty"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates %v, want %v", got, want)
	}
	if !cands[3].Dirty || cands[2].Dirty {
		t.Fatalf("dirty flags wrong: %+v", cands)
	}

	results := Delete(repo, cands)
	failed := map[string]bool{}
	for _, r := range results {
		if r.Err != nil {
			failed[r.Branch] = true
		}
	}
	if !reflect.DeepEqual(failed, map[string]bool{"wt-dirty": true}) {
		t.Fatalf("unexpected failures: %+v", results)
	}
	wantLeft := []string{"current-local", "main", "pushed", "wt-dirty"}
	if got := branchNames(t, repo); !reflect.DeepEqual(got, wantLeft) {
		t.Fatalf("remaining %v, want %v", got, wantLeft)
	}
	if _, err := os.Stat(filepath.Join(root, "wt-clean")); !os.IsNotExist(err) {
		t.Fatal("wt-clean worktree should be gone")
	}
	if _, err := os.Stat(filepath.Join(root, "wt-dirty")); err != nil {
		t.Fatal("wt-dirty worktree should remain")
	}
}

func TestKeyHandling(t *testing.T) {
	repo, _ := setup(t)
	cands, err := Collect(repo)
	if err != nil {
		t.Fatal(err)
	}
	var m tea.Model = New(repo)
	m, _ = m.Update(loadedMsg{candidates: cands})
	key := func(s string) { m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}) }
	key("j")
	key(" ") // unselect local-only
	if n := len(m.(Model).selectedCandidates()); n != len(cands)-1 {
		t.Fatalf("selected %d", n)
	}
	key("n")
	if m.(Model).state != stateDone || len(m.(Model).results) != 0 {
		t.Fatal("n should cancel without deleting")
	}
	if got := branchNames(t, repo); len(got) != 7 {
		t.Fatalf("branches deleted on decline: %v", got)
	}
}

func TestProgressThenExit(t *testing.T) {
	repo, _ := setup(t)
	cands, err := Collect(repo)
	if err != nil {
		t.Fatal(err)
	}
	var m tea.Model = New(repo)
	m, _ = m.Update(loadedMsg{candidates: cands})
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if m.(Model).state != stateDeleting {
		t.Fatal("expected deleting state")
	}
	for steps := 0; cmd != nil && m.(Model).state == stateDeleting; steps++ {
		m, cmd = m.Update(cmd())
		if steps > len(cands) {
			t.Fatal("too many steps")
		}
	}
	fm := m.(Model)
	if fm.state != stateFinished || len(fm.results) != len(cands) {
		t.Fatalf("state=%v results=%d", fm.state, len(fm.results))
	}
	if v := fm.View(); !strings.Contains(v, "✗ wt-dirty") || !strings.Contains(v, "contains modified or untracked files") {
		t.Fatalf("failure details missing:\n%s", v)
	}
	// stays open until the user exits
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil || m.(Model).state != stateFinished {
		t.Fatal("scrolling must not exit")
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("q should quit")
	}
}
