package gitexec

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Runner struct {
	Dir string
}

func run(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (r *Runner) run(args ...string) error {
	return run(r.Dir, args...)
}

// Run executes an arbitrary git command in the runner's directory.
func (r *Runner) Run(args ...string) error {
	return r.run(args...)
}

func (r *Runner) runOutput(args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Dir
	return cmd.Output()
}

func (r *Runner) Clone(repo, token, dir string) error {
	url := strings.Replace(repo, "https://", fmt.Sprintf("https://x-access-token:%s@", token), 1)
	return run("", "clone", url, dir)
}

func (r *Runner) Checkout(branch string) error {
	return r.run("checkout", branch)
}

func (r *Runner) CheckoutNew(branch, base string) error {
	return r.run("checkout", "-b", branch, base)
}

func (r *Runner) Add(files ...string) error {
	args := append([]string{"add"}, files...)
	return r.run(args...)
}

func (r *Runner) Commit(message string) error {
	return r.run("commit", "-m", message)
}

func (r *Runner) Push(branch string) error {
	return r.run("push", "origin", branch)
}
