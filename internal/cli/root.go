package cli

import "github.com/spf13/cobra"

func Run() error {
	root := &cobra.Command{
		Use:   "cttw",
		Short: "Claudivicus Take The Wheel",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTUI()
		},
	}
	root.AddCommand(daemonCmd(), problemCmd(), problemsCmd(), tuiCmd())
	return root.Execute()
}
