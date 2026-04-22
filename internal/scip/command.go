package scip

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type MissingBinaryError struct {
	Driver      string
	Command     string
	RepoRoot    string
	InstallHint string
	Cause       error
}

func (e *MissingBinaryError) Error() string {
	if e == nil {
		return "SCIP driver binary is missing"
	}
	message := strings.TrimSpace(e.Command)
	if message == "" {
		message = strings.TrimSpace(e.Driver)
	}
	if message == "" {
		message = "SCIP driver"
	}
	message += " is not installed"
	if hint := strings.TrimSpace(e.InstallHint); hint != "" {
		message += ". Install it with: " + hint
	}
	if repo := strings.TrimSpace(e.RepoRoot); repo != "" {
		message += " (repo: " + repo + ")"
	}
	return message
}

func (e *MissingBinaryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

type Runner interface {
	Run(ctx context.Context, dir string, name string, args ...string) error
}

type execRunner struct{}

type commandRunError struct {
	Err    error
	Output string
}

func (e *commandRunError) Error() string {
	if e == nil {
		return "command failed"
	}
	message := "command failed"
	if e.Err != nil {
		message = e.Err.Error()
	}
	if summary := summarizeCommandOutput(e.Output); summary != "" {
		message += ": " + summary
	}
	return message
}

func (e *commandRunError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (execRunner) Run(ctx context.Context, dir string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		return &commandRunError{Err: err, Output: output.String()}
	}
	return nil
}

type CommandDriver struct {
	name       string
	language   string
	command    string
	supports   []string
	runner     Runner
	lookPath   func(string) (string, error)
	argBuilder func(Request) []string
}

func DefaultRegistry() (*Registry, error) {
	return NewRegistry(
		NewGoDriver(nil),
		NewTypeScriptDriver(nil),
		NewPythonDriver(nil),
	)
}

func NewGoDriver(runner Runner) Driver {
	return newCommandDriver("scip-go", "go", []string{"go.mod"}, runner, func(req Request) []string {
		return []string{"--project-root", req.RepoRoot, "--module-root", req.RepoRoot, "--output", req.OutputPath}
	})
}

func NewTypeScriptDriver(runner Runner) Driver {
	return newCommandDriver("scip-typescript", "typescript", []string{"tsconfig.json", "package.json"}, runner, func(req Request) []string {
		return []string{"index", "--cwd", req.RepoRoot, "--output", req.OutputPath}
	})
}

func NewPythonDriver(runner Runner) Driver {
	return newCommandDriver("scip-python", "python", []string{"pyproject.toml", "requirements.txt", "setup.py"}, runner, func(req Request) []string {
		return []string{"index", "--project-root", req.RepoRoot, "--output", req.OutputPath}
	})
}

func newCommandDriver(name, language string, supports []string, runner Runner, argBuilder func(Request) []string) *CommandDriver {
	if runner == nil {
		runner = execRunner{}
	}
	return &CommandDriver{name: name, language: language, command: name, supports: supports, runner: runner, lookPath: exec.LookPath, argBuilder: argBuilder}
}

func (d *CommandDriver) Name() string { return d.name }

func (d *CommandDriver) Language() string { return d.language }

func (d *CommandDriver) Supports(repoRoot string) bool {
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		return false
	}
	for _, candidate := range d.supports {
		if fileExists(filepath.Join(repoRoot, candidate)) {
			return true
		}
	}
	return false
}

func (d *CommandDriver) Bootstrap(ctx context.Context, req Request) error {
	req = req.Normalize()
	if d == nil {
		return fmt.Errorf("scip driver is nil")
	}
	if strings.TrimSpace(req.RepoRoot) == "" {
		return errors.New("scip request repo root is required")
	}
	if _, err := d.lookPath(d.command); err == nil {
		return nil
	} else if !isMissingBinary(err) {
		return fmt.Errorf("locate %s: %w", d.name, err)
	}
	installer, args := installCommand(d.command)
	if strings.TrimSpace(installer) == "" {
		return &MissingBinaryError{Driver: d.name, Command: d.command, RepoRoot: req.RepoRoot, InstallHint: installHint(d.command)}
	}
	if err := d.runner.Run(ctx, req.RepoRoot, installer, args...); err != nil {
		return fmt.Errorf("install %s: %w", d.name, err)
	}
	if _, err := d.lookPath(d.command); err != nil {
		return &MissingBinaryError{Driver: d.name, Command: d.command, RepoRoot: req.RepoRoot, InstallHint: installHint(d.command), Cause: err}
	}
	return nil
}

func (d *CommandDriver) Index(ctx context.Context, req Request) (Result, error) {
	req = req.Normalize()
	if err := req.Validate(); err != nil {
		return Result{}, err
	}
	if d == nil {
		return Result{}, fmt.Errorf("scip driver is nil")
	}
	if err := os.MkdirAll(filepath.Dir(req.OutputPath), 0o755); err != nil {
		return Result{}, fmt.Errorf("create scip output dir: %w", err)
	}
	if err := d.runner.Run(ctx, req.RepoRoot, d.command, d.argBuilder(req)...); err != nil {
		if isMissingBinary(err) {
			return Result{}, &MissingBinaryError{Driver: d.name, Command: d.command, RepoRoot: req.RepoRoot, InstallHint: installHint(d.command), Cause: err}
		}
		return Result{}, fmt.Errorf("run %s: %w", d.name, err)
	}
	return Result{Driver: d.name, Language: d.language, Artifact: req.OutputPath}, nil
}

func isMissingBinary(err error) bool {
	return errors.Is(err, exec.ErrNotFound)
}

func installHint(command string) string {
	installer, args := installCommand(command)
	if strings.TrimSpace(installer) != "" {
		return strings.TrimSpace(strings.Join(append([]string{installer}, args...), " "))
	}
	switch strings.TrimSpace(command) {
	case "scip-typescript":
		return "npm install -g @sourcegraph/scip-typescript"
	case "scip-go":
		return "go install github.com/sourcegraph/scip-go/cmd/scip-go@latest"
	case "scip-python":
		return "pipx install scip-python"
	default:
		return "install " + strings.TrimSpace(command) + " and retry"
	}
}

func installCommand(command string) (string, []string) {
	switch strings.TrimSpace(command) {
	case "scip-typescript":
		return "npm", []string{"install", "-g", "@sourcegraph/scip-typescript"}
	case "scip-go":
		return "go", []string{"install", "github.com/sourcegraph/scip-go/cmd/scip-go@latest"}
	case "scip-python":
		return "pipx", []string{"install", "scip-python"}
	default:
		return "", nil
	}
}

func summarizeCommandOutput(output string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "panic:") {
			return trimmed
		}
	}
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
