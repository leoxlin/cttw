package cli

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/llin/cttw/internal/config"
	"github.com/llin/cttw/internal/tui"
	"github.com/spf13/cobra"
)

func Run() error {
	root := &cobra.Command{
		Use:   "cttw",
		Short: "Claudivicus Take The Wheel",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(os.Getenv("CTTW_CONFIG"), nil)
			if err != nil {
				return err
			}
			p := tea.NewProgram(tui.New(cfg.DaemonSocket))
			_, err = p.Run()
			return err
		},
	}
	root.AddCommand(daemonCmd(), taskCmd(), tasksCmd(), tuiCmd())
	return root.Execute()
}

func tuiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Start the interactive TUI",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(os.Getenv("CTTW_CONFIG"), nil)
			if err != nil {
				return err
			}
			p := tea.NewProgram(tui.New(cfg.DaemonSocket))
			_, err = p.Run()
			return err
		},
	}
}
