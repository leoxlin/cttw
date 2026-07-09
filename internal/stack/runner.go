// Package stack implements native stacked pull request publishing for cttw.
//
// It mirrors the modular/stack-pr workflow: ordered branches are force-pushed,
// and a pull request is created (or updated) for each branch with the previous
// branch as its base. PR bodies are cross-linked with a stable marker section.
package stack

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/llin/cttw/internal/github"
)

const stackMarker = "<!-- cttw-stack -->"

// Group is a single PR in a stack.
type Group struct {
	ID          string
	Title       string
	Description string
	Branch      string
	BaseBranch  string
	PRNumber    int
}

// GitPusher pushes local branches to the remote.
type GitPusher interface {
	PushForce(branch string) error
}

// Runner publishes a stack of PRs.
type Runner struct {
	Git    GitPusher
	GitHub github.Client
	Owner  string
	Name   string
}

// Submit pushes all group branches and creates or updates their PRs.
// On success each group's PRNumber is populated with the live PR number.
func (r *Runner) Submit(ctx context.Context, groups []Group) error {
	if len(groups) == 0 {
		return nil
	}
	if r.Git == nil {
		return errors.New("git pusher is required")
	}
	if r.GitHub == nil {
		return errors.New("github client is required")
	}
	if r.Owner == "" || r.Name == "" {
		return errors.New("owner and name are required")
	}

	for i := range groups {
		if groups[i].Branch == "" {
			return fmt.Errorf("group %d has no branch", i)
		}
		if i > 0 && groups[i].BaseBranch == "" {
			return fmt.Errorf("group %d has no base branch", i)
		}
	}

	// Push branches in stack order so the remote refs exist before PR creation.
	for _, g := range groups {
		if err := r.Git.PushForce(g.Branch); err != nil {
			return fmt.Errorf("push %s: %w", g.Branch, err)
		}
	}

	// First pass: ensure each PR exists and collect its number.
	for i := range groups {
		g := &groups[i]
		prNum, err := r.resolveOrCreatePR(ctx, g)
		if err != nil {
			return fmt.Errorf("group %q (%s): %w", g.Title, g.Branch, err)
		}
		g.PRNumber = prNum
	}

	// Second pass: rewrite bodies with cross-links using the final numbers.
	for i := range groups {
		g := &groups[i]
		body := buildBody(g.Description, groups, i)
		if err := r.GitHub.UpdatePullRequest(ctx, r.Owner, r.Name, g.PRNumber, g.Title, body, g.BaseBranch); err != nil {
			return fmt.Errorf("update body for #%d: %w", g.PRNumber, err)
		}
	}

	return nil
}

func (r *Runner) resolveOrCreatePR(ctx context.Context, g *Group) (int, error) {
	// If we already recorded a PR number, verify it still exists and points to the right head.
	if g.PRNumber > 0 {
		pr, err := r.GitHub.GetPullRequest(ctx, r.Owner, r.Name, g.PRNumber)
		if err == nil && pr != nil && pr.Head.Ref == g.Branch {
			return pr.Number, nil
		}
		// Fall through to search by head; we'll update the stored number afterward.
	}

	prs, err := r.GitHub.ListPullRequests(ctx, r.Owner, r.Name, g.Branch, "")
	if err != nil {
		return 0, fmt.Errorf("list prs: %w", err)
	}
	for _, pr := range prs {
		if pr.Head.Ref == g.Branch {
			// Title, body, and base are refreshed in the second pass once all
			// PR numbers in the stack are known.
			return pr.Number, nil
		}
	}

	num, err := r.GitHub.CreatePullRequest(ctx, r.Owner, r.Name, g.Title, "", g.Branch, g.BaseBranch)
	if err != nil {
		return 0, fmt.Errorf("create pr: %w", err)
	}
	return num, nil
}

// buildBody returns the group description with a deterministic stack marker section.
func buildBody(description string, groups []Group, idx int) string {
	description = strings.TrimSpace(description)
	// Strip any previously generated marker section so updates are idempotent.
	if i := strings.Index(description, "\n"+stackMarker); i >= 0 {
		description = strings.TrimSpace(description[:i])
	} else if i := strings.Index(description, stackMarker); i >= 0 {
		description = strings.TrimSpace(description[:i])
	}

	var b strings.Builder
	if description != "" {
		b.WriteString(description)
		b.WriteString("\n\n")
	}
	b.WriteString(stackMarker)
	b.WriteString("\n### Stack\n\n")
	for i := len(groups) - 1; i >= 0; i-- {
		g := groups[i]
		if g.PRNumber == 0 {
			continue
		}
		line := fmt.Sprintf("- #%d", g.PRNumber)
		if i == idx {
			line += " <- this PR"
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}
