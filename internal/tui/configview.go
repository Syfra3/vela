package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Syfra3/vela/internal/config"
	"github.com/Syfra3/vela/pkg/types"
)

// ConfigViewModel is the TUI model for config viewing/editing.
type ConfigViewModel struct {
	cfg        *types.Config
	configPath string
	quitting   bool
	err        error
}

func NewConfigViewModel() ConfigViewModel {
	cfg, _ := config.Load()
	home, _ := os.UserHomeDir()
	configPath := filepath.Join(home, ".vela", "config.yaml")

	return ConfigViewModel{
		cfg:        cfg,
		configPath: configPath,
	}
}

func (m ConfigViewModel) Init() tea.Cmd {
	return nil
}

func (m ConfigViewModel) Quitting() bool {
	return m.quitting
}

func (m ConfigViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			m.quitting = true
			return m, nil

		case "e":
			// Open in editor
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "vim"
			}
			// TODO: Launch editor (requires shell command execution)
			m.err = fmt.Errorf("editor launch not implemented yet - use: %s %s", editor, m.configPath)
			return m, nil
		}
	}
	return m, nil
}

func (m ConfigViewModel) View() string {
	// Content only — menu will wrap with header/footer
	return m.ViewContent()
}

func (m ConfigViewModel) ViewContent() string {
	var b strings.Builder

	textStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("42")).
		Bold(true)

	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196"))

	b.WriteString(textStyle.Render(fmt.Sprintf("Config file: %s", m.configPath)))
	b.WriteString("\n\n")

	if m.cfg != nil {
		// LLM section
		b.WriteString(labelStyle.Render("LLM:"))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  Provider: %s\n", valueStyle.Render(m.cfg.LLM.Provider)))
		b.WriteString(fmt.Sprintf("  Model: %s\n", valueStyle.Render(m.cfg.LLM.Model)))
		b.WriteString(fmt.Sprintf("  Endpoint: %s\n", valueStyle.Render(m.cfg.LLM.Endpoint)))
		b.WriteString(fmt.Sprintf("  Timeout: %s\n", valueStyle.Render(m.cfg.LLM.Timeout.String())))
		b.WriteString("\n")

		// Extraction section
		b.WriteString(labelStyle.Render("Extraction:"))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  Languages: %s\n", valueStyle.Render(strings.Join(m.cfg.Extraction.CodeLanguages, ", "))))
		b.WriteString(fmt.Sprintf("  Include Docs: %s\n", valueStyle.Render(fmt.Sprintf("%t", m.cfg.Extraction.IncludeDocs))))
		b.WriteString(fmt.Sprintf("  Chunk Size: %s\n", valueStyle.Render(fmt.Sprintf("%d", m.cfg.Extraction.ChunkSize))))
		b.WriteString("\n")

		// UI section
		b.WriteString(labelStyle.Render("UI:"))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  Theme: %s\n", valueStyle.Render(m.cfg.UI.Theme)))
		b.WriteString(fmt.Sprintf("  Show Progress: %s\n", valueStyle.Render(fmt.Sprintf("%t", m.cfg.UI.ShowProgress))))
		b.WriteString("\n")

		// Obsidian section
		b.WriteString(labelStyle.Render("Obsidian:"))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  Auto-sync: %s\n", valueStyle.Render(fmt.Sprintf("%t", m.cfg.Obsidian.AutoSync))))
		vaultDir := m.cfg.Obsidian.VaultDir
		if vaultDir == "" {
			vaultDir = config.DefaultVaultDir()
		}
		b.WriteString(fmt.Sprintf("  Vault dir: %s\n", valueStyle.Render(filepath.Join(vaultDir, "obsidian"))))
		b.WriteString("\n")
	}

	if m.err != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", m.err)))
		b.WriteString("\n\n")
	}

	return b.String()
}
