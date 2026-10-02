package plan

import (
	"reflect"
	"testing"

	"branch-clean/internal/git"
)

func TestBuild(t *testing.T) {
	in := Input{
		Branches: []git.Branch{
			{Name: "main", HasUpstream: true},
			{Name: "current"},
			{Name: "pushed", HasUpstream: true},
			{Name: "same-name"},
			{Name: "gone", HasUpstream: true, UpstreamGone: true},
			{Name: "local-only"},
			{Name: "wt-branch"},
			{Name: "locked-branch"},
			{Name: "slash/feature"},
		},
		Remotes:        []string{"origin"},
		RemoteBranches: []string{"origin/main", "origin/same-name"},
		DefaultBranch:  "main",
		Worktrees: []git.Worktree{
			{Path: "/repo", Branch: "current", Main: true},
			{Path: "/wt/a", Branch: "wt-branch"},
			{Path: "/wt/b", Branch: "locked-branch", Locked: true},
		},
		IsDirty: func(p string) bool { return p == "/wt/a" },
	}
	want := []Candidate{
		{Branch: "gone", Reason: "upstream gone"},
		{Branch: "local-only", Reason: "no remote branch"},
		{Branch: "slash/feature", Reason: "no remote branch"},
		{Branch: "wt-branch", Worktree: "/wt/a", Dirty: true, Reason: "no remote branch"},
	}
	if got := Build(in); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}
