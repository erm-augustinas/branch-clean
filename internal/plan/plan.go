// Package plan decides which local branches and worktrees should be removed.
package plan

import (
	"sort"
	"strings"

	"branch-clean/internal/git"
)

type Candidate struct {
	Branch   string
	Worktree string // path, empty when the branch has no worktree
	Dirty    bool
	Reason   string
}

type Input struct {
	Branches       []git.Branch
	RemoteBranches []string // "<remote>/<branch>"
	Remotes        []string
	Worktrees      []git.Worktree
	DefaultBranch  string
	CurrentPath    string // worktree the tool is run from; never removed
	IsDirty        func(path string) bool
}

func Build(in Input) []Candidate {
	remoteHas := map[string]bool{}
	for _, rb := range in.RemoteBranches {
		for _, remote := range in.Remotes {
			if name, ok := strings.CutPrefix(rb, remote+"/"); ok {
				remoteHas[name] = true
			}
		}
	}

	protected := map[string]bool{"main": in.DefaultBranch == "", "master": in.DefaultBranch == ""}
	if in.DefaultBranch != "" {
		protected[in.DefaultBranch] = true
	}
	wtByBranch := map[string]git.Worktree{}
	for _, wt := range in.Worktrees {
		if wt.Branch == "" {
			continue
		}
		if wt.Main || (in.CurrentPath != "" && wt.Path == in.CurrentPath) {
			protected[wt.Branch] = true
		}
		wtByBranch[wt.Branch] = wt
	}

	var out []Candidate
	for _, b := range in.Branches {
		if protected[b.Name] {
			continue
		}
		var reason string
		switch {
		case b.UpstreamGone:
			reason = "upstream gone"
		case b.HasUpstream, remoteHas[b.Name]:
			continue
		default:
			reason = "no remote branch"
		}
		c := Candidate{Branch: b.Name, Reason: reason}
		if wt, ok := wtByBranch[b.Name]; ok {
			if wt.Locked {
				continue
			}
			c.Worktree = wt.Path
			if in.IsDirty != nil && !wt.Prunable {
				c.Dirty = in.IsDirty(wt.Path)
			}
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Branch < out[j].Branch })
	return out
}
