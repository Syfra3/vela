package tui

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"

	"github.com/Syfra3/vela/internal/config"
	"github.com/Syfra3/vela/internal/daemon"
	igraph "github.com/Syfra3/vela/internal/graph"
	"github.com/Syfra3/vela/internal/listener"
	"github.com/Syfra3/vela/pkg/types"
)

// ---------------------------------------------------------------------------
// Watch TUI Screen — spec §6.7
// ---------------------------------------------------------------------------

// watchMenuItem enumerates the action items in the watch submenu.
type watchMenuItem int

const (
	watchItemToggle   watchMenuItem = iota // Start / Stop daemon
	watchItemObsidian                      // Toggle Obsidian auto-sync
	watchItemAdd                           // Add source
	watchItemInstall                       // Install service
	watchItemLogs                          // View logs hint
	watchItemBack                          // Back to main menu
	watchItemCount
)

var watchMenuLabels = [watchItemCount]string{
	watchItemToggle:   "Start/Stop daemon",
	watchItemObsidian: "Obsidian auto-sync",
	watchItemAdd:      "Add source",
	watchItemInstall:  "Install as service",
	watchItemLogs:     "View logs",
	watchItemBack:     "Back",
}

var watchMenuKeys = [watchItemCount]string{
	watchItemToggle:   "s",
	watchItemObsidian: "o",
	watchItemAdd:      "a",
	watchItemInstall:  "i",
	watchItemLogs:     "l",
	watchItemBack:     "b",
}

// watchStatusMsg carries a periodic status refresh from the daemon.
type watchStatusMsg struct {
	status      string
	sources     map[string]listener.SourceStatus
	daemonOK    bool
	ancoraOK    bool // ancora mcp socket is live
	pid         int
	obsidianOn  bool   // obsidian.auto_sync from config
	obsidianDir string // obsidian.vault_dir from config
}

// watchActionMsg is returned after performing a daemon action (start/stop).
type watchActionMsg struct {
	err error
}

// WatchModel is the TUI model for the Watch screen.
type WatchModel struct {
	cursor   int
	quitting bool
	message  string
	msgIsErr bool

	// daemon + integration state (refreshed every 3s)
	daemonOK    bool
	ancoraOK    bool // ancora mcp socket alive
	pid         int
	status      string
	sources     map[string]listener.SourceStatus
	obsidianOn  bool   // current obsidian.auto_sync value
	obsidianDir string // current obsidian.vault_dir value

	d *daemon.Daemon // may be nil if daemon fails to init
}

// NewWatchModel creates a WatchModel. The daemon is lazily loaded.
func NewWatchModel() WatchModel {
	return WatchModel{
		cursor: 0,
	}
}

func (m WatchModel) Init() tea.Cmd {
	return tea.Batch(
		m.refreshStatus(),
		tickWatchStatus(),
	)
}

func (m WatchModel) Quitting() bool { return m.quitting }

func (m WatchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case watchStatusTick:
		return m, m.refreshStatus()

	case watchStatusMsg:
		m.daemonOK = msg.daemonOK
		m.ancoraOK = msg.ancoraOK
		m.pid = msg.pid
		m.status = msg.status
		m.sources = msg.sources
		m.obsidianOn = msg.obsidianOn
		m.obsidianDir = msg.obsidianDir
		return m, tickWatchStatus()

	case watchActionMsg:
		if msg.err != nil {
			m.message = msg.err.Error()
			m.msgIsErr = true
		} else {
			m.message = "Done."
			m.msgIsErr = false
		}
		return m, m.refreshStatus()
	}
	return m, nil
}

func (m WatchModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc", "b":
		m.quitting = true
		return m, nil

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}

	case "down", "j":
		if m.cursor < int(watchItemCount)-1 {
			m.cursor++
		}

	case "enter", " ":
		return m.handleSelect()

	// Direct key shortcuts
	case "s":
		m.cursor = int(watchItemToggle)
		return m.handleSelect()
	case "o":
		m.cursor = int(watchItemObsidian)
		return m.handleSelect()
	case "a":
		m.cursor = int(watchItemAdd)
		return m.handleSelect()
	case "i":
		m.cursor = int(watchItemInstall)
		return m.handleSelect()
	}
	return m, nil
}

func (m WatchModel) handleSelect() (tea.Model, tea.Cmd) {
	switch watchMenuItem(m.cursor) {
	case watchItemToggle:
		if m.daemonOK {
			return m, stopDaemonCmd()
		}
		return m, startDaemonCmd()

	case watchItemObsidian:
		return m, toggleObsidianCmd(!m.obsidianOn)

	case watchItemInstall:
		return m, installServiceCmd()

	case watchItemBack:
		m.quitting = true
		return m, nil

	case watchItemAdd, watchItemLogs:
		m.message = "Use 'vela watch add <name>' or 'vela watch logs' from the CLI."
		m.msgIsErr = false
	}
	return m, nil
}

// View renders the watch screen content only (menu wraps with header/footer).
func (m WatchModel) View() string { return m.ViewContent() }

func (m WatchModel) ViewContent() string {
	var b strings.Builder

	dot := func(ok bool) string {
		if ok {
			return lipgloss.NewStyle().Foreground(colorSuccess).Render("●")
		}
		return lipgloss.NewStyle().Foreground(colorErr).Render("○")
	}
	label := lipgloss.NewStyle().Foreground(colorSubtext).Width(18)
	value := lipgloss.NewStyle().Foreground(colorText)

	// ── Integration status panel ─────────────────────────────────────────
	b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("Integration Status"))
	b.WriteString("\n")

	// Ancora MCP row — socket probe, not daemon state
	ancoraStatus := "offline  (start: ancora mcp)"
	if m.ancoraOK {
		ancoraStatus = "online"
	}
	b.WriteString(fmt.Sprintf("  %s  %s %s\n",
		dot(m.ancoraOK),
		label.Render("Ancora MCP"),
		value.Render(ancoraStatus),
	))

	// Vela daemon row
	daemonStatus := "stopped"
	if m.daemonOK {
		daemonStatus = fmt.Sprintf("running  (pid %d)", m.pid)
	}
	b.WriteString(fmt.Sprintf("  %s  %s %s\n",
		dot(m.daemonOK),
		label.Render("Vela Daemon"),
		value.Render(daemonStatus),
	))

	// Sources rows — only when daemon is running
	if m.daemonOK && len(m.sources) > 0 {
		for name, s := range m.sources {
			connStr := "disconnected"
			if s.Connected {
				connStr = fmt.Sprintf("connected  (%d events)", s.EventCount)
			}
			b.WriteString(fmt.Sprintf("  %s  %s %s\n",
				dot(s.Connected),
				label.Render("  └ "+name),
				value.Render(connStr),
			))
		}
	} else if m.daemonOK && len(m.sources) == 0 {
		// Status file not written yet — daemon just started
		b.WriteString(fmt.Sprintf("  %s  %s %s\n",
			lipgloss.NewStyle().Foreground(colorWarn).Render("◌"),
			label.Render("  └ sources"),
			lipgloss.NewStyle().Foreground(colorSubtext).Render("connecting..."),
		))
	}

	b.WriteString("\n")

	// ── Obsidian sync panel ───────────────────────────────────────────────
	b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("Obsidian Sync"))
	b.WriteString("\n")

	obsStatus := lipgloss.NewStyle().Foreground(colorErr).Render("disabled")
	if m.obsidianOn {
		obsStatus = lipgloss.NewStyle().Foreground(colorSuccess).Render("enabled")
	}
	b.WriteString(fmt.Sprintf("  %s  %s %s\n",
		dot(m.obsidianOn),
		label.Render("Auto-sync"),
		value.Render(obsStatus),
	))

	// Always show vault directory — it clarifies where files land.
	vaultPath := m.obsidianDir
	if vaultPath == "" {
		vaultPath = config.DefaultVaultDir()
	}
	vaultDisplay := filepath.Join(vaultPath, "obsidian")
	b.WriteString(fmt.Sprintf("  %s  %s %s\n",
		lipgloss.NewStyle().Foreground(colorSubtext).Render(" "),
		label.Render("Vault dir"),
		lipgloss.NewStyle().Foreground(colorSubtext).Render(vaultDisplay),
	))

	b.WriteString("\n")

	// ── Action items ─────────────────────────────────────────────────────
	labelStyle := lipgloss.NewStyle().Foreground(colorText).Width(22)
	keyStyle := lipgloss.NewStyle().Foreground(colorMuted).Width(4)

	for i := 0; i < int(watchItemCount); i++ {
		cursor := "  "
		ls := labelStyle
		if i == m.cursor {
			cursor = lipgloss.NewStyle().Foreground(colorAccent).Render("▸ ")
			ls = labelStyle.Copy().Foreground(colorAccent)
		}

		itemLabel := watchMenuLabels[watchMenuItem(i)]
		// Rename dynamic labels based on current state.
		switch watchMenuItem(i) {
		case watchItemToggle:
			if m.daemonOK {
				itemLabel = "Stop daemon"
			} else {
				itemLabel = "Start daemon"
			}
		case watchItemObsidian:
			if m.obsidianOn {
				itemLabel = "Disable Obsidian sync"
			} else {
				itemLabel = "Enable Obsidian sync"
			}
		}

		key := keyStyle.Render("[" + watchMenuKeys[watchMenuItem(i)] + "]")
		b.WriteString(fmt.Sprintf("%s%s %s\n", cursor, ls.Render(itemLabel), key))
	}

	// ── Message ──────────────────────────────────────────────────────────
	if m.message != "" {
		b.WriteString("\n")
		style := lipgloss.NewStyle().Foreground(colorSuccess)
		if m.msgIsErr {
			style = lipgloss.NewStyle().Foreground(colorErr)
		}
		b.WriteString(style.Render(m.message))
		b.WriteString("\n")
	}

	return b.String()
}

func (m WatchModel) FooterHelp() string {
	return "↑↓ navigate • Enter select • s start/stop • b back"
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

func tickWatchStatus() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return watchStatusTick{}
	})
}

type watchStatusTick struct{}

func (m WatchModel) refreshStatus() tea.Cmd {
	return func() tea.Msg {
		cfg, err := config.Load()
		if err != nil {
			return watchStatusMsg{}
		}

		// Check PID file liveness.
		pf, err := daemon.NewPIDFile(cfg.Daemon.PIDFile)
		if err != nil {
			return watchStatusMsg{}
		}
		alive, pid, _ := pf.IsAlive()

		// Read source statuses from the status file written by the daemon's
		// statusLoop — the only cross-process-safe way to get registry state.
		srcs := readStatusFile(cfg.Daemon.StatusFile)

		vaultDir := cfg.Obsidian.VaultDir
		if vaultDir == "" {
			vaultDir = config.DefaultVaultDir()
		}

		return watchStatusMsg{
			daemonOK:    alive,
			ancoraOK:    ancoraSocketAlive(),
			pid:         pid,
			status:      fmt.Sprintf("running (pid %d)", pid),
			sources:     srcs,
			obsidianOn:  cfg.Obsidian.AutoSync,
			obsidianDir: vaultDir,
		}
	}
}

// readStatusFile reads ~/.vela/watch-status.json and converts it to a
// map[string]SourceStatus for the Watch TUI. Returns empty map on any error.
func readStatusFile(path string) map[string]listener.SourceStatus {
	out := make(map[string]listener.SourceStatus)
	if path == "" {
		return out
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}

	// Try new format first: sources as map[string]DaemonSourceStatus.
	var ds types.DaemonStatus
	if err := json.Unmarshal(data, &ds); err == nil && len(ds.Sources) > 0 {
		for name, s := range ds.Sources {
			out[name] = listener.SourceStatus{
				Connected:  s.Connected,
				EventCount: s.EventCount,
			}
		}
		return out
	}

	// Fall back to legacy format: sources as map[string]bool.
	var legacy struct {
		Sources map[string]bool `json:"sources"`
	}
	if err := json.Unmarshal(data, &legacy); err == nil {
		for name, connected := range legacy.Sources {
			out[name] = listener.SourceStatus{Connected: connected}
		}
	}
	return out
}

// ancoraSocketAlive returns true if the ancora IPC socket exists and accepts
// connections — meaning ancora mcp is running right now.
func ancoraSocketAlive() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	sockPath := home + "/.syfra/ancora.sock"
	conn, err := net.DialTimeout("unix", sockPath, 300*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func startDaemonCmd() tea.Cmd {
	return func() tea.Msg {
		// Re-exec self with `watch start --foreground` as a detached child.
		// Same pattern as the CLI `watch start` command — avoids inline goroutine
		// that dies when this TUI process exits.
		self, err := os.Executable()
		if err != nil {
			return watchActionMsg{err: fmt.Errorf("resolving executable: %w", err)}
		}
		child := exec.Command(self, "watch", "start", "--foreground")
		child.Stdout = nil
		child.Stderr = nil
		child.Stdin = nil
		if err := child.Start(); err != nil {
			return watchActionMsg{err: fmt.Errorf("starting daemon: %w", err)}
		}
		_ = child.Process.Release()
		// Brief pause for child to write PID file.
		time.Sleep(300 * time.Millisecond)
		return watchActionMsg{err: nil}
	}
}

func stopDaemonCmd() tea.Cmd {
	return func() tea.Msg {
		d, err := buildDaemon()
		if err != nil {
			return watchActionMsg{err: err}
		}
		return watchActionMsg{err: d.Stop()}
	}
}

func installServiceCmd() tea.Cmd {
	return func() tea.Msg {
		cfg, err := config.Load()
		if err != nil {
			return watchActionMsg{err: err}
		}
		return watchActionMsg{err: daemon.InstallService(cfg.Daemon.PIDFile, cfg.Daemon.LogFile)}
	}
}

// buildDaemon constructs a Daemon for status/control operations. It always
// starts with an empty graph (graph mutations happen inside the running daemon).
func buildDaemon() (*daemon.Daemon, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	g, _ := igraph.Build(nil, nil)
	return daemon.New(cfg, g)
}

// ---------------------------------------------------------------------------
// Obsidian toggle command
// ---------------------------------------------------------------------------

// toggleObsidianCmd writes obsidian.auto_sync to config.yaml and sends SIGHUP
// to the running daemon so it picks up the change without restarting.
func toggleObsidianCmd(enable bool) tea.Cmd {
	return func() tea.Msg {
		if err := setObsidianAutoSync(enable); err != nil {
			return watchActionMsg{err: fmt.Errorf("updating config: %w", err)}
		}
		// Best-effort: notify running daemon via SIGHUP so it hot-reloads.
		_ = sendSIGHUP()
		return watchActionMsg{err: nil}
	}
}

// setObsidianAutoSync reads config.yaml, flips obsidian.auto_sync, and writes
// it back. Uses yaml round-trip so all other keys are preserved.
func setObsidianAutoSync(enable bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	cfgPath := filepath.Join(home, ".vela", "config.yaml")

	// Read raw YAML so we preserve comments/ordering.
	data, err := os.ReadFile(cfgPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading config: %w", err)
	}

	// Unmarshal into generic map to preserve unknown keys.
	var raw map[string]interface{}
	if len(data) > 0 {
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("parsing config: %w", err)
		}
	}
	if raw == nil {
		raw = make(map[string]interface{})
	}

	// Upsert obsidian section.
	obs, _ := raw["obsidian"].(map[string]interface{})
	if obs == nil {
		obs = map[string]interface{}{}
	}
	obs["auto_sync"] = enable
	if _, hasDir := obs["vault_dir"]; !hasDir {
		obs["vault_dir"] = config.DefaultVaultDir()
	}
	raw["obsidian"] = obs

	out, err := yaml.Marshal(raw)
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(cfgPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(cfgPath, out, 0644)
}

// sendSIGHUP finds the running daemon PID and sends SIGHUP to it.
func sendSIGHUP() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pf, err := daemon.NewPIDFile(cfg.Daemon.PIDFile)
	if err != nil {
		return err
	}
	alive, pid, err := pf.IsAlive()
	if err != nil || !alive {
		return nil // daemon not running — no signal needed
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Signal(syscall.SIGHUP)
}

func checkAlive(d *daemon.Daemon) (bool, int, error) {
	cfg, _ := config.Load()
	pf, err := daemon.NewPIDFile(cfg.Daemon.PIDFile)
	if err != nil {
		return false, 0, err
	}
	return pf.IsAlive()
}
