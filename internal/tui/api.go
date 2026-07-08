package tui

import "github.com/llin/cttw/internal/api"

var (
	listProblems = func(socket string) ([]api.ProblemResponse, error) {
		return api.NewClient(socket).ListProblems()
	}
	getProblem = func(socket, id string) (*api.ProblemResponse, error) {
		return api.NewClient(socket).GetProblem(id)
	}
	createProblem = func(socket, owner, repo, description string) (*api.ProblemResponse, error) {
		return api.NewClient(socket).CreateProblem(owner, repo, description)
	}
)
