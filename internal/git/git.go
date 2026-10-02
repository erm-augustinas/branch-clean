// Package git is a thin wrapper around the git CLI.
package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

type Repo struct {
	Dir string
}

type Branch struct {
	Name         string
	UpstreamGone bool
	HasUpstream  bool
}

type Worktree struct {
	Path     string
	Branch   string // empty when detached or bare
	Locked   bool
	Prunable bool
	Main     bool
}

func (r Repo) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}

func (r Repo) IsRepo() bool {
	out, err := r.run("rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

// Toplevel returns the root of the worktree containing r.Dir.
func (r Repo) Toplevel() (string, error) {
	out, err := r.run("rev-parse", "--show-toplevel")
	return strings.TrimSpace(out), err
}

func (r Repo) Fetch() error {
	_, err := r.run("fetch", "--prune", "--all")
	return err
}

func (r Repo) PruneWorktrees() error {
	_, err := r.run("worktree", "prune")
	return err
}

func (r Repo) Remotes() ([]string, error) {
	out, err := r.run("remote")
	if err != nil {
		return nil, err
	}
	return splitLines(out), nil
}

func (r Repo) LocalBranches() ([]Branch, error) {
	out, err := r.run("for-each-ref", "--format=%(refname)%00%(upstream)%00%(upstream:track)", "refs/heads")
	if err != nil {
		return nil, err
	}
	var branches []Branch
	for _, line := range splitLines(out) {
		parts := strings.Split(line, "\x00")
		if len(parts) != 3 {
			continue
		}
		branches = append(branches, Branch{
			Name:         strings.TrimPrefix(parts[0], "refs/heads/"),
			HasUpstream:  parts[1] != "",
			UpstreamGone: strings.Contains(parts[2], "gone"),
		})
	}
	return branches, nil
}

// RemoteBranches returns remote-tracking branch names as "<remote>/<branch>".
func (r Repo) RemoteBranches() ([]string, error) {
	out, err := r.run("for-each-ref", "--format=%(refname)", "refs/remotes")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, ref := range splitLines(out) {
		name := strings.TrimPrefix(ref, "refs/remotes/")
		if strings.HasSuffix(name, "/HEAD") {
			continue
		}
		names = append(names, name)
	}
	return names, nil
}

// DefaultBranch returns the remote's default branch, or "" if unknown.
func (r Repo) DefaultBranch() string {
	out, err := r.run("symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.TrimSpace(out), "origin/")
}

func (r Repo) Worktrees() ([]Worktree, error) {
	out, err := r.run("worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var wts []Worktree
	var cur *Worktree
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			wts = append(wts, Worktree{Path: strings.TrimPrefix(line, "worktree "), Main: len(wts) == 0})
			cur = &wts[len(wts)-1]
		case cur == nil:
		case strings.HasPrefix(line, "branch "):
			cur.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		case line == "locked" || strings.HasPrefix(line, "locked "):
			cur.Locked = true
		case line == "prunable" || strings.HasPrefix(line, "prunable "):
			cur.Prunable = true
		}
	}
	return wts, nil
}

func (r Repo) IsDirty(path string) bool {
	out, err := Repo{Dir: path}.run("status", "--porcelain")
	return err == nil && strings.TrimSpace(out) != ""
}

func (r Repo) RemoveWorktree(path string) error {
	_, err := r.run("worktree", "remove", path)
	return err
}

func (r Repo) DeleteBranch(name string) error {
	_, err := r.run("branch", "-D", "--", name)
	return err
}

func splitLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}
