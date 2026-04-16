package extract

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Syfra3/vela/pkg/types"
)

type fakeLLMProvider struct {
	result *types.ExtractionResult
	err    error
}

func (f *fakeLLMProvider) ExtractGraph(context.Context, string, string) (*types.ExtractionResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func (f *fakeLLMProvider) Health(context.Context) error { return nil }
func (f *fakeLLMProvider) Name() string                 { return "fake" }

func TestLLMExtractorExtractMergesLLMAndExplicitReferences(t *testing.T) {
	t.Parallel()

	e := &LLMExtractor{
		client: &fakeLLMProvider{result: &types.ExtractionResult{
			Nodes: []types.Node{{Description: `[{"type":"concept","target":"Security"}]`}},
		}},
	}

	obs := types.ObservationNode{
		Title:   "Decision for `AuthService`",
		Content: "See internal/auth/service.go and [[Security]]",
	}

	refs, err := e.extract(context.Background(), obs)
	if err != nil {
		t.Fatalf("extract() error = %v", err)
	}

	got := make(map[string]string, len(refs))
	for _, ref := range refs {
		got[ref.Type] = ref.Target
	}
	if got["concept"] != "Security" {
		t.Fatalf("concept ref = %q, want Security", got["concept"])
	}
	if got["file"] != "internal/auth/service.go" {
		t.Fatalf("file ref = %q, want internal/auth/service.go", got["file"])
	}
	if got["function"] != "AuthService" {
		t.Fatalf("function ref = %q, want AuthService", got["function"])
	}
}

func TestLLMExtractorStartWorkerPoolInvokesCallback(t *testing.T) {
	t.Parallel()

	refsCh := make(chan []types.ObsReference, 1)
	e := &LLMExtractor{
		client: &fakeLLMProvider{result: &types.ExtractionResult{
			Nodes: []types.Node{{NodeType: "concept", Label: "Security"}},
		}},
		workerCount: 2,
		onRefsFound: func(_ int64, refs []types.ObsReference) {
			refsCh <- refs
		},
	}

	queue := make(chan types.ObservationNode, 1)
	queue <- types.ObservationNode{AncoraID: 99}
	close(queue)

	done := make(chan struct{})
	go func() {
		e.Start(context.Background(), queue)
		close(done)
	}()

	select {
	case refs := <-refsCh:
		if len(refs) != 1 || refs[0].Type != "concept" || refs[0].Target != "Security" {
			t.Fatalf("callback refs = %#v, want concept Security", refs)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for worker callback")
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker pool did not terminate after queue close")
	}
}

func TestLLMExtractorExtractReturnsProviderError(t *testing.T) {
	t.Parallel()

	e := &LLMExtractor{client: &fakeLLMProvider{err: errors.New("boom")}}
	_, err := e.extract(context.Background(), types.ObservationNode{})
	if err == nil || err.Error() != "boom" {
		t.Fatalf("extract() error = %v, want boom", err)
	}
}
