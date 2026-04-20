package tui

import (
	"strings"
	"testing"
	vdoctor "github.com/Syfra3/vela/internal/doctor"
)

func TestNewDoctorModelShowsLoadingImmediately(t *testing.T) {
	t.Parallel()

	m := NewDoctorModel()
	view := m.ViewContent()

	if !m.checking {
		t.Fatal("expected new doctor model to start in checking state")
	}
	if !strings.Contains(view, "Checking configured provider and integrations...") {
		t.Fatalf("expected loading message in doctor view, got %q", view)
	}
}

func TestHandleMenuSelectDoctorStartsHealthCheck(t *testing.T) {
	t.Parallel()

	m := NewMenuModel()
	doctorIndex := -1
	for i, item := range m.items {
		if item.key == "doctor" {
			doctorIndex = i
			break
		}
	}
	if doctorIndex == -1 {
		t.Fatal("doctor menu item not found")
	}

	m.cursor = doctorIndex
	updated, cmd := m.handleMenuSelect()
	menu := updated.(MenuModel)

	if menu.screen != screenDoctor {
		t.Fatalf("screen = %v, want %v", menu.screen, screenDoctor)
	}
	if !menu.doctorModel.checking {
		t.Fatal("expected doctor model to be checking after selection")
	}
	if cmd == nil {
		t.Fatal("expected doctor selection to return init command")
	}
	if !strings.Contains(menu.viewDoctor(), "Checking configured provider and integrations...") {
		t.Fatalf("expected doctor screen to show loading state, got %q", menu.viewDoctor())
	}
	if strings.Contains(menu.viewDoctor(), "r re-check") {
		t.Fatalf("expected footer to stay in initial loading state, got %q", menu.viewDoctor())
	}
}

func TestDoctorModelRendersConfiguredProviderAndIntegrationReport(t *testing.T) {
	t.Parallel()

	m := DoctorModel{}
	updated, _ := m.Update(doctorReportMsg{
		llmResults:        []healthCheckMsg{{provider: "local[llama3 @ http://localhost:11434]", ok: true}},
		integrationChecks: stepLikesToDoctorSteps([]StepLike{{Name: "Graph update / reconcile", Status: "ok", Detail: "graph.json is healthy"}}),
	})

	view := updated.(DoctorModel).ViewContent()
	if !strings.Contains(view, "local[llama3 @ http://localhost:11434]") {
		t.Fatalf("expected provider name in view, got %q", view)
	}
	if !strings.Contains(view, "Graph update / reconcile") {
		t.Fatalf("expected integration section in view, got %q", view)
	}
}

type StepLike struct {
	Name   string
	Status string
	Detail string
}

func (s StepLike) toDoctorStep() vdoctor.Step {
	return vdoctor.Step{Name: s.Name, Status: vdoctor.StepStatus(s.Status), Detail: s.Detail}
}

func stepLikesToDoctorSteps(steps []StepLike) []vdoctor.Step {
	out := make([]vdoctor.Step, 0, len(steps))
	for _, step := range steps {
		out = append(out, step.toDoctorStep())
	}
	return out
}
