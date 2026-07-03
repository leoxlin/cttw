package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/llin/cttw/internal/api"
	"github.com/llin/cttw/internal/config"
	"github.com/spf13/cobra"
)

func taskCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "task <description>",
		Short: "Create a new task",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			desc := strings.Join(args, " ")
			cfg, err := config.Load(os.Getenv("CTTW_CONFIG"), nil)
			if err != nil {
				return err
			}
			client := api.NewClient(cfg.DaemonSocket)
			tr, err := client.CreateTask(desc)
			if err != nil {
				return err
			}
			fmt.Printf("task %s created\n", tr.ID)
			return nil
		},
	}
}

func tasksCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tasks",
		Short: "List tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(os.Getenv("CTTW_CONFIG"), nil)
			if err != nil {
				return err
			}
			client := api.NewClient(cfg.DaemonSocket)
			tasks, err := client.ListTasks()
			if err != nil {
				return err
			}
			for _, t := range tasks {
				fmt.Printf("%s  %-12s  %s\n", t.ID[:8], t.Status, t.Description)
			}
			return nil
		},
	}
}
