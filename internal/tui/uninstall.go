package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Syfra3/vela/internal/config"
)

type uninstallState int

const (
	uninstallStateConfirm uninstallState = iota
	uninstallStateRunning
	uninstallStateDone
)

type uninstallResult struct {
	Removed  []string
	Warnings []string
}

type uninstallResultMsg struct {
	result uninstallResult
	err    error
}

var uninstallTargetsFunc = uninstallTargets

type UninstallModel struct {
	state    uninstallState
	quitting bool
	targets  []string
	result   uninstallResult
	err      error
}

func NewUninstallModel() UninstallModel {
	targets, _ := uninstallTargetsFunc()
	return UninstallModel{targets: targets}
}

func (m UninstallModel) Init() tea.Cmd { return nil }

func (m UninstallModel) Quitting() bool { return m.quitting }

func (m UninstallModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch m.state {
		case uninstallStateConfirm:
			switch msg.String() {
			case "ctrl+c", "esc", "b":
				m.quitting = true
				return m, nil
			case "enter", "u", "p":
				m.state = uninstallStateRunning
				m.err = nil
				m.result = uninstallResult{}
				return m, uninstallAllCmd()
			}
		case uninstallStateDone:
			switch msg.String() {
			case "ctrl+c", "esc", "b", "q", "enter":
				m.quitting = true
				return m, nil
			}
		}
	case uninstallResultMsg:
		m.state = uninstallStateDone
		m.result = msg.result
		m.err = msg.err
	}
	return m, nil
}

func (m UninstallModel) View() string { return m.ViewContent() }

func (m UninstallModel) ViewContent() string {
	var b strings.Builder
	textStyle := lipgloss.NewStyle().Foreground(colorText)
	warnStyle := lipgloss.NewStyle().Foreground(colorWarn).Bold(true)
	errorStyle := lipgloss.NewStyle().Foreground(colorErr)
	successStyle := lipgloss.NewStyle().Foreground(colorSuccess)
	mutedStyle := lipgloss.NewStyle().Foreground(colorSubtext)

	switch m.state {
	case uninstallStateConfirm:
		b.WriteString(warnStyle.Render("This purges Vela-managed graph, cache, and export data."))
		b.WriteString("\n")
		b.WriteString(textStyle.Render("It deletes only Vela-managed paths:"))
		b.WriteString("\n\n")
		for _, target := range m.targets {
			b.WriteString(textStyle.Render("  • " + target))
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(mutedStyle.Render("Source repositories are not deleted. Custom files outside these paths are left alone."))
	case uninstallStateRunning:
		b.WriteString(textStyle.Render("Purging Vela-managed data..."))
	case uninstallStateDone:
		if m.err != nil {
			b.WriteString(errorStyle.Render(fmt.Sprintf("Purge failed: %v", m.err)))
			b.WriteString("\n")
		} else {
			b.WriteString(successStyle.Render("Purge complete."))
			b.WriteString("\n")
		}
		if len(m.result.Removed) > 0 {
			b.WriteString("\n")
			b.WriteString(textStyle.Render("Removed:"))
			b.WriteString("\n")
			for _, path := range m.result.Removed {
				b.WriteString(successStyle.Render("  • " + path))
				b.WriteString("\n")
			}
		}
		if len(m.result.Warnings) > 0 {
			b.WriteString("\n")
			b.WriteString(warnStyle.Render("Warnings:"))
			b.WriteString("\n")
			for _, warning := range m.result.Warnings {
				b.WriteString(errorStyle.Render("  • " + warning))
				b.WriteString("\n")
			}
		}
	}
	return b.String()
}

func (m UninstallModel) FooterHelp() string {
	switch m.state {
	case uninstallStateConfirm:
		return "p/u/Enter purge • esc back"
	case uninstallStateRunning:
		return "waiting for purge to finish"
	default:
		return "Enter or esc back to menu"
	}
}

func uninstallAllCmd() tea.Cmd {
	return func() tea.Msg {
		result, err := uninstallAll()
		return uninstallResultMsg{result: result, err: err}
	}
}

func uninstallAll() (uninstallResult, error) {
	targets, err := uninstallTargetsFunc()
	if err != nil {
		return uninstallResult{}, err
	}
	result := uninstallResult{}
	for _, target := range targets {
		if _, err := os.Stat(target); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return result, fmt.Errorf("checking %s: %w", target, err)
		}
		if err := os.RemoveAll(target); err != nil {
			return result, fmt.Errorf("removing %s: %w", target, err)
		}
		result.Removed = append(result.Removed, target)
	}
	return result, nil
}

func uninstallTargets() ([]string, error) {
	targets := []string{config.OutDir(".")}
	vaultDir := config.DefaultVaultDir()
	if cfg, err := config.Load(); err == nil {
		vaultDir = config.ResolveVaultDir(cfg.Obsidian.VaultDir)
	}
	if vaultDir != "" {
		if filepath.Clean(vaultDir) == filepath.Clean(config.DefaultVaultDir()) {
			targets = append(targets, vaultDir)
		} else {
			targets = append(targets, filepath.Join(vaultDir, "obsidian"))
		}
	}
	return uniqueSortedPaths(targets), nil
}

func uniqueSortedPaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	unique := make([]string, 0, len(paths))
	for _, path := range paths {
		clean := filepath.Clean(path)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		unique = append(unique, clean)
	}
	sort.Strings(unique)
	return unique
}

var _ = errors.New
