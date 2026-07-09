package stack

import (
	"context"
	"errors"
	"testing"

	"github.com/llin/cttw/internal/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeGit struct {
	pushed []string
	err    error
}

func (f *fakeGit) PushForce(branch string) error {
	if f.err != nil {
		return f.err
	}
	f.pushed = append(f.pushed, branch)
	return nil
}

func prWithHead(num int, ref string) github.PullRequest {
	p := github.PullRequest{Number: num}
	p.Head.Ref = ref
	return p
}

type fakeGitHub struct {
	createNum    int
	createErr    error
	getPR        *github.PullRequest
	getErr       error
	listPRs      []github.PullRequest
	listErr      error
	updateErr    error
	updated      []updateCall
	created      []createCall
}

type createCall struct {
	Title string
	Body  string
	Head  string
	Base  string
}

type updateCall struct {
	Number int
	Title  string
	Body   string
	Base   string
}

func (f *fakeGitHub) CreateIssue(ctx context.Context, owner, repo, title, body string) (int, error) {
	return 0, nil
}

func (f *fakeGitHub) CreateSubIssue(ctx context.Context, owner, repo string, parentNumber, childNumber int) error {
	return nil
}

func (f *fakeGitHub) CreateBranch(ctx context.Context, owner, repo, branch, base string) error {
	return nil
}

func (f *fakeGitHub) CreatePullRequest(ctx context.Context, owner, repo, title, body, head, base string) (int, error) {
	if f.createErr != nil {
		return 0, f.createErr
	}
	f.createNum++
	f.created = append(f.created, createCall{Title: title, Body: body, Head: head, Base: base})
	return f.createNum, nil
}

func (f *fakeGitHub) GetPullRequest(ctx context.Context, owner, repo string, number int) (*github.PullRequest, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.getPR, nil
}

func (f *fakeGitHub) ListPullRequests(ctx context.Context, owner, repo, head, base string) ([]github.PullRequest, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.listPRs, nil
}

func (f *fakeGitHub) UpdatePullRequest(ctx context.Context, owner, repo string, number int, title, body, base string) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	f.updated = append(f.updated, updateCall{Number: number, Title: title, Body: body, Base: base})
	return nil
}

func TestRunner_Submit_CreatesPRsAndCrossLinksBodies(t *testing.T) {
	git := &fakeGit{}
	gh := &fakeGitHub{}
	r := &Runner{Git: git, GitHub: gh, Owner: "o", Name: "r"}

	groups := []Group{
		{ID: "g1", Title: "base feature", Description: "add base", Branch: "cttw/base", BaseBranch: "main"},
		{ID: "g2", Title: "top feature", Description: "add top", Branch: "cttw/top", BaseBranch: "cttw/base"},
	}

	ctx := context.Background()
	require.NoError(t, r.Submit(ctx, groups))

	assert.Equal(t, []string{"cttw/base", "cttw/top"}, git.pushed)
	require.Len(t, gh.created, 2)
	assert.Equal(t, "cttw/base", gh.created[0].Head)
	assert.Equal(t, "main", gh.created[0].Base)
	assert.Equal(t, "cttw/top", gh.created[1].Head)
	assert.Equal(t, "cttw/base", gh.created[1].Base)

	// Both PR numbers should be assigned and bodies updated with cross-links.
	require.Len(t, gh.updated, 2)
	assert.Equal(t, 1, groups[0].PRNumber)
	assert.Equal(t, 2, groups[1].PRNumber)

	assert.Contains(t, gh.updated[0].Body, "### Stack")
	assert.Contains(t, gh.updated[0].Body, "#2")
	assert.Contains(t, gh.updated[0].Body, "#1 <- this PR")
	assert.Contains(t, gh.updated[1].Body, "#2 <- this PR")
}

func TestRunner_Submit_UpdatesExistingPRByHead(t *testing.T) {
	git := &fakeGit{}
	gh := &fakeGitHub{
		listPRs: []github.PullRequest{prWithHead(42, "cttw/base")},
	}
	r := &Runner{Git: git, GitHub: gh, Owner: "o", Name: "r"}

	groups := []Group{
		{ID: "g1", Title: "base feature", Description: "add base", Branch: "cttw/base", BaseBranch: "main"},
	}

	ctx := context.Background()
	require.NoError(t, r.Submit(ctx, groups))

	assert.Empty(t, gh.created)
	require.Len(t, gh.updated, 1)
	assert.Equal(t, 42, gh.updated[0].Number)
	assert.Equal(t, "main", gh.updated[0].Base)
	assert.Equal(t, 42, groups[0].PRNumber)
}

func TestRunner_Submit_ReusesStoredPRNumber(t *testing.T) {
	git := &fakeGit{}
	gh := &fakeGitHub{
		getPR: func() *github.PullRequest { p := prWithHead(99, "cttw/base"); return &p }(),
	}
	r := &Runner{Git: git, GitHub: gh, Owner: "o", Name: "r"}

	groups := []Group{
		{ID: "g1", Title: "base feature", Description: "add base", Branch: "cttw/base", BaseBranch: "main", PRNumber: 99},
	}

	ctx := context.Background()
	require.NoError(t, r.Submit(ctx, groups))

	assert.Empty(t, gh.created)
	assert.Empty(t, gh.listPRs)
	require.Len(t, gh.updated, 1)
	assert.Equal(t, 99, gh.updated[0].Number)
	assert.Equal(t, 99, groups[0].PRNumber)
}

func TestRunner_Submit_StripsOldMarkerOnUpdate(t *testing.T) {
	git := &fakeGit{}
	gh := &fakeGitHub{
		getPR: func() *github.PullRequest { p := prWithHead(5, "cttw/base"); return &p }(),
	}
	r := &Runner{Git: git, GitHub: gh, Owner: "o", Name: "r"}

	groups := []Group{
		{ID: "g1", Title: "base feature", Description: "add base\n\n<!-- cttw-stack -->\nold marker", Branch: "cttw/base", BaseBranch: "main", PRNumber: 5},
	}

	ctx := context.Background()
	require.NoError(t, r.Submit(ctx, groups))

	require.Len(t, gh.updated, 1)
	body := gh.updated[0].Body
	assert.Contains(t, body, "add base")
	assert.NotContains(t, body, "old marker")
	assert.Contains(t, body, stackMarker)
}

func TestRunner_Submit_FailsOnEmptyStack(t *testing.T) {
	r := &Runner{Git: &fakeGit{}, GitHub: &fakeGitHub{}, Owner: "o", Name: "r"}
	require.NoError(t, r.Submit(context.Background(), nil))
}

func TestRunner_Submit_FailsOnPushError(t *testing.T) {
	pushErr := errors.New("network error")
	r := &Runner{
		Git:    &fakeGit{err: pushErr},
		GitHub: &fakeGitHub{},
		Owner:  "o",
		Name:   "r",
	}
	err := r.Submit(context.Background(), []Group{{Branch: "feat"}})
	require.Error(t, err)
	assert.ErrorIs(t, err, pushErr)
}

func TestRunner_Submit_FailsOnCreateError(t *testing.T) {
	createErr := errors.New("github error")
	r := &Runner{
		Git:    &fakeGit{},
		GitHub: &fakeGitHub{createErr: createErr},
		Owner:  "o",
		Name:   "r",
	}
	err := r.Submit(context.Background(), []Group{{ID: "g1", Title: "t", Branch: "feat", BaseBranch: "main"}})
	require.Error(t, err)
	assert.ErrorIs(t, err, createErr)
}

func TestBuildBody_KeepsDescriptionAndAddsMarker(t *testing.T) {
	groups := []Group{
		{PRNumber: 10},
		{PRNumber: 11},
	}
	body := buildBody("Description line.", groups, 0)
	assert.Contains(t, body, "Description line.")
	assert.Contains(t, body, "#11")
	assert.Contains(t, body, "#10 <- this PR")
}
