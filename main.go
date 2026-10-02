package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"branch-clean/internal/git"
	"branch-clean/internal/ui"
)

func main() {
	repo := git.Repo{}
	if !repo.IsRepo() {
		fmt.Fprintln(os.Stderr, "not inside a git repository")
		os.Exit(1)
	}
	final, err := tea.NewProgram(ui.New(repo)).Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := final.(ui.Model).Err(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
