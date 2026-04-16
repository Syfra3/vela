package setup

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// CheckOllama checks if Ollama is installed and running.
func CheckOllama() (installed bool, running bool, path string, err error) {
	// Check if ollama binary exists
	path, err = exec.LookPath("ollama")
	if err != nil {
		return false, false, "", nil // Not installed
	}

	installed = true

	// Check if Ollama server is running
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost:11434/api/tags", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return installed, false, path, nil // Installed but not running
	}
	defer resp.Body.Close()

	running = resp.StatusCode == http.StatusOK
	return installed, running, path, nil
}

// InstallOllama guides installation of Ollama based on platform.
func InstallOllama() error {
	switch runtime.GOOS {
	case "darwin":
		return installOllamaMac()
	case "linux":
		return installOllamaLinux()
	case "windows":
		return fmt.Errorf("automatic Ollama installation not supported on Windows - visit https://ollama.com/download")
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func installOllamaMac() error {
	// Try Homebrew first
	if _, err := exec.LookPath("brew"); err == nil {
		fmt.Println("Installing Ollama via Homebrew...")
		fmt.Println("This may take a few minutes...")
		cmd := exec.Command("brew", "install", "ollama")
		cmd.Stdout = os.Stdout // Show output
		cmd.Stderr = os.Stderr // Show errors
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("brew install ollama failed: %w", err)
		}
		return nil
	}

	// Fallback: curl install script
	fmt.Println("Homebrew not found. Installing Ollama via curl...")
	fmt.Println("You may be prompted for your password (sudo)...")
	fmt.Println("This may take a few minutes...")

	// Download script to temp file first (avoids stdin pipe issue)
	downloadCmd := exec.Command("curl", "-fsSL", "-o", "/tmp/ollama-install.sh", "https://ollama.com/install.sh")
	// Suppress curl download output
	if err := downloadCmd.Run(); err != nil {
		return fmt.Errorf("failed to download install script: %w", err)
	}

	// Execute script with stdin available for sudo
	installCmd := exec.Command("sh", "/tmp/ollama-install.sh")
	installCmd.Stdout = os.Stdout
	installCmd.Stderr = os.Stderr
	installCmd.Stdin = os.Stdin // NOW stdin works for sudo
	if err := installCmd.Run(); err != nil {
		return fmt.Errorf("install script failed: %w", err)
	}

	// Cleanup
	os.Remove("/tmp/ollama-install.sh")
	return nil
}

func installOllamaLinux() error {
	fmt.Println("Installing Ollama via curl...")
	fmt.Println("You may be prompted for your password (sudo)...")
	fmt.Println("This may take a few minutes...")

	// Download script to temp file first (avoids stdin pipe issue)
	downloadCmd := exec.Command("curl", "-fsSL", "-o", "/tmp/ollama-install.sh", "https://ollama.com/install.sh")
	// Suppress curl download output
	if err := downloadCmd.Run(); err != nil {
		return fmt.Errorf("failed to download install script: %w", err)
	}

	// Execute script with stdin available for sudo
	installCmd := exec.Command("sh", "/tmp/ollama-install.sh")
	installCmd.Stdout = os.Stdout
	installCmd.Stderr = os.Stderr
	installCmd.Stdin = os.Stdin // NOW stdin works for sudo
	if err := installCmd.Run(); err != nil {
		return fmt.Errorf("install script failed: %w", err)
	}

	// Cleanup
	os.Remove("/tmp/ollama-install.sh")
	return nil
}

// StartOllama attempts to start the Ollama service.
func StartOllama() error {
	switch runtime.GOOS {
	case "darwin":
		return startOllamaMac()
	case "linux":
		return startOllamaLinux()
	case "windows":
		return fmt.Errorf("automatic Ollama start not supported on Windows - run 'ollama serve' manually")
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func startOllamaMac() error {
	// Try brew services
	if _, err := exec.LookPath("brew"); err == nil {
		cmd := exec.Command("brew", "services", "start", "ollama")
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	// Fallback: launch as daemon
	cmd := exec.Command("ollama", "serve")
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ollama: %w", err)
	}

	// Detach process
	cmd.Process.Release()
	return nil
}

func startOllamaLinux() error {
	// Try systemctl
	if _, err := exec.LookPath("systemctl"); err == nil {
		cmd := exec.Command("systemctl", "start", "ollama")
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	// Fallback: launch as daemon
	cmd := exec.Command("ollama", "serve")
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ollama: %w", err)
	}

	cmd.Process.Release()
	return nil
}

// GetOllamaModels returns list of installed Ollama models.
func GetOllamaModels() ([]string, error) {
	cmd := exec.Command("ollama", "list")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	var models []string

	// Skip header line
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		// Extract model name (first column)
		parts := strings.Fields(line)
		if len(parts) > 0 {
			models = append(models, parts[0])
		}
	}

	return models, nil
}

// PullModel downloads an Ollama model.
func PullModel(model string) error {
	cmd := exec.Command("ollama", "pull", model)
	cmd.Stdout = os.Stdout // Show progress from Ollama
	cmd.Stderr = os.Stderr // Show errors
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to pull model %s: %w", model, err)
	}
	return nil
}

// PullModelSilent downloads an Ollama model without output (for TUI mode).
func PullModelSilent(model string) error {
	cmd := exec.Command("ollama", "pull", model)
	// Suppress all output to avoid breaking TUI layout
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to pull model %s: %w", model, err)
	}
	return nil
}
