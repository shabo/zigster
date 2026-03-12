package app

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/luki/sensors/internal/monitor"
	"github.com/luki/sensors/internal/stress"
	"github.com/luki/sensors/internal/viewer"
)

// Run dispatches CLI arguments to the monitor, history viewer, or stress runner.
func Run(args []string) int {
	switch {
	case len(args) > 0 && args[0] == "--history":
		viewer.Run()
		return 0

	case len(args) > 0 && args[0] == "stress":
		stress.Run(args[1:])
		return 0

	default:
		p := tea.NewProgram(
			monitor.New(),
			tea.WithAltScreen(),
			tea.WithMouseCellMotion(),
		)
		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		return 0
	}
}
