package repo

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/llin/cttw/internal/gitexec"
)

type Registry struct {
	Root string
}

type Repo struct {
	Owner         string
	Name          string
	Dir           string
	DefaultBranch string
}

func (r *Registry) Dir(owner, name string) string {
	return filepath.Join(r.Root, owner, name)
}

func (r *Registry) Ensure(ctx context.Context, owner, name, defaultBranch, token string) (*Repo, error) {
	if defaultBranch == "" {
		defaultBranch = "main"
	}
	dir := r.Dir(owner, name)
	gitDir := filepath.Join(dir, ".git")

	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
			return nil, fmt.Errorf("create repo parent dir: %w", err)
		}
		cloneURL := fmt.Sprintf("https://github.com/%s/%s.git", owner, name)
		if err := (&gitexec.Runner{}).Clone(cloneURL, token, dir); err != nil {
			return nil, fmt.Errorf("clone repo: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("stat repo dir: %w", err)
	}

	// Verify the existing directory is actually the requested repository.
	remote, err := (&gitexec.Runner{Dir: dir}).Output("config", "--get", "remote.origin.url")
	if err != nil {
		return nil, fmt.Errorf("read remote origin url: %w", err)
	}
	if !remoteMatches(string(remote), owner, name) {
		return nil, fmt.Errorf("repo at %s does not match %s/%s", dir, owner, name)
	}

	// Keep remote refs up to date; do not touch the working tree.
	if err := (&gitexec.Runner{Dir: dir}).Run("remote", "update"); err != nil {
		return nil, fmt.Errorf("remote update: %w", err)
	}

	return &Repo{
		Owner:         owner,
		Name:          name,
		Dir:           dir,
		DefaultBranch: defaultBranch,
	}, nil
}

// remoteMatches reports whether a git remote URL points to the given
// github.com owner/name repository. It accepts HTTPS and SSH forms, with or
// without a trailing .git.
func remoteMatches(remote, owner, name string) bool {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return false
	}
	// Normalize SSH form to HTTPS-like path for matching.
	remote = strings.TrimPrefix(remote, "git@github.com:")
	remote = strings.TrimPrefix(remote, "https://github.com/")
	remote = strings.TrimPrefix(remote, "http://github.com/")
	remote = strings.TrimSuffix(remote, ".git")
	want := owner + "/" + name
	matched, _ := regexp.MatchString(`^`+regexp.QuoteMeta(want)+`(?:/.*)?$`, remote)
	return matched
}
