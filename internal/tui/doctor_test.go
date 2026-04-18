package tui

import (
	"strings"
	"testing"
)

func TestNewDoctorModelShowsLoadingImmediately(t *testing.T) {
	t.Parallel()

	m := NewDoctorModel()
	view := m.ViewContent()

	if !m.checking {
		t.Fatal("expected new doctor model to start in checking state")
	}
	if !strings.Contains(view, "Checking LLM providers...") {
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
	if !strings.Contains(menu.viewDoctor(), "Checking LLM providers...") {
		t.Fatalf("expected doctor screen to show loading state, got %q", menu.viewDoctor())
	}
	if strings.Contains(menu.viewDoctor(), "r re-check") {
		t.Fatalf("expected footer to stay in initial loading state, got %q", menu.viewDoctor())
	}
}
