package query

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Syfra3/vela/pkg/types"
)

func TestRunRequestSupportsGraphTruthQueryKinds(t *testing.T) {
	t.Parallel()

	engine := loadRequestTestEngine(t)
	tests := []struct {
		name string
		req  types.QueryRequest
		want []string
	}{
		{
			name: "dependencies",
			req:  types.QueryRequest{Kind: types.QueryKindDependencies, Subject: "AuthService", Limit: 5},
			want: []string{"Dependencies for \"AuthService\"", "Database", "UserRepo"},
		},
		{
			name: "reverse dependencies",
			req:  types.QueryRequest{Kind: types.QueryKindReverseDependencies, Subject: "Database", Limit: 5},
			want: []string{"Reverse dependencies for \"Database\"", "AuthService", "UserRepo"},
		},
		{
			name: "impact",
			req:  types.QueryRequest{Kind: types.QueryKindImpact, Subject: "Database", Limit: 5},
			want: []string{"Impact for \"Database\"", "AuthService", "APIHandler"},
		},
		{
			name: "path",
			req:  types.QueryRequest{Kind: types.QueryKindPath, Subject: "APIHandler", Target: "Database"},
			want: []string{"APIHandler", "AuthService", "Database"},
		},
		{
			name: "explain",
			req:  types.QueryRequest{Kind: types.QueryKindExplain, Subject: "AuthService"},
			want: []string{"Edges for \"AuthService\"", "APIHandler", "Database"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.RunRequest(tt.req)
			if err != nil {
				t.Fatalf("RunRequest() error = %v", err)
			}
			for _, want := range tt.want {
				if !strings.Contains(result, want) {
					t.Fatalf("expected %q in result, got:\n%s", want, result)
				}
			}
		})
	}
}

func TestRunRequest_FileQueriesPreferFileDependencyEdges(t *testing.T) {
	t.Parallel()

	graph := map[string]any{
		"nodes": []map[string]any{
			{"id": "project:vela", "label": "vela", "kind": "project", "file": "vela"},
			{"id": "vela:file:cmd/vela/main.go", "label": "cmd/vela/main.go", "kind": "file", "file": "cmd/vela/main.go"},
			{"id": "vela:file:internal/config/config.go", "label": "internal/config/config.go", "kind": "file", "file": "internal/config/config.go"},
			{"id": "vela:file:pkg/types/types.go", "label": "pkg/types/types.go", "kind": "file", "file": "pkg/types/types.go"},
		},
		"edges": []map[string]any{
			{"from": "project:vela", "to": "vela:file:cmd/vela/main.go", "kind": "contains"},
			{"from": "project:vela", "to": "vela:file:internal/config/config.go", "kind": "contains"},
			{"from": "project:vela", "to": "vela:file:pkg/types/types.go", "kind": "contains"},
			{"from": "vela:file:cmd/vela/main.go", "to": "vela:file:internal/config/config.go", "kind": "depends_on"},
			{"from": "vela:file:internal/config/config.go", "to": "vela:file:pkg/types/types.go", "kind": "depends_on"},
		},
	}
	data, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	engine, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile() error = %v", err)
	}

	pathResult, err := engine.RunRequest(types.QueryRequest{Kind: types.QueryKindPath, Subject: "cmd/vela/main.go", Target: "pkg/types/types.go"})
	if err != nil {
		t.Fatalf("RunRequest(path) error = %v", err)
	}
	for _, want := range []string{"cmd/vela/main.go", "internal/config/config.go", "pkg/types/types.go"} {
		if !strings.Contains(pathResult, want) {
			t.Fatalf("expected %q in path result, got %q", want, pathResult)
		}
	}

	reverseResult, err := engine.RunRequest(types.QueryRequest{Kind: types.QueryKindReverseDependencies, Subject: "pkg/types/types.go", Limit: 5})
	if err != nil {
		t.Fatalf("RunRequest(reverse) error = %v", err)
	}
	if !strings.Contains(reverseResult, "internal/config/config.go") {
		t.Fatalf("expected file reverse dependency in result, got %q", reverseResult)
	}
	if strings.Contains(reverseResult, "vela [repo/project]") {
		t.Fatalf("did not expect containment-only reverse dependency result, got %q", reverseResult)
	}
}

func TestRunRequest_PathPrefersTargetPackageBarrelChain(t *testing.T) {
	t.Parallel()

	graph := map[string]any{
		"nodes": []map[string]any{
			{"id": "app:employee-selection", "label": "apps/mobile/src/modules/auth/presentation/screens/EmployeeSelection/hook.ts", "kind": "file", "file": "apps/mobile/src/modules/auth/presentation/screens/EmployeeSelection/hook.ts"},
			{"id": "app:auth-context", "label": "apps/mobile/src/modules/auth/context/AuthContext.tsx", "kind": "file", "file": "apps/mobile/src/modules/auth/context/AuthContext.tsx"},
			{"id": "app:enrollment-service", "label": "apps/mobile/src/modules/auth/domain/EnrollmentService.ts", "kind": "file", "file": "apps/mobile/src/modules/auth/domain/EnrollmentService.ts"},
			{"id": "pkg:index", "label": "packages/api-client/src/index.ts", "kind": "file", "file": "packages/api-client/src/index.ts"},
			{"id": "pkg:hooks", "label": "packages/api-client/src/hooks/index.ts", "kind": "file", "file": "packages/api-client/src/hooks/index.ts"},
			{"id": "pkg:users-index", "label": "packages/api-client/src/hooks/users/index.ts", "kind": "file", "file": "packages/api-client/src/hooks/users/index.ts"},
			{"id": "pkg:users", "label": "packages/api-client/src/hooks/users/use-users.ts", "kind": "file", "file": "packages/api-client/src/hooks/users/use-users.ts"},
		},
		"edges": []map[string]any{
			{"from": "app:employee-selection", "to": "app:auth-context", "kind": "depends_on", "metadata": map[string]any{"projected_from": "static_import"}},
			{"from": "app:auth-context", "to": "app:enrollment-service", "kind": "depends_on", "metadata": map[string]any{"projected_from": "static_import"}},
			{"from": "app:enrollment-service", "to": "pkg:users", "kind": "depends_on", "metadata": map[string]any{"projected_from": "static_import"}},
			{"from": "app:employee-selection", "to": "pkg:index", "kind": "depends_on", "metadata": map[string]any{"projected_from": "workspace_package"}},
			{"from": "pkg:index", "to": "pkg:hooks", "kind": "depends_on", "metadata": map[string]any{"projected_from": "static_import"}},
			{"from": "pkg:hooks", "to": "pkg:users-index", "kind": "depends_on", "metadata": map[string]any{"projected_from": "static_import"}},
			{"from": "pkg:users-index", "to": "pkg:users", "kind": "depends_on", "metadata": map[string]any{"projected_from": "static_import"}},
		},
	}
	data, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	engine, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile() error = %v", err)
	}

	pathResult, err := engine.RunRequest(types.QueryRequest{Kind: types.QueryKindPath, Subject: "apps/mobile/src/modules/auth/presentation/screens/EmployeeSelection/hook.ts", Target: "packages/api-client/src/hooks/users/use-users.ts"})
	if err != nil {
		t.Fatalf("RunRequest(path) error = %v", err)
	}
	for _, want := range []string{
		"packages/api-client/src/index.ts",
		"packages/api-client/src/hooks/index.ts",
		"packages/api-client/src/hooks/users/index.ts",
	} {
		if !strings.Contains(pathResult, want) {
			t.Fatalf("expected %q in ranked path result, got %q", want, pathResult)
		}
	}
	for _, unwanted := range []string{
		"apps/mobile/src/modules/auth/context/AuthContext.tsx",
		"apps/mobile/src/modules/auth/domain/EnrollmentService.ts",
	} {
		if strings.Contains(pathResult, unwanted) {
			t.Fatalf("did not expect %q in ranked path result, got %q", unwanted, pathResult)
		}
	}
}

func TestRunRequest_PathPrefersExplanatoryIntermediateOverDirectJump(t *testing.T) {
	t.Parallel()

	graph := map[string]any{
		"nodes": []map[string]any{
			{"id": "main", "label": "cmd/vela/main.go", "kind": "file", "file": "cmd/vela/main.go"},
			{"id": "config", "label": "internal/config/config.go", "kind": "file", "file": "internal/config/config.go"},
			{"id": "types", "label": "pkg/types/types.go", "kind": "file", "file": "pkg/types/types.go"},
		},
		"edges": []map[string]any{
			{"from": "main", "to": "types", "kind": "depends_on", "metadata": map[string]any{"projected_from": "static_import"}},
			{"from": "main", "to": "config", "kind": "depends_on", "metadata": map[string]any{"projected_from": "static_import"}},
			{"from": "config", "to": "types", "kind": "depends_on", "metadata": map[string]any{"projected_from": "static_import"}},
		},
	}
	data, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	engine, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile() error = %v", err)
	}

	pathResult, err := engine.RunRequest(types.QueryRequest{Kind: types.QueryKindPath, Subject: "cmd/vela/main.go", Target: "pkg/types/types.go"})
	if err != nil {
		t.Fatalf("RunRequest(path) error = %v", err)
	}
	if !strings.Contains(pathResult, "internal/config/config.go") {
		t.Fatalf("expected config intermediary in ranked path result, got %q", pathResult)
	}
}

func TestRunRequest_ReverseDependenciesPreferExternalPackageBarrelCallers(t *testing.T) {
	t.Parallel()

	graph := map[string]any{
		"nodes": []map[string]any{
			{"id": "pkg:index", "label": "packages/api-client/src/index.ts", "kind": "file", "file": "packages/api-client/src/index.ts"},
			{"id": "pkg:hooks", "label": "packages/api-client/src/hooks/index.ts", "kind": "file", "file": "packages/api-client/src/hooks/index.ts"},
			{"id": "app:employee-selection", "label": "apps/mobile/src/modules/auth/presentation/screens/EmployeeSelection/hook.ts", "kind": "file", "file": "apps/mobile/src/modules/auth/presentation/screens/EmployeeSelection/hook.ts"},
			{"id": "app:auth-context", "label": "apps/mobile/src/modules/auth/presentation/context/AuthContext.tsx", "kind": "file", "file": "apps/mobile/src/modules/auth/presentation/context/AuthContext.tsx"},
		},
		"edges": []map[string]any{
			{"from": "pkg:hooks", "to": "pkg:index", "kind": "depends_on", "metadata": map[string]any{"projected_from": "static_import"}},
			{"from": "app:employee-selection", "to": "pkg:index", "kind": "depends_on", "metadata": map[string]any{"projected_from": "static_import"}},
			{"from": "app:auth-context", "to": "pkg:index", "kind": "depends_on", "metadata": map[string]any{"projected_from": "static_import"}},
		},
	}
	data, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	engine, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile() error = %v", err)
	}

	reverseResult, err := engine.RunRequest(types.QueryRequest{Kind: types.QueryKindReverseDependencies, Subject: "packages/api-client/src/index.ts", Limit: 5})
	if err != nil {
		t.Fatalf("RunRequest(reverse) error = %v", err)
	}
	for _, want := range []string{
		"apps/mobile/src/modules/auth/presentation/screens/EmployeeSelection/hook.ts",
		"apps/mobile/src/modules/auth/presentation/context/AuthContext.tsx",
	} {
		if !strings.Contains(reverseResult, want) {
			t.Fatalf("expected %q in reverse dependency result, got %q", want, reverseResult)
		}
	}
	if strings.Contains(reverseResult, "packages/api-client/src/hooks/index.ts") {
		t.Fatalf("did not expect internal package barrel caller in result, got %q", reverseResult)
	}
}

func loadRequestTestEngine(t *testing.T) *Engine {
	t.Helper()
	graph := map[string]any{
		"nodes": []map[string]any{
			{"id": "api", "label": "APIHandler", "kind": "handler", "file": "api.go"},
			{"id": "auth", "label": "AuthService", "kind": "struct", "file": "auth.go"},
			{"id": "db", "label": "Database", "kind": "struct", "file": "db.go"},
			{"id": "user", "label": "UserRepo", "kind": "struct", "file": "user.go"},
		},
		"edges": []map[string]any{
			{"from": "api", "to": "auth", "kind": "calls"},
			{"from": "auth", "to": "db", "kind": "depends_on"},
			{"from": "auth", "to": "user", "kind": "depends_on"},
			{"from": "user", "to": "db", "kind": "depends_on"},
		},
	}
	data, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	engine, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile() error = %v", err)
	}
	return engine
}
