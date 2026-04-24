package app

import (
	"errors"
	"testing"

	"github.com/Syfra3/vela/pkg/types"
)

type stubRunner struct {
	request types.QueryRequest
	output  string
	err     error
}

func (s *stubRunner) RunRequest(req types.QueryRequest) (string, error) {
	s.request = req
	return s.output, s.err
}

func TestQueryServiceRun_NormalizesAndExecutesRequest(t *testing.T) {
	t.Parallel()

	runner := &stubRunner{output: "Dependencies for \"AuthService\""}
	svc := QueryService{LoadEngine: func(string) (QueryRunner, error) { return runner, nil }}

	output, req, err := svc.Run(QueryRequestInput{GraphPath: "/tmp/graph.json", Kind: types.QueryKindDependencies, Subject: "AuthService", Limit: 0})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if output != runner.output {
		t.Fatalf("output = %q, want %q", output, runner.output)
	}
	if req.Limit != types.DefaultQueryLimit {
		t.Fatalf("Limit = %d, want default %d", req.Limit, types.DefaultQueryLimit)
	}
	if !req.IncludeProvenance {
		t.Fatal("expected IncludeProvenance to be enabled")
	}
	if runner.request != req {
		t.Fatalf("runner request = %+v, want %+v", runner.request, req)
	}
}

func TestQueryServiceRun_RejectsInvalidRequestsBeforeExecution(t *testing.T) {
	t.Parallel()

	called := false
	svc := QueryService{LoadEngine: func(string) (QueryRunner, error) {
		called = true
		return &stubRunner{}, nil
	}}

	_, _, err := svc.Run(QueryRequestInput{Kind: types.QueryKindPath, Subject: "AuthService"})
	if err == nil {
		t.Fatal("Run() error = nil, want validation error")
	}
	if called {
		t.Fatal("expected invalid request to fail before loading engine")
	}
	if err.Error() != "query target is required for path queries" {
		t.Fatalf("unexpected error %q", err)
	}

	loadErr := errors.New("load fail")
	svc = QueryService{LoadEngine: func(string) (QueryRunner, error) { return nil, loadErr }}
	_, _, err = svc.Run(QueryRequestInput{Kind: types.QueryKindDependencies, Subject: "AuthService"})
	if !errors.Is(err, loadErr) {
		t.Fatalf("Run() error = %v, want %v", err, loadErr)
	}
}
