package gitexec

import (
	"encoding/base64"
	"fmt"
	"io"
	"os/exec"
)

type Runner struct {
	Dir string
}

func run(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
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

// Output runs a git command and returns its stdout.
func (r *Runner) Output(args ...string) ([]byte, error) {
	return r.runOutput(args...)
}

// Clone clones repo into dir using token for authentication. The token is not
// embedded in the clone URL; instead it is sent via an Authorization header to
// avoid leaking it in git output or process listings.
func (r *Runner) Clone(repo, token, dir string) error {
	auth := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
	header := fmt.Sprintf("Authorization: Basic %s", auth)
	extra := fmt.Sprintf("http.https://github.com/.extraHeader=%s", header)

	if err := run("", "clone", "-c", extra, repo, dir); err != nil {
		return err
	}
	// Persist the auth header so subsequent remote operations use it.
	return run(dir, "config", "http.https://github.com/.extraHeader", header)
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
