package cli

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/llin/cttw/internal/config"
	"github.com/llin/cttw/internal/tui"
	"github.com/spf13/cobra"
)

func tuiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Launch interactive TUI",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadClient(os.Getenv("CTTW_CONFIG"), nil)
			if err != nil {
				return err
			}
			p := tea.NewProgram(tui.New(cfg.DaemonSocket))
			_, err = p.Run()
			return err
		},
	}
}
