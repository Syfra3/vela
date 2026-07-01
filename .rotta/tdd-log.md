# Rotta TDD Log

## Vela Clustering Installation Support

- Status: ready for implementation.
- Approved spec: `specs/hard_spec.md`
- Approved feature: `features/vela_install_clustering.feature`

## SCN-001 — Install baseline clustering into a new repo-local virtual environment

- Requirements: `REQ-001`, `REQ-006`
- Test: `TestSCN001_InstallClusteringCreatesRepoLocalVenvAndVerifiesNetworkx`
- Scenario source: `features/vela_install_clustering.feature` lines 6-13

### RED

- Added the smallest command-level test for `vela install --clustering` with no existing repo-local `.venv`.
- Failing command: `go test ./cmd/vela -run TestSCN001_InstallClusteringCreatesRepoLocalVenvAndVerifiesNetworkx -count=1`
- Failure evidence:
  - `undefined: runClusteringInstallCommand`
  - `undefined: verifyClusteringNetworkX`
  - package `github.com/Syfra3/vela/cmd/vela` failed to build.

### GREEN

- Added the `--clustering` flag for `vela install`.
- Added minimal clustering install flow for this scenario only:
  - create repo-local `.venv` with `python3 -m venv` when it does not exist;
  - install from repo-local `requirements-clustering.txt` with `.venv/bin/pip install -r`;
  - verify `networkx` imports using `.venv/bin/python3`;
  - report clustering installation success after install and verification pass.
- Passing focused command: `go test ./cmd/vela -run TestSCN001_InstallClusteringCreatesRepoLocalVenvAndVerifiesNetworkx -count=1`
- Passing full suite: `go test ./...`

### REFACTOR

- No additional refactor beyond `gofmt`; implementation kept intentionally minimal to avoid SCN-002+ permission, repair, cache-warning, optional-Leiden, and verification-failure behavior.
- Passing full suite after formatting: `go test ./...`

### Files Changed

- `cmd/vela/main_test.go`
- `cmd/vela/cutover.go`
- `.rotta/tdd-log.md`

## SCN-011 — Permission-denied venv creation is translated into an actionable Vela error

- Requirements: `REQ-002`, `REQ-006`
- Test: `TestSCN011_InstallClusteringTranslatesPermissionDeniedVenvPreparationToActionableError`
- Scenario source: `features/vela_install_clustering.feature` lines 97-104

### RED

- Added the smallest command-level test for `vela install --clustering` with an existing repo-local `.venv` containing a read-only activation file.
- The test proves the command fails before dependency installation, identifies the repo-local `.venv` and blocked activation file, preserves permission-denied detail, recommends explicit `--repair-venv`, and does not claim installation success or silently mention broad `chmod`/`chown` repair.
- Failing command: `go test ./cmd/vela -run TestSCN011_InstallClusteringTranslatesPermissionDeniedVenvPreparationToActionableError -count=1`
- Failure evidence:
  - `expected error/output to contain "permission denied"`
  - The existing error identified the blocked activation file and repair guidance but did not preserve permission-denied detail for the user-facing Vela error.

### GREEN

- Added minimal permission-denied detail to the existing read-only repo-local `.venv` writable preflight error while preserving the SCN-003 behavior of failing before dependency installation and without automatic repair.
- Passing focused command: `go test ./cmd/vela -run TestSCN011_InstallClusteringTranslatesPermissionDeniedVenvPreparationToActionableError -count=1`
- Passing SCN-001 through SCN-011 command: `go test ./cmd/vela -run 'TestSCN0(0[1-9]|1[01])_InstallClustering' -count=1`
- Passing full suite: `go test ./... -count=1`

### REFACTOR

- Ran `gofmt` on touched Go files.
- No additional refactor; implementation stayed limited to SCN-011 permission-denied diagnostic detail and did not add SCN-012 networkx verification failure behavior.
- Passing full suite after formatting/log update: `go test ./... -count=1`

### Files Changed

- `cmd/vela/main_test.go`
- `cmd/vela/cutover.go`
- `.rotta/tdd-log.md`

## SCN-012 — Networkx verification failure prevents partial success

- Requirements: `REQ-001`, `REQ-006`
- Test: `TestSCN012_InstallClusteringNetworkxVerificationFailurePreventsPartialSuccess`
- Scenario source: `features/vela_install_clustering.feature` lines 106-113

### RED

- Baseline pre-check: `go test ./... -count=1` passed before changing SCN-012 code/tests. The working tree already contained uncommitted SCN-001 through SCN-011 and other prior scenario changes, as instructed by the orchestrator.
- Added the smallest command-level test for `vela install --clustering` where dependency installation from `requirements-clustering.txt` completes but `networkx` verification fails in the selected repo-local `.venv`.
- The test proves dependency installation happens before verification failure, the command exits with failure, the error reports clustering verification failure with the underlying import failure, and success is not reported.
- Failing command: `go test ./cmd/vela -run TestSCN012_InstallClusteringNetworkxVerificationFailurePreventsPartialSuccess -count=1`
- Failure evidence:
  - `expected error/output to contain "clustering verification failed"`
  - Actual error was `verify networkx from repo-local .venv: synthetic networkx import failure` and no success message was emitted.

### GREEN

- Added minimal user-facing verification-failure categorization around the existing `networkx` verification error: `clustering verification failed: verify networkx from repo-local .venv`.
- Preserved existing behavior that dependency install output remains visible after install completes, and `clustering installation succeeded` is only printed after verification passes.
- Passing focused command: `go test ./cmd/vela -run TestSCN012_InstallClusteringNetworkxVerificationFailurePreventsPartialSuccess -count=1`
- Passing SCN-001 through SCN-012 command: `go test ./cmd/vela -run 'TestSCN0(0[1-9]|1[0-2])_InstallClustering' -count=1`
- Passing full suite: `go test ./... -count=1`

### REFACTOR

- Ran `gofmt` on touched Go files.
- No additional refactor; implementation stayed limited to SCN-012 verification-failure diagnostics and did not add behavior beyond preventing partial success.
- Passing full suite after formatting/log update: `go test ./... -count=1`

### Files Changed

- `cmd/vela/main_test.go`
- `cmd/vela/cutover.go`
- `.rotta/tdd-log.md`

## SCN-005 — Explicit repair mode reports unrecoverable permission failures

- Requirements: `REQ-003`, `REQ-006`
- Test: `TestSCN005_InstallClusteringRepairVenvReportsUnrecoverablePermissionFailure`
- Scenario source: `features/vela_install_clustering.feature` lines 45-52

### RED

- Added the smallest command-level test for `vela install --clustering --repair-venv` where repairing a read-only repo-local `.venv` path returns an unrecoverable permission error.
- Failing command: `go test ./cmd/vela -run TestSCN005_InstallClusteringRepairVenvReportsUnrecoverablePermissionFailure -count=1`
- Failure evidence:
  - `undefined: chmodClusteringVenvPath`
  - package `github.com/Syfra3/vela/cmd/vela` failed to build.
  - This proved the repair operation was not yet observable enough to force an unrecoverable permission failure before dependency installation.

### GREEN

- Added a minimal injectable chmod wrapper used only by repo-local `.venv` repair.
- Preserved underlying permission failure details by wrapping chmod failures with the failing path before returning `repair repo-local .venv writability`.
- Ensured unrecoverable repair failure stops before dependency installation, `networkx` verification, and clustering success output.
- Passing focused command: `go test ./cmd/vela -run TestSCN005_InstallClusteringRepairVenvReportsUnrecoverablePermissionFailure -count=1`
- Passing SCN-001 through SCN-005 command: `go test ./cmd/vela -run 'TestSCN00[1-5]_InstallClustering' -count=1`
- Passing full suite: `go test ./... -count=1`

### REFACTOR

- Ran `gofmt` on touched Go files.
- No additional refactor; implementation stayed limited to unrecoverable repair error reporting for SCN-005 and did not add SCN-006+ optional dependency or graph-cache behavior.
- Passing full suite after formatting: `go test ./... -count=1`

### Files Changed

- `cmd/vela/main_test.go`
- `cmd/vela/cutover.go`
- `.rotta/tdd-log.md`

## SCN-006 — Baseline clustering install does not install optional Leiden dependencies

- Requirements: `REQ-004`
- Test: `TestSCN006_InstallClusteringKeepsOptionalLeidenSeparateByDefault`
- Scenario source: `features/vela_install_clustering.feature` lines 54-60

### RED

- Added the smallest command-level test for `vela install --clustering` proving baseline install uses only `requirements-clustering.txt`, does not invoke `graspologic` or `requirements-clustering-leiden.txt`, and reports optional Leiden support as separate.
- Failing command: `go test ./cmd/vela -run TestSCN006_InstallClusteringKeepsOptionalLeidenSeparateByDefault -count=1`
- Failure evidence:
  - `expected install output to contain "optional Leiden support remains separate from baseline clustering support"`
  - Actual output reported venv creation, baseline requirements install, `networkx` verification, and success only.

### GREEN

- Added minimal success output after baseline requirements installation to distinguish optional Leiden support from baseline clustering support.
- Preserved the baseline install command sequence: `python3 -m venv .venv` when needed, then `.venv/bin/pip install -r requirements-clustering.txt`; no `graspologic` or Leiden requirements install was added.
- Passing focused command: `go test ./cmd/vela -run TestSCN006_InstallClusteringKeepsOptionalLeidenSeparateByDefault -count=1`
- Passing SCN-001 through SCN-006 command: `go test ./cmd/vela -run 'TestSCN00[1-6]_InstallClustering' -count=1`
- Passing full suite: `go test ./... -count=1`

### REFACTOR

- Ran `gofmt` on touched Go files.
- No additional refactor; implementation stayed limited to SCN-006 optional dependency boundary messaging and did not add SCN-007 optional dependency preservation or SCN-008+ graph-cache behavior.
- Passing full suite after formatting: `go test ./... -count=1`

### Files Changed

- `cmd/vela/main_test.go`
- `cmd/vela/cutover.go`
- `.rotta/tdd-log.md`

## SCN-007 — Existing optional Leiden dependencies are preserved

- Requirements: `REQ-004`
- Test: `TestSCN007_InstallClusteringPreservesExistingOptionalLeidenDependencies`
- Scenario source: `features/vela_install_clustering.feature` lines 62-68

### RED

- Added the smallest command-level test for `vela install --clustering` with optional Leiden/graspologic already present in the selected repo-local `.venv`.
- Failing command: `go test ./cmd/vela -run TestSCN007_InstallClusteringPreservesExistingOptionalLeidenDependencies -count=1`
- Failure evidence:
  - `undefined: optionalLeidenDependencyVersion`
  - package `github.com/Syfra3/vela/cmd/vela` failed to build.
  - This proved the install flow had no observable before/after optional Leiden dependency preservation check.

### GREEN

- Added a minimal optional Leiden dependency version probe around the baseline requirements install.
- When optional Leiden dependencies are present before install, the flow now verifies the same version remains after baseline install and reports that existing optional Leiden dependencies were preserved and not downgraded.
- Preserved the baseline install command sequence: only `.venv/bin/pip install -r requirements-clustering.txt`; no `graspologic`, uninstall, force-reinstall, or Leiden requirements install command was added.
- Passing focused command: `go test ./cmd/vela -run TestSCN007_InstallClusteringPreservesExistingOptionalLeidenDependencies -count=1`
- Passing SCN-001 through SCN-007 command: `go test ./cmd/vela -run 'TestSCN00[1-7]_InstallClustering' -count=1`
- Passing full suite: `go test ./... -count=1`

### REFACTOR

- Ran `gofmt` on touched Go files.
- No additional refactor; implementation stayed limited to SCN-007 optional dependency preservation and did not add SCN-008+ graph-cache warning/rebuild behavior.
- Passing full suite after formatting: `go test ./... -count=1`

### Files Changed

- `cmd/vela/main_test.go`
- `cmd/vela/cutover.go`
- `.rotta/tdd-log.md`

## SCN-008 — Successful install warns that an existing graph cache may lack community metadata

- Requirements: `REQ-005`, `REQ-006`
- Test: `TestSCN008_InstallClusteringWarnsExistingGraphCacheMayLackCommunityMetadata`
- Scenario source: `features/vela_install_clustering.feature` lines 70-77

### RED

- Added the smallest command-level test for `vela install --clustering` when an existing `.vela/graph.db` cache artifact is present before successful clustering installation.
- The test proves install success remains successful, warns that the existing graph cache may have been built without community metadata, recommends a non-destructive rebuild command, and preserves the graph cache artifact content.
- Failing command: `go test ./cmd/vela -run TestSCN008_InstallClusteringWarnsExistingGraphCacheMayLackCommunityMetadata -count=1`
- Failure evidence:
  - `expected install output to contain "existing graph cache may have been built without community metadata"`
  - Actual output reported venv creation, baseline requirements install, optional Leiden separation, `networkx` verification, and install success only.

### GREEN

- Added minimal graph-cache detection after successful dependency install and `networkx` verification.
- When `.vela/graph.db` exists, install now warns that existing graph cache may have been built without community metadata and recommends `vela build <project>` as a non-destructive graph metadata rebuild command.
- No cache deletion, purge, removal, force rebuild, or rebuild execution behavior was added for this scenario.
- Passing focused command: `go test ./cmd/vela -run TestSCN008_InstallClusteringWarnsExistingGraphCacheMayLackCommunityMetadata -count=1`
- Passing SCN-001 through SCN-008 command: `go test ./cmd/vela -run 'TestSCN00[1-8]_InstallClustering' -count=1`
- Passing full suite: `go test ./... -count=1`

### REFACTOR

- Ran `gofmt` on touched Go files.
- Extracted graph-cache existence detection into `graphCacheExists` to keep the install flow readable while avoiding SCN-009+ no-cache messaging and SCN-010 force-rebuild behavior.
- Passing full suite after formatting: `go test ./... -count=1`

### Files Changed

- `cmd/vela/main_test.go`
- `cmd/vela/cutover.go`
- `.rotta/tdd-log.md`

## SCN-004 — Explicit repair mode makes the repo-local virtual environment writable before installing

- Requirements: `REQ-003`, `REQ-006`
- Test: `TestSCN004_InstallClusteringRepairVenvMakesRepoLocalVenvWritableBeforeInstalling`
- Scenario source: `features/vela_install_clustering.feature` lines 34-43

### RED

- Added the smallest command-level test for `vela install --clustering --repair-venv` with an existing repo-local `.venv` containing a read-only activation file.
- Failing command: `go test ./cmd/vela -run TestSCN004_InstallClusteringRepairVenvMakesRepoLocalVenvWritableBeforeInstalling -count=1`
- Failure evidence:
  - `Execute(install --clustering --repair-venv) error = unknown flag: --repair-venv`
  - This proved explicit repair mode was not yet exposed before dependency installation could continue.

### GREEN

- Added the `--repair-venv` flag for `vela install --clustering`.
- Added minimal repair behavior for existing repo-local `.venv` paths only: walk the selected `.venv`, add user write permission where missing, report repair, then run the existing writable preflight, dependency install, `networkx` verification, and success output.
- Passing focused command: `go test ./cmd/vela -run TestSCN004_InstallClusteringRepairVenvMakesRepoLocalVenvWritableBeforeInstalling -count=1`
- Passing SCN-001/SCN-002/SCN-003/SCN-004 command: `go test ./cmd/vela -run 'TestSCN00[1-4]_InstallClustering' -count=1`
- Passing full suite: `go test ./... -count=1`

### REFACTOR

- Ran `gofmt` on touched Go files.
- Extracted repair behavior into `repairRepoLocalVenvWritability` to keep the install flow readable while avoiding SCN-005+ unrecoverable repair reporting and SCN-008+ graph-cache behavior.
- Passing full suite after formatting: `go test ./... -count=1`

### Files Changed

- `cmd/vela/main_test.go`
- `cmd/vela/cutover.go`
- `.rotta/tdd-log.md`

## SCN-002 — Reuse an existing writable repo-local virtual environment

- Requirements: `REQ-001`, `REQ-006`
- Test: `TestSCN002_InstallClusteringReusesExistingWritableRepoLocalVenv`
- Scenario source: `features/vela_install_clustering.feature` lines 15-22

### RED

- Added the smallest command-level test for `vela install --clustering` with an existing writable repo-local `.venv`.
- Failing command: `go test ./cmd/vela -run TestSCN002_InstallClusteringReusesExistingWritableRepoLocalVenv -count=1`
- Failure evidence:
  - `expected install output to contain "using existing repo-local .venv"`
  - Actual output only reported dependency installation, `networkx` verification, and success.

### GREEN

- Added minimal existing-venv selection output when repo-local `.venv` already exists.
- Preserved SCN-001 behavior: no new venv is created for SCN-002; dependency install still uses `.venv/bin/pip`; verification still uses `.venv/bin/python3`.
- Passing focused command: `go test ./cmd/vela -run TestSCN002_InstallClusteringReusesExistingWritableRepoLocalVenv -count=1`
- Passing SCN-001/SCN-002 command: `go test ./cmd/vela -run 'TestSCN00[12]_InstallClustering' -count=1`
- Passing full suite: `go test ./...`

### REFACTOR

- Ran `gofmt` on touched Go files.
- No additional refactor; implementation kept intentionally minimal to avoid SCN-003+ permission preflight/repair, cache-warning, optional-Leiden, force-rebuild, or verification-failure behavior.
- Passing full suite after formatting: `go test ./...`

### Files Changed

- `cmd/vela/main_test.go`
- `cmd/vela/cutover.go`
- `.rotta/tdd-log.md`

## SCN-003 — Fail clearly when the existing virtual environment is not writable

- Requirements: `REQ-002`, `REQ-006`
- Test: `TestSCN003_InstallClusteringFailsWhenExistingRepoLocalVenvIsNotWritable`
- Scenario source: `features/vela_install_clustering.feature` lines 24-32

### RED

- Added the smallest command-level test for `vela install --clustering` with an existing repo-local `.venv` containing a read-only activation file.
- Failing command: `go test ./cmd/vela -run TestSCN003_InstallClusteringFailsWhenExistingRepoLocalVenvIsNotWritable -count=1`
- Failure evidence:
  - `verifyClusteringNetworkX("/tmp/TestSCN003_InstallClusteringFailsWhenExistingRepoLocalVenvIsNotW1906966043/001/.venv/bin/python3") called before writable preflight passed`
  - This proved the command attempted dependency/verification flow instead of failing before dependency installation.

### GREEN

- Added a minimal writable preflight for existing repo-local `.venv` paths before dependency installation.
- The preflight walks the selected `.venv` and fails if a directory or file lacks user write permission; the error identifies `repo-local .venv`, includes the blocked path, and recommends `--repair-venv` or documented manual remediation.
- No repair behavior, `chmod`, or `chown` was added for this scenario.
- Passing focused command: `go test ./cmd/vela -run TestSCN003_InstallClusteringFailsWhenExistingRepoLocalVenvIsNotWritable -count=1`
- Passing SCN-001/SCN-002/SCN-003 command: `go test ./cmd/vela -run 'TestSCN00[123]_InstallClustering' -count=1`
- Passing full suite: `go test ./...`

### REFACTOR

- Ran `gofmt` on touched Go files.
- Extracted the preflight into `ensureRepoLocalVenvWritable` to keep the install flow readable while avoiding SCN-004+ repair behavior.
- Passing full suite after formatting: `go test ./...`

### Files Changed

- `cmd/vela/main_test.go`
- `cmd/vela/cutover.go`
- `.rotta/tdd-log.md`

## SCN-009 — No cache warning is needed when no graph cache exists

- Requirements: `REQ-005`, `REQ-006`
- Test: `TestSCN009_InstallClusteringReportsNextBuildCanUseClusteringWhenNoGraphCacheExists`
- Scenario source: `features/vela_install_clustering.feature` lines 79-85

### RED

- Added the smallest command-level test for `vela install --clustering` when no `.vela/graph.db` cache exists for the current workspace.
- The test proves install success remains successful, reports that the next graph build can use clustering, does not create a graph cache artifact, and does not emit existing-cache/delete/purge/remove messaging.
- Failing command: `go test ./cmd/vela -run TestSCN009_InstallClusteringReportsNextBuildCanUseClusteringWhenNoGraphCacheExists -count=1`
- Failure evidence:
  - `expected install output to contain "next graph build can use clustering"`
  - Actual output reported venv creation, baseline requirements install, optional Leiden separation, `networkx` verification, and install success only.

### GREEN

- Added minimal no-cache branch after successful dependency install and `networkx` verification.
- When `.vela/graph.db` is absent, install now reports `next graph build can use clustering` instead of emitting the existing-cache warning/rebuild recommendation.
- No graph cache creation, deletion, purge, removal, or force-rebuild behavior was added for this scenario.
- Passing focused command: `go test ./cmd/vela -run TestSCN009_InstallClusteringReportsNextBuildCanUseClusteringWhenNoGraphCacheExists -count=1`
- Passing SCN-001 through SCN-009 command: `go test ./cmd/vela -run 'TestSCN00[1-9]_InstallClustering' -count=1`
- Passing full suite: `go test ./... -count=1`

### REFACTOR

- Ran `gofmt` on touched Go files.
- No additional refactor; implementation stayed limited to SCN-009 no-cache messaging and did not add SCN-010 force-rebuild behavior.
- Passing full suite after formatting: `go test ./... -count=1`

### Files Changed

- `cmd/vela/main_test.go`
- `cmd/vela/cutover.go`
- `.rotta/tdd-log.md`

## SCN-010 — Explicit force rebuild separates install success from rebuild failure

- Requirements: `REQ-005`, `REQ-006`
- Test: `TestSCN010_InstallClusteringForceRebuildSeparatesInstallSuccessFromRebuildFailure`
- Scenario source: `features/vela_install_clustering.feature` lines 87-95

### RED

- Added the smallest command-level test for `vela install --clustering --force-rebuild` when a graph cache exists, dependency installation and `networkx` verification succeed, but the explicit graph rebuild fails.
- The test proves the dependency install runs before rebuild, the rebuild failure is reported separately from install success, and no rollback/destructive-cache messaging is emitted.
- Failing command: `go test ./cmd/vela -run TestSCN010_InstallClusteringForceRebuildSeparatesInstallSuccessFromRebuildFailure -count=1`
- Failure evidence:
  - `rebuild requests = []types.BuildRequest(nil), want one force rebuild for "/tmp/TestSCN010_InstallClusteringForceRebuildSeparatesInstallSuccessF2705202804/001"`
  - This proved explicit force-rebuild behavior was not yet wired into clustering install after successful dependency installation and verification.

### GREEN

- Added the explicit `--force-rebuild` flag for `vela install --clustering`.
- After successful dependency installation and `networkx` verification, force-rebuild mode now runs the graph build service for the selected project and reports rebuild failure separately while preserving the prior `clustering installation succeeded` output.
- The rebuild failure returns a non-zero command error without running rollback, uninstall, cache deletion, purge, or removal behavior.
- Passing focused command: `go test ./cmd/vela -run TestSCN010_InstallClusteringForceRebuildSeparatesInstallSuccessFromRebuildFailure -count=1`
- Passing SCN-001 through SCN-010 command: `go test ./cmd/vela -run 'TestSCN0(0[1-9]|10)_InstallClustering' -count=1`
- Passing full suite: `go test ./... -count=1`

### REFACTOR

- Ran `gofmt` on touched Go files.
- No additional refactor; implementation stayed limited to SCN-010 explicit force-rebuild failure separation and did not add SCN-011 permission-denied venv creation or SCN-012 verification-failure behavior.
- Passing focused command after formatting: `go test ./cmd/vela -run TestSCN010_InstallClusteringForceRebuildSeparatesInstallSuccessFromRebuildFailure -count=1`
- Passing SCN-001 through SCN-010 command after formatting: `go test ./cmd/vela -run 'TestSCN0(0[1-9]|10)_InstallClustering' -count=1`
- Passing full suite after formatting: `go test ./... -count=1`

### Files Changed

- `cmd/vela/main_test.go`
- `cmd/vela/cutover.go`
- `.rotta/tdd-log.md`
