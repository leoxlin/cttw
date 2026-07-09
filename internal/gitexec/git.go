package gitexec

import (
	"encoding/base64"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

type Runner struct {
	Dir   string
	Token string
}

// extraHeader returns the git -c argument that injects the Authorization header
// for https://github.com/ when a token is configured. It is used per-invocation
// so the token is never persisted in the cloned repository's local config.
func (r *Runner) extraHeader() string {
	auth := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + r.Token))
	header := fmt.Sprintf("Authorization: Basic %s", auth)
	return fmt.Sprintf("http.https://github.com/.extraHeader=%s", header)
}

func (r *Runner) run(args ...string) error {
	if r.Token != "" {
		args = append([]string{"-c", r.extraHeader()}, args...)
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Dir
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

// Run executes an arbitrary git command in the runner's directory.
func (r *Runner) Run(args ...string) error {
	return r.run(args...)
}

func (r *Runner) runOutput(args ...string) ([]byte, error) {
	if r.Token != "" {
		args = append([]string{"-c", r.extraHeader()}, args...)
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Dir
	return cmd.Output()
}

// Output runs a git command and returns its stdout.
func (r *Runner) Output(args ...string) ([]byte, error) {
	return r.runOutput(args...)
}

// Clone clones repo into dir using token for authentication. The token is not
// embedded in the clone URL or persisted in the repo config; instead it is sent
// via an Authorization header on each git invocation to avoid leaking it in git
// output, process listings, or on-disk configuration.
func (r *Runner) Clone(repo, token, dir string) error {
	r.Dir = dir
	r.Token = token
	args := []string{"clone", repo, dir}
	if token != "" {
		args = append([]string{"-c", r.extraHeader()}, args...)
	}
	cmd := exec.Command("git", args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
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

func (r *Runner) CurrentBranch() (string, error) {
	out, err := r.Output("symbolic-ref", "--short", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (r *Runner) Head() (string, error) {
	out, err := r.Output("rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (r *Runner) HasChanges() (bool, error) {
	out, err := r.Output("status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) != "", nil
}

func (r *Runner) AddAll() error {
	return r.run("add", "-A")
}

func (r *Runner) CommitAll(message string) (bool, error) {
	if err := r.AddAll(); err != nil {
		return false, err
	}
	if err := r.run("diff", "--cached", "--quiet"); err == nil {
		return false, nil
	}
	if err := r.run("-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false", "commit", "-m", message); err != nil {
		return false, fmt.Errorf("commit failed: %w", err)
	}
	return true, nil
}

func (r *Runner) ResetHardClean() error {
	if err := r.run("reset", "--hard", "HEAD"); err != nil {
		return err
	}
	return r.run("clean", "-fd")
}

func (r *Runner) PushSetUpstream(branch string) error {
	if err := r.run("push", "-u", "origin", branch); err != nil {
		return err
	}
	return nil
}

func (r *Runner) Commit(message string) error {
	return r.run("commit", "-m", message)
}

func (r *Runner) Fetch(ref string) error {
	return r.run("fetch", "origin", ref)
}

func (r *Runner) Pull(branch string) error {
	return r.run("pull", "origin", branch)
}

func (r *Runner) Push(branch string) error {
	return r.run("push", "origin", branch)
}

// StackRunner shells out to the gh stack CLI extension.
type StackRunner struct {
	Dir string
}

// StackInit initializes a stack with the given base and ordered branches.
func (s *StackRunner) StackInit(base string, branches []string) error {
	args := []string{"stack", "init", "--base", base}
	args = append(args, branches...)
	return s.run(args...)
}

// StackSubmit pushes branches and creates/updates stacked PRs.
// Auto generates titles; open creates ready-for-review PRs.
func (s *StackRunner) StackSubmit(auto, open bool) error {
	args := []string{"stack", "submit"}
	if auto {
		args = append(args, "--auto")
	}
	if open {
		args = append(args, "--open")
	}
	return s.run(args...)
}

func (s *StackRunner) run(args ...string) error {
	cmd := exec.Command("gh", args...)
	cmd.Dir = s.Dir
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}
