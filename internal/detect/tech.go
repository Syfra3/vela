package detect

import (
	"os"
	"path/filepath"
)

// Tech represents a detected technology stack in a directory.
type Tech int

const (
	TechUnknown Tech = iota
	TechNode         // package.json present
	TechGo           // go.mod present
	TechPython       // pyproject.toml | setup.py | requirements.txt
	TechRust         // Cargo.toml
	TechJava         // pom.xml | build.gradle
)

// String returns human-readable tech name.
func (t Tech) String() string {
	switch t {
	case TechNode:
		return "node"
	case TechGo:
		return "go"
	case TechPython:
		return "python"
	case TechRust:
		return "rust"
	case TechJava:
		return "java"
	default:
		return "unknown"
	}
}

// DetectTech returns the primary tech stack detected in dir by checking
// for well-known manifest files. Only checks the given directory — not recursive.
func DetectTech(dir string) Tech {
	probes := []struct {
		file string
		tech Tech
	}{
		{"package.json", TechNode},
		{"go.mod", TechGo},
		{"Cargo.toml", TechRust},
		{"pom.xml", TechJava},
		{"build.gradle", TechJava},
		{"build.gradle.kts", TechJava},
		{"pyproject.toml", TechPython},
		{"setup.py", TechPython},
		{"requirements.txt", TechPython},
	}
	for _, p := range probes {
		if fileExists(filepath.Join(dir, p.file)) {
			return p.tech
		}
	}
	return TechUnknown
}

// DefaultIgnorePatterns returns the built-in ignore patterns for a tech stack.
// These cover generated artefacts, build outputs, and dependency directories
// that are never useful for knowledge-graph extraction.
func DefaultIgnorePatterns(t Tech) []string {
	generic := []string{
		".git/",
		".svn/",
		".hg/",
		".DS_Store",
		"Thumbs.db",
		"*.log",
		"*.lock", // lockfiles: not useful for graph
		"coverage/",
		".nyc_output/",
		".cache/",
		"*.tmp",
		"*.temp",
	}

	switch t {
	case TechNode:
		return append(generic,
			// dependencies
			"node_modules/",
			// build outputs — framework-specific
			"dist/",
			"build/",
			"out/",
			".next/",
			".nuxt/",
			".svelte-kit/",
			".turbo/",
			".vercel/",
			".netlify/",
			// angular
			".angular/",
			// generated type declarations from tsc
			"*.d.ts",
			// compiled JS next to TS source (common in libs)
			"*.js.map",
			// vite / webpack chunks
			"*.chunk.js",
			// storybook
			"storybook-static/",
		)

	case TechGo:
		return append(generic,
			"vendor/",
			"bin/",
			// compiled output placed next to source (rare but happens)
			"*.test",
		)

	case TechPython:
		return append(generic,
			"__pycache__/",
			"*.pyc",
			"*.pyo",
			"*.pyd",
			".venv/",
			"venv/",
			"env/",
			".env/",
			"*.egg-info/",
			"dist/",
			"build/",
			"*.dist-info/",
			".mypy_cache/",
			".pytest_cache/",
			".ruff_cache/",
			"htmlcov/",
			"site/", // mkdocs build
		)

	case TechRust:
		return append(generic,
			"target/",
		)

	case TechJava:
		return append(generic,
			"target/", // maven
			"build/",  // gradle
			".gradle/",
			"out/", // IntelliJ
			"*.class",
			"*.jar",
			"*.war",
			"*.ear",
		)

	default:
		return generic
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
