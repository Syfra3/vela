# Hard Spec: Vela clustering installation support

## Adversarial Pre-Mortem
- Failure mode 1: Installation appears successful but community detection still degrades at graph rebuild time because the installer does not verify the exact Python environment Vela will use.
- Failure mode 2: A repair path silently changes permissions on every update or install, masking ownership problems and surprising users with broad filesystem mutation.
- Failure mode 3: Clustering becomes available after a previous degraded graph build, but stale graph cache continues to omit community metadata unless the user is clearly told to rebuild or explicitly requests a force rebuild.

## Hidden Assumptions
- Vela's Python clustering runtime resolves dependencies from the repo-local `.venv` when that directory exists, so install verification must target that environment rather than an arbitrary global interpreter.
- `requirements-clustering.txt` represents the baseline portable clustering dependency set and currently provides `networkx`; Leiden/graspologic dependencies remain optional and separate.
- Some existing `.venv` directories may be present but not user-writable, including read-only activation files, and recreating the venv can fail before dependency installation begins.
- Graph freshness checks can report fresh source/index state while clustering capability has changed; dependency freshness and graph semantic completeness are related but distinct.

## Alternatives Considered
| Approach | Reason Rejected |
|----------|----------------|
| Install clustering dependencies opportunistically during every `vela update` | Violates user control, can repeatedly fail on locked venvs, and turns routine graph maintenance into dependency mutation. |
| Automatically `chmod -R u+w .venv` whenever installation encounters permission errors | Too destructive and surprising; permission repair must be explicit because it mutates an existing environment. |
| Install `graspologic` by default with `--clustering` | Baseline clustering must be portable and lightweight; Leiden/graspologic dependencies are optional and should remain separate if supported. |
| Delete graph caches when clustering dependencies become available | Destructive cache invalidation is unnecessary and can discard usable work; rebuild should be recommended or explicitly forced. |
| Ignore existing repo-local `.venv` and install into a global/user environment | Would not match Vela's runtime dependency resolution when `.venv` exists, causing false-positive install success. |

## Summary
Add explicit install-time support for Vela's baseline clustering dependencies through `vela install --clustering`. The command must create or reuse the repo-local virtual environment that Vela will use, install `requirements-clustering.txt`, verify `networkx` in that environment, fail with clear repair guidance when an existing `.venv` is not writable, and provide an explicit `--repair-venv` path for permission repair. When clustering capability changes from unavailable to available, install must make stale graph-cache risk visible and recommend or perform only explicitly requested rebuild behavior without deleting caches by default.

## Requirements

### REQ-001: Clustering install command
**Description:** `vela install --clustering` shall install Vela's baseline clustering dependencies into the Python environment that Vela will use for community detection.
**Acceptance Criteria:**
- The command creates a repo-local `.venv` when no repo-local `.venv` exists.
- The command reuses the repo-local `.venv` when it already exists and is writable.
- The command installs dependencies from `requirements-clustering.txt`.
- The command verifies that `networkx` can be imported from the selected environment after installation.
- The command reports success only after the dependency install and `networkx` verification both pass.
**Edge Cases:**
- If `.venv` exists but lacks a usable Python executable, the command must report the environment as invalid and give an actionable recovery path.
- If `requirements-clustering.txt` is missing, unreadable, or installation fails, the command must fail with the underlying dependency-install error and the path it attempted to use.
- If `networkx` import verification fails after installation, the command must fail rather than reporting a partial success.
**Out of Scope:**
- Installing production/test dependencies unrelated to clustering.
- Implementing or changing community-detection algorithms.

### REQ-002: Writable venv preflight and actionable failure
**Description:** Before mutating an existing repo-local `.venv`, `vela install --clustering` shall preflight that the environment is writable enough to create/recreate activation metadata and install packages.
**Acceptance Criteria:**
- If an existing `.venv` is not writable by the current user, the command fails before attempting dependency installation.
- The error message identifies the repo-local `.venv` as the failing environment.
- The error message recommends an explicit repair command using `vela install --clustering --repair-venv` or equivalent documented manual remediation.
- The default install path must not run `chmod`, `chown`, or broad permission repair automatically.
**Edge Cases:**
- Read-only files inside `.venv/bin` or activation scripts must be treated as a failed writable preflight, not as a generic install failure.
- Permission-denied errors from `python3 -m venv .venv` must be translated into a Vela-level actionable error.
- If the filesystem is read-only or ownership prevents repair, the error must preserve the underlying OS failure details.
**Out of Scope:**
- Automatically repairing permissions during `vela update`, graph rebuild, or any command other than an explicit repair install invocation.

### REQ-003: Explicit venv repair mode
**Description:** `vela install --clustering --repair-venv` shall be the only built-in path that may make an existing repo-local `.venv` writable before clustering installation.
**Acceptance Criteria:**
- Repair mode only operates on the repo-local `.venv` selected for Vela's clustering runtime.
- Repair mode makes the existing `.venv` writable for the current user before dependency installation is attempted.
- Repair mode reports that it changed permissions and then continues with the same install and verification flow as REQ-001.
- If repair cannot make the venv writable, the command fails with an actionable error and does not claim dependencies were installed.
**Edge Cases:**
- If `.venv` does not exist, `--repair-venv` must not fail solely because there is nothing to repair; it may proceed to create the venv.
- If only some files are repaired and installation still fails, the final error must distinguish repair success from dependency-install failure.
- Repair mode must not traverse outside the repo-local `.venv` via symlink or path confusion.
**Out of Scope:**
- Repairing global Python environments, system site-packages, or unrelated project virtual environments.

### REQ-004: Optional clustering dependency boundaries
**Description:** Baseline clustering installation shall install only the portable fallback dependencies and shall not install optional Leiden/graspologic dependencies by default.
**Acceptance Criteria:**
- `vela install --clustering` installs `requirements-clustering.txt` only for baseline clustering.
- The command does not install `graspologic` unless an existing, explicit optional Leiden dependency mechanism is requested separately.
- Output must distinguish baseline clustering support from optional Leiden support when relevant.
**Edge Cases:**
- If optional Leiden dependencies are already installed, baseline install must not remove or downgrade them.
- If optional Leiden support has a separate flag or requirements file, this spec does not change that contract except to keep it separate from baseline `--clustering`.
**Out of Scope:**
- Adding new default Leiden/graspologic installation behavior.
- Changing `requirements-clustering-leiden.txt` semantics except as needed to preserve separation.

### REQ-005: Graph cache staleness after clustering availability changes
**Description:** When `vela install --clustering` changes baseline clustering capability from unavailable to available, Vela shall make graph-cache staleness risk visible and support a non-destructive path to refresh community metadata.
**Acceptance Criteria:**
- After successful install, if clustering was previously unavailable or verification newly succeeds, output must state that existing graph cache may have been built without community metadata.
- The command must recommend a graph rebuild command or, if Vela already supports a force rebuild option for install, use it only when explicitly requested by the user.
- The default install path must not delete graph caches or other graph artifacts.
- If an explicit force rebuild option is supported, it must rebuild graph/community data using the now-verified clustering environment and report rebuild success or failure separately from dependency install success.
**Edge Cases:**
- If no graph cache exists, install should not warn about deleting or rebuilding cache; it may report that the next build will use clustering.
- If rebuild fails after dependency installation succeeds, the command must not roll back dependencies and must report the install as successful with rebuild failure details.
- Freshness status based on `--graph` or source timestamps must not suppress the clustering-availability warning when clustering capability has changed.
**Out of Scope:**
- Destructive cache deletion unless a separate explicit destructive command exists and is invoked.
- Changing the definition of source freshness used by `vela status`.

### REQ-006: User-facing diagnostics and idempotency
**Description:** Clustering installation shall be safe to rerun and shall produce diagnostics that distinguish environment selection, permission preflight, dependency installation, verification, and rebuild recommendation/outcome.
**Acceptance Criteria:**
- Re-running `vela install --clustering` on a writable venv with `networkx` already installed succeeds without unnecessary repair.
- Command output identifies the selected environment as repo-local `.venv` when applicable.
- Failures are categorized enough for a user to know whether to repair permissions, reinstall dependencies, or rebuild graph cache.
- The command exits non-zero for failed preflight, failed dependency install, failed verification, or failed explicitly requested rebuild.
**Edge Cases:**
- Partial installs must not be reported as complete unless verification passes.
- A successful dependency install followed by a rebuild warning/recommendation must still exit successfully when no explicit rebuild was requested.
- A successful dependency install followed by an explicitly requested rebuild failure must make both facts visible.
**Out of Scope:**
- Silent background repair, background dependency installation, or automatic retries that hide root causes.

## Open Questions
- None. Implementation may choose the exact name of any explicit force-rebuild install flag if one does not already exist, but the behavior must remain opt-in and non-destructive by default.

## Trade-offs
- The default install path is stricter and may fail on read-only `.venv` directories even when a manual chmod would work; this preserves user control and avoids surprising permission mutation.
- Baseline clustering stays limited to `networkx`, so advanced Leiden quality remains optional rather than automatic.
- Rebuild is recommended rather than forced by default, reducing destructive risk at the cost of requiring one extra user action to refresh old degraded graph caches.

## Risk Level
medium — Justification: The change touches installation, Python environment management, and graph-cache freshness messaging. The behavioral surface is user-facing and failure-prone around permissions, but the spec avoids algorithm changes and requires explicit repair/rebuild actions for risky mutations.
