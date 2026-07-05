package cli

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/llin/cttw/internal/api"
	"github.com/llin/cttw/internal/config"
	"github.com/spf13/cobra"
)

func daemonCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "daemon"}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "start",
			Short: "Start the cttw daemon",
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := config.Load(os.Getenv("CTTW_CONFIG"), nil)
				if err != nil {
					return err
				}

				// If the daemon is already responding, report that instead of failing.
				client := api.NewClient(cfg.DaemonSocket)
				if err := client.Status(); err == nil {
					fmt.Printf("daemon already running (%s)\n", cfg.DaemonSocket)
					return nil
				}

				exe, _ := os.Executable()
				c := exec.Command(exe, "--daemon")
				c.Stdout = os.Stdout
				c.Stderr = os.Stderr
				c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
				if err := c.Start(); err != nil {
					return err
				}

				if err := waitForDaemon(client, 5*time.Second, 100*time.Millisecond); err != nil {
					return fmt.Errorf("daemon did not become ready: %w", err)
				}
				fmt.Printf("daemon started (pid %d) on %s\n", c.Process.Pid, cfg.DaemonSocket)
				return nil
			},
		},
		&cobra.Command{
			Use:   "stop",
			Short: "Stop the cttw daemon",
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := config.Load(os.Getenv("CTTW_CONFIG"), nil)
				if err != nil {
					return err
				}
				client := api.NewClient(cfg.DaemonSocket)
				if err := client.Shutdown(); err != nil {
					return err
				}
				fmt.Println("daemon stopped")
				return nil
			},
		},
		&cobra.Command{
			Use:   "status",
			Short: "Show daemon status",
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := config.Load(os.Getenv("CTTW_CONFIG"), nil)
				if err != nil {
					return err
				}
				client := api.NewClient(cfg.DaemonSocket)
				if err := client.Status(); err != nil {
					fmt.Println("daemon not running")
					return nil
				}
				fmt.Printf("daemon running (%s)\n", cfg.DaemonSocket)
				return nil
			},
		},
	)
	return cmd
}

func waitForDaemon(client *api.Client, timeout, interval time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := client.Status(); err == nil {
			return nil
		}
		time.Sleep(interval)
	}
	return fmt.Errorf("timed out after %s", timeout)
}
