package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/llin/cttw/internal/api"
	"github.com/llin/cttw/internal/config"
	"github.com/spf13/cobra"
)

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func problemCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "problem <owner>/<repo> <description>",
		Short: "Create a new problem",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoSpec := args[0]
			desc := strings.Join(args[1:], " ")
			parts := strings.Split(repoSpec, "/")
			if len(parts) != 2 {
				return fmt.Errorf("repo must be owner/name")
			}
			owner, name := parts[0], parts[1]

			cfg, err := config.Load(os.Getenv("CTTW_CONFIG"), nil)
			if err != nil {
				return err
			}
			client := api.NewClient(cfg.DaemonSocket)
			pr, err := client.CreateProblem(owner, name, desc)
			if err != nil {
				return err
			}
			fmt.Printf("problem %s created (%s)\n", pr.ID, pr.Status)
			return nil
		},
	}
}

func problemsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "problems",
		Short: "List problems",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(os.Getenv("CTTW_CONFIG"), nil)
			if err != nil {
				return err
			}
			client := api.NewClient(cfg.DaemonSocket)
			problems, err := client.ListProblems()
			if err != nil {
				return err
			}
			for _, p := range problems {
				fmt.Printf("%s  %-12s  %s\n", shortID(p.ID), p.Status, p.Description)
			}
			return nil
		},
	}
}
