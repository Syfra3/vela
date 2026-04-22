package app

import (
	"github.com/Syfra3/vela/internal/query"
	"github.com/Syfra3/vela/pkg/types"
)

type QueryRunner interface {
	RunRequest(req types.QueryRequest) (string, error)
}

type QueryRequestInput struct {
	GraphPath         string
	Kind              types.QueryKind
	Subject           string
	Target            string
	Limit             int
	IncludeProvenance bool
}

type QueryService struct {
	LoadEngine func(string) (QueryRunner, error)
}

func NormalizeQueryRequest(input QueryRequestInput) (types.QueryRequest, error) {
	req := types.QueryRequest{
		Kind:              input.Kind,
		Subject:           input.Subject,
		Target:            input.Target,
		Limit:             input.Limit,
		IncludeProvenance: input.IncludeProvenance,
	}.Normalize()
	return req, req.Validate()
}

func (s QueryService) Run(input QueryRequestInput) (string, types.QueryRequest, error) {
	req, err := NormalizeQueryRequest(input)
	if err != nil {
		return "", types.QueryRequest{}, err
	}
	loadEngine := s.LoadEngine
	if loadEngine == nil {
		loadEngine = func(graphPath string) (QueryRunner, error) {
			return query.LoadFromFile(graphPath)
		}
	}
	runner, err := loadEngine(input.GraphPath)
	if err != nil {
		return "", types.QueryRequest{}, err
	}
	output, err := runner.RunRequest(req)
	return output, req, err
}
