package setup

import (
	"context"
	"errors"
	"testing"

	"github.com/Syfra3/vela/pkg/types"
)

func TestWizardValidateLocalReadiness(t *testing.T) {
	originalCheckLLMHealth := wizardCheckLLMHealth
	originalCheckVelaMCPForTarget := wizardCheckVelaMCPForTarget
	originalCheckOllama := wizardCheckOllama
	originalGetOllamaModels := wizardGetOllamaModels
	originalLookPath := wizardLookPath
	t.Cleanup(func() {
		wizardCheckLLMHealth = originalCheckLLMHealth
		wizardCheckVelaMCPForTarget = originalCheckVelaMCPForTarget
		wizardCheckOllama = originalCheckOllama
		wizardGetOllamaModels = originalGetOllamaModels
		wizardLookPath = originalLookPath
	})

	wizardLookPath = func(file string) (string, error) { return "/usr/bin/vela", nil }
	wizardCheckOllama = func() (bool, bool, string, error) { return true, true, "/usr/bin/ollama", nil }
	wizardGetOllamaModels = func() ([]string, error) { return []string{"llama3:latest", LocalSearchEmbeddingModel + ":latest"}, nil }
	wizardCheckLLMHealth = func(ctx context.Context, cfg *types.LLMConfig) error { return nil }
	wizardCheckVelaMCPForTarget = func(target string) bool { return target == IntegrationTargetClaudeCode }

	m := WizardModel{setupMode: IntegrationModeVelaOnly, providerChoice: 0, selectedModel: "llama3", mcpTarget: 0}
	msg := m.validate()().(validationMsg)

	if msg.err != nil {
		t.Fatalf("validate err = %v, want nil", msg.err)
	}
	if !msg.llmOK || !msg.mcpOK {
		t.Fatalf("validate status = llm:%v mcp:%v, want both true", msg.llmOK, msg.mcpOK)
	}
	if len(msg.messages) < 3 {
		t.Fatalf("messages = %v, want readiness details", msg.messages)
	}
}

func TestWizardValidateFailsWhenEmbeddingModelMissing(t *testing.T) {
	originalCheckLLMHealth := wizardCheckLLMHealth
	originalCheckVelaMCPForTarget := wizardCheckVelaMCPForTarget
	originalCheckOllama := wizardCheckOllama
	originalGetOllamaModels := wizardGetOllamaModels
	originalLookPath := wizardLookPath
	t.Cleanup(func() {
		wizardCheckLLMHealth = originalCheckLLMHealth
		wizardCheckVelaMCPForTarget = originalCheckVelaMCPForTarget
		wizardCheckOllama = originalCheckOllama
		wizardGetOllamaModels = originalGetOllamaModels
		wizardLookPath = originalLookPath
	})

	wizardLookPath = func(file string) (string, error) { return "/usr/bin/vela", nil }
	wizardCheckOllama = func() (bool, bool, string, error) { return true, true, "/usr/bin/ollama", nil }
	wizardGetOllamaModels = func() ([]string, error) { return []string{"llama3:latest"}, nil }
	wizardCheckLLMHealth = func(ctx context.Context, cfg *types.LLMConfig) error { return nil }
	wizardCheckVelaMCPForTarget = func(string) bool { return true }

	m := WizardModel{setupMode: IntegrationModeVelaOnly, providerChoice: 0, selectedModel: "llama3", mcpTarget: 0}
	msg := m.validate()().(validationMsg)

	if msg.err == nil {
		t.Fatal("expected validation error when embedding model is missing")
	}
	if msg.err.Error() != "Ollama embedding model \"nomic-embed-text\" is not available" {
		t.Fatalf("validate error = %v", msg.err)
	}
}

func TestWizardValidateFailsWhenRemoteHealthCheckFails(t *testing.T) {
	originalCheckLLMHealth := wizardCheckLLMHealth
	originalLookPath := wizardLookPath
	t.Cleanup(func() {
		wizardCheckLLMHealth = originalCheckLLMHealth
		wizardLookPath = originalLookPath
	})

	wizardLookPath = func(file string) (string, error) { return "/usr/bin/vela", nil }
	wizardCheckLLMHealth = func(ctx context.Context, cfg *types.LLMConfig) error {
		return errors.New("provider unreachable")
	}

	m := WizardModel{setupMode: IntegrationModeAncoraVela, providerChoice: 1, remoteProvider: 0, apiKey: "secret", mcpTarget: 2}
	msg := m.validate()().(validationMsg)

	if msg.err == nil {
		t.Fatal("expected validation error")
	}
	if msg.llmOK {
		t.Fatalf("llmOK = %v, want false", msg.llmOK)
	}
}

func TestWizardValidateAncoraVelaModeDoesNotRequireDirectVelaMCP(t *testing.T) {
	originalCheckLLMHealth := wizardCheckLLMHealth
	originalLookPath := wizardLookPath
	t.Cleanup(func() {
		wizardCheckLLMHealth = originalCheckLLMHealth
		wizardLookPath = originalLookPath
	})

	wizardLookPath = func(file string) (string, error) {
		switch file {
		case "vela":
			return "/usr/bin/vela", nil
		case "ancora":
			return "/usr/bin/ancora", nil
		default:
			return "", errors.New("missing")
		}
	}
	wizardCheckLLMHealth = func(ctx context.Context, cfg *types.LLMConfig) error { return nil }

	m := WizardModel{setupMode: IntegrationModeAncoraVela, providerChoice: 1, remoteProvider: 0, apiKey: "secret"}
	msg := m.validate()().(validationMsg)

	if msg.err != nil {
		t.Fatalf("validate err = %v, want nil", msg.err)
	}
	if !msg.mcpOK {
		t.Fatal("expected Ancora+Vela mode to validate MCP integration without direct Vela registration")
	}
}

func TestWizardValidationMessageMovesToErrorState(t *testing.T) {
	t.Parallel()

	m := WizardModel{step: StepValidation, working: true}
	updated, _ := m.Update(validationMsg{err: errors.New("provider unreachable")})
	wizard := updated.(WizardModel)

	if wizard.step != StepError {
		t.Fatalf("step = %v, want %v", wizard.step, StepError)
	}
	if wizard.err == nil {
		t.Fatal("expected wizard error to be stored")
	}
}

func TestWizardValidationFinalizesSetupAndStartsDaemon(t *testing.T) {
	originalEnableObsidianAutoSync := wizardEnableObsidianAutoSync
	originalEnsureDaemonRunning := wizardEnsureDaemonRunning
	originalSaveIntegrationState := wizardSaveIntegrationState
	t.Cleanup(func() {
		wizardEnableObsidianAutoSync = originalEnableObsidianAutoSync
		wizardEnsureDaemonRunning = originalEnsureDaemonRunning
		wizardSaveIntegrationState = originalSaveIntegrationState
	})

	obsidianEnabled := false
	wizardEnableObsidianAutoSync = func(enable bool) error {
		obsidianEnabled = enable
		return nil
	}
	wizardEnsureDaemonRunning = func() (bool, error) {
		return true, nil
	}
	wizardSaveIntegrationState = func(IntegrationState) error { return nil }

	updated, cmd := (WizardModel{step: StepValidation, working: true, setupMode: IntegrationModeAncoraVela}).Update(validationMsg{
		llmOK:    true,
		mcpOK:    true,
		messages: []string{"✓ Validation passed"},
	})
	wizard := updated.(WizardModel)

	if !wizard.working {
		t.Fatal("expected wizard to stay busy while finalizing setup")
	}

	finalizeMsg, ok := cmd().(setupFinalizeMsg)
	if !ok {
		t.Fatalf("cmd() type = %T, want setupFinalizeMsg", cmd())
	}
	if !obsidianEnabled {
		t.Fatal("expected setup finalization to enable obsidian auto-sync")
	}

	updated, _ = wizard.Update(finalizeMsg)
	wizard = updated.(WizardModel)

	if wizard.step != StepComplete {
		t.Fatalf("step = %v, want %v", wizard.step, StepComplete)
	}
	if wizard.working {
		t.Fatal("expected finalization to finish")
	}
	if len(wizard.message) != 7 {
		t.Fatalf("messages = %v, want validation + completion details", wizard.message)
	}
	if wizard.message[3] != "✓ Integration mode saved: Ancora + Vela" {
		t.Fatalf("message[3] = %q", wizard.message[3])
	}
	if wizard.message[4] != "✓ Direct Vela MCP registration removed so primary ownership stays with Ancora" {
		t.Fatalf("message[4] = %q", wizard.message[4])
	}
	if wizard.message[5] != "✓ Obsidian auto-sync enabled by default" {
		t.Fatalf("message[5] = %q", wizard.message[5])
	}
	if wizard.message[6] != "✓ Watch daemon started" {
		t.Fatalf("message[6] = %q", wizard.message[6])
	}
}
