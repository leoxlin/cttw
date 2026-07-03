package repo

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

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
