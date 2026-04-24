#!/usr/bin/env bash
set -euo pipefail

VELA_DIR="/home/geen/Documents/personal/vela"
VELA_BIN="$VELA_DIR/bin/vela"
DEP_EVAL_DIR="/home/geen/Documents/personal/dep-eval"
DEP_EVAL_BIN="$DEP_EVAL_DIR/bin/dep-eval"
RESULTS_BASE_DEFAULT="$DEP_EVAL_DIR/results/vela-benchmark-loops"
GRAPHIFY_CMD="${GRAPHIFY_BIN:-graphify}"
HELPER_SCRIPT="$VELA_DIR/scripts/benchmark-loop-helper.mjs"

RESULTS_BASE="$RESULTS_BASE_DEFAULT"
HYPOTHESES_FILE=""
MAX_HYPOTHESES=10
ALLOW_DIRTY=0
PLAN_ONLY=0

usage() {
  printf 'Usage: %s [--results-base <dir>] [--hypotheses-file <path>] [--max-hypotheses <n>] [--allow-dirty] [--plan-only]\n' "$0"
  printf '\n'
  printf 'Runs the benchmark-driven improvement loop for real Vela:\n'
  printf '  1. Builds real Vela with make dev\n'
  printf '  2. Runs a focused query benchmark to seed current metrics\n'
  printf '  3. Suggests and ranks hypotheses from current benchmark gaps\n'
  printf '  4. Applies one hypothesis at a time via hypothesis apply/revert commands\n'
  printf '  5. Gates each attempt with PASS / NOT PASS focused validation\n'
  printf '  6. Runs both full corpora benchmarks plus the query benchmark again\n'
  printf '  7. Writes a final report with targets, attempts, and before/after deltas\n'
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --results-base)
      if [[ $# -lt 2 ]]; then
        printf 'Missing value for --results-base\n' >&2
        exit 1
      fi
      RESULTS_BASE="$2"
      shift 2
      ;;
    --hypotheses-file)
      if [[ $# -lt 2 ]]; then
        printf 'Missing value for --hypotheses-file\n' >&2
        exit 1
      fi
      HYPOTHESES_FILE="$2"
      shift 2
      ;;
    --max-hypotheses)
      if [[ $# -lt 2 ]]; then
        printf 'Missing value for --max-hypotheses\n' >&2
        exit 1
      fi
      MAX_HYPOTHESES="$2"
      shift 2
      ;;
    --allow-dirty)
      ALLOW_DIRTY=1
      shift
      ;;
    --plan-only)
      PLAN_ONLY=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      printf 'Unknown argument: %s\n' "$1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

require_file() {
  local path="$1"
  if [[ ! -e "$path" ]]; then
    printf 'Required path not found: %s\n' "$path" >&2
    exit 1
  fi
}

require_command() {
  local command_name="$1"
  if ! command -v "$command_name" >/dev/null 2>&1; then
    printf 'Required command not found on PATH: %s\n' "$command_name" >&2
    exit 1
  fi
}

ensure_clean_worktree() {
  local status
  status="$(git -C "$VELA_DIR" status --porcelain)"
  if [[ -n "$status" ]]; then
    printf 'Refusing to auto-apply hypotheses in a dirty worktree. Use --allow-dirty to override, or run with --plan-only.\n' >&2
    exit 1
  fi
}

run_and_capture() {
  local mode="$1"
  shift
  local log_path="$1"
  shift
  local output
  output="$(VELA_BIN="$VELA_BIN" "$DEP_EVAL_BIN" "$@" 2>&1 | tee "$log_path")"
  if [[ "$mode" == "full" ]]; then
    if [[ "$output" =~ Running[[:space:]]([^[:space:]]+)[[:space:]]on[[:space:]]corpus ]]; then
      printf '%s\n' "${BASH_REMATCH[1]}"
      return 0
    fi
  fi
  if [[ "$mode" == "query" ]]; then
    if [[ "$output" =~ Running[[:space:]]graph-query[[:space:]]benchmark[[:space:]]([^[:space:]]+)[[:space:]]with[[:space:]]adapters ]]; then
      printf '%s\n' "${BASH_REMATCH[1]}"
      return 0
    fi
  fi
  printf 'Could not parse %s run ID from dep-eval output. See %s\n' "$mode" "$log_path" >&2
  exit 1
}

run_make_dev() {
  local log_path="$1"
  printf 'Building real Vela with make dev...\n'
  make -C "$VELA_DIR" dev | tee "$log_path"
  if [[ ! -x "$VELA_BIN" ]]; then
    printf 'Expected built binary at %s but it is not executable\n' "$VELA_BIN" >&2
    exit 1
  fi
}

run_query_benchmark() {
  local label="$1"
  local run_id
  printf 'Running focused query benchmark (%s)...\n' "$label"
  run_id="$(run_and_capture query "$log_dir/query-${label}.log" query-run --adapter vela,vela-next-sim,graphify --suite code-v1 --results-dir "$results_dir")"
  local run_dir="$results_dir/$run_id"
  local report_json="$run_dir/graph-query-report.json"
  local report_md="$run_dir/graph-query-report.md"
  require_file "$report_json"
  require_file "$report_md"
  LAST_QUERY_RUN_ID="$run_id"
  LAST_QUERY_REPORT_JSON="$report_json"
  LAST_QUERY_REPORT_MD="$report_md"
}

record_attempt_failure() {
  local attempt_json="$1"
  local hypothesis_id="$2"
  local status="$3"
  local reason="$4"
  node "$HELPER_SCRIPT" record-attempt \
    --hypotheses-file "$HYPOTHESES_FILE" \
    --hypothesis-id "$hypothesis_id" \
    --status "$status" \
    --reason "$reason" \
    --out-json "$attempt_json"
}

require_file "$VELA_DIR/Makefile"
require_file "$DEP_EVAL_BIN"
require_file "$DEP_EVAL_DIR/node_modules/.bin/tsx"
require_file "$HELPER_SCRIPT"
require_command make
require_command "$GRAPHIFY_CMD"
require_command git
require_command node

if [[ -n "$HYPOTHESES_FILE" ]]; then
  require_file "$HYPOTHESES_FILE"
fi

if [[ -n "$HYPOTHESES_FILE" && "$ALLOW_DIRTY" -ne 1 ]]; then
  ensure_clean_worktree
fi

timestamp="$(date -u +%Y-%m-%dT%H-%M-%SZ)"
loop_dir="$RESULTS_BASE/$timestamp"
results_dir="$loop_dir/results"
log_dir="$loop_dir/logs"
attempts_dir="$loop_dir/attempts"
manifest_path="$loop_dir/manifest.txt"
plan_json="$loop_dir/hypothesis-plan.json"
plan_md="$loop_dir/hypothesis-plan.md"
final_report_json="$loop_dir/final-report.json"
final_report_md="$loop_dir/final-report.md"

mkdir -p "$results_dir" "$log_dir" "$attempts_dir"

run_make_dev "$log_dir/make-dev-seed.log"
run_query_benchmark seed
accepted_query_report_json="$LAST_QUERY_REPORT_JSON"
accepted_query_report_md="$LAST_QUERY_REPORT_MD"
accepted_query_run_id="$LAST_QUERY_RUN_ID"

printf 'Suggesting hypotheses from current benchmark gaps...\n'
suggest_args=(
  "$HELPER_SCRIPT"
  suggest
  --current-query-report "$accepted_query_report_json"
  --out-json "$plan_json"
  --out-md "$plan_md"
)
if [[ -n "$HYPOTHESES_FILE" ]]; then
  suggest_args+=(--hypotheses-file "$HYPOTHESES_FILE")
fi
node "${suggest_args[@]}"

if [[ "$PLAN_ONLY" -eq 1 || -z "$HYPOTHESES_FILE" ]]; then
  {
    printf 'Loop timestamp: %s\n' "$timestamp"
    printf 'Mode: %s\n' "plan-only"
    printf 'Vela dir: %s\n' "$VELA_DIR"
    printf 'Vela binary: %s\n' "$VELA_BIN"
    printf 'Results root: %s\n' "$loop_dir"
    printf 'Seed query benchmark run ID: %s\n' "$accepted_query_run_id"
    printf 'Seed query benchmark report JSON: %s\n' "$accepted_query_report_json"
    printf 'Seed query benchmark report MD: %s\n' "$accepted_query_report_md"
    printf 'Hypothesis plan JSON: %s\n' "$plan_json"
    printf 'Hypothesis plan MD: %s\n' "$plan_md"
    printf 'Build log: %s\n' "$log_dir/make-dev-seed.log"
    printf 'Seed query log: %s\n' "$log_dir/query-seed.log"
  } > "$manifest_path"

  printf '\nBenchmark loop planning complete.\n'
  printf 'Manifest: %s\n' "$manifest_path"
  printf 'Hypothesis plan: %s\n' "$plan_md"
  exit 0
fi

mapfile -t hypothesis_ids < <(node -e 'const fs=require("fs"); const plan=JSON.parse(fs.readFileSync(process.argv[1], "utf8")); const max=Number(process.argv[2]); const ids=(plan.actionable_ids||[]).slice(0,max); for (const id of ids) console.log(id);' "$plan_json" "$MAX_HYPOTHESES")

if [[ ${#hypothesis_ids[@]} -eq 0 ]]; then
  printf 'No actionable hypotheses matched the current benchmark gaps. See %s\n' "$plan_md" >&2
  exit 1
fi

attempt_index=0
for hypothesis_id in "${hypothesis_ids[@]}"; do
  attempt_index=$((attempt_index + 1))
  attempt_dir="$attempts_dir/$(printf '%02d' "$attempt_index")-$hypothesis_id"
  attempt_json="$attempt_dir/result.json"
  mkdir -p "$attempt_dir"

  printf 'Applying hypothesis %s...\n' "$hypothesis_id"
  if ! node "$HELPER_SCRIPT" run-command \
    --hypotheses-file "$HYPOTHESES_FILE" \
    --hypothesis-id "$hypothesis_id" \
    --mode apply \
    --workdir "$VELA_DIR" \
    --log-path "$attempt_dir/apply.log"; then
    record_attempt_failure "$attempt_json" "$hypothesis_id" APPLY_FAILED "apply command failed"
    continue
  fi

  if ! make -C "$VELA_DIR" dev | tee "$attempt_dir/make-dev.log"; then
    record_attempt_failure "$attempt_json" "$hypothesis_id" BUILD_FAILED "make dev failed"
    node "$HELPER_SCRIPT" run-command \
      --hypotheses-file "$HYPOTHESES_FILE" \
      --hypothesis-id "$hypothesis_id" \
      --mode revert \
      --workdir "$VELA_DIR" \
      --log-path "$attempt_dir/revert.log" >/dev/null 2>&1 || true
    continue
  fi

  if [[ ! -x "$VELA_BIN" ]]; then
    record_attempt_failure "$attempt_json" "$hypothesis_id" BUILD_FAILED "make dev did not leave an executable vela binary"
    node "$HELPER_SCRIPT" run-command \
      --hypotheses-file "$HYPOTHESES_FILE" \
      --hypothesis-id "$hypothesis_id" \
      --mode revert \
      --workdir "$VELA_DIR" \
      --log-path "$attempt_dir/revert.log" >/dev/null 2>&1 || true
    continue
  fi

  run_id="$(run_and_capture query "$attempt_dir/query.log" query-run --adapter vela,vela-next-sim,graphify --suite code-v1 --results-dir "$results_dir")"
  current_query_run_dir="$results_dir/$run_id"
  current_query_report_json="$current_query_run_dir/graph-query-report.json"
  current_query_report_md="$current_query_run_dir/graph-query-report.md"
  require_file "$current_query_report_json"
  require_file "$current_query_report_md"

  if node "$HELPER_SCRIPT" focused-gate \
    --hypotheses-file "$HYPOTHESES_FILE" \
    --hypothesis-id "$hypothesis_id" \
    --baseline-query-report "$accepted_query_report_json" \
    --current-query-report "$current_query_report_json" \
    --out-json "$attempt_json"; then
    printf 'PASS: %s\n' "$hypothesis_id"
    accepted_query_report_json="$current_query_report_json"
    accepted_query_report_md="$current_query_report_md"
    accepted_query_run_id="$run_id"
    continue
  fi

  printf 'NOT PASS: %s\n' "$hypothesis_id"
  node "$HELPER_SCRIPT" run-command \
    --hypotheses-file "$HYPOTHESES_FILE" \
    --hypothesis-id "$hypothesis_id" \
    --mode revert \
    --workdir "$VELA_DIR" \
    --log-path "$attempt_dir/revert.log" >/dev/null 2>&1 || true
done

run_make_dev "$log_dir/make-dev-final.log"

printf 'Running full benchmark on vela...\n'
vela_full_run_id="$(run_and_capture full "$log_dir/full-benchmark-vela.log" run --adapter vela,vela-next-sim,graphify --corpus vela --results-dir "$results_dir")"

printf 'Running full benchmark on stock-chef...\n'
stock_chef_full_run_id="$(run_and_capture full "$log_dir/full-benchmark-stock-chef.log" run --adapter vela,vela-next-sim,graphify --corpus stock-chef --results-dir "$results_dir")"

run_query_benchmark final

vela_full_run_dir="$results_dir/$vela_full_run_id"
stock_chef_full_run_dir="$results_dir/$stock_chef_full_run_id"
query_run_dir="$results_dir/$LAST_QUERY_RUN_ID"
vela_full_report_json="$vela_full_run_dir/report.json"
vela_full_report_md="$vela_full_run_dir/report.md"
stock_chef_full_report_json="$stock_chef_full_run_dir/report.json"
stock_chef_full_report_md="$stock_chef_full_run_dir/report.md"
query_report_json="$query_run_dir/graph-query-report.json"
query_report_md="$query_run_dir/graph-query-report.md"

require_file "$vela_full_report_json"
require_file "$vela_full_report_md"
require_file "$stock_chef_full_report_json"
require_file "$stock_chef_full_report_md"
require_file "$query_report_json"
require_file "$query_report_md"

printf 'Writing final benchmark loop report...\n'
node "$HELPER_SCRIPT" final-report \
  --attempts-dir "$attempts_dir" \
  --full-vela-report "$vela_full_report_json" \
  --full-stock-report "$stock_chef_full_report_json" \
  --query-report "$query_report_json" \
  --out-json "$final_report_json" \
  --out-md "$final_report_md"

{
  printf 'Loop timestamp: %s\n' "$timestamp"
  printf 'Vela dir: %s\n' "$VELA_DIR"
  printf 'Vela binary: %s\n' "$VELA_BIN"
  printf 'dep-eval dir: %s\n' "$DEP_EVAL_DIR"
  printf 'Results root: %s\n' "$loop_dir"
  printf 'Hypotheses file: %s\n' "$HYPOTHESES_FILE"
  printf 'Hypothesis plan JSON: %s\n' "$plan_json"
  printf 'Hypothesis plan MD: %s\n' "$plan_md"
  printf 'Accepted focused query run ID: %s\n' "$accepted_query_run_id"
  printf 'Accepted focused query report JSON: %s\n' "$accepted_query_report_json"
  printf 'Accepted focused query report MD: %s\n' "$accepted_query_report_md"
  printf 'Full benchmark vela run ID: %s\n' "$vela_full_run_id"
  printf 'Full benchmark vela report JSON: %s\n' "$vela_full_report_json"
  printf 'Full benchmark vela report MD: %s\n' "$vela_full_report_md"
  printf 'Full benchmark stock-chef run ID: %s\n' "$stock_chef_full_run_id"
  printf 'Full benchmark stock-chef report JSON: %s\n' "$stock_chef_full_report_json"
  printf 'Full benchmark stock-chef report MD: %s\n' "$stock_chef_full_report_md"
  printf 'Final query benchmark run ID: %s\n' "$LAST_QUERY_RUN_ID"
  printf 'Final query benchmark report JSON: %s\n' "$query_report_json"
  printf 'Final query benchmark report MD: %s\n' "$query_report_md"
  printf 'Final loop report JSON: %s\n' "$final_report_json"
  printf 'Final loop report MD: %s\n' "$final_report_md"
  printf 'Attempts dir: %s\n' "$attempts_dir"
  printf 'Seed build log: %s\n' "$log_dir/make-dev-seed.log"
  printf 'Final build log: %s\n' "$log_dir/make-dev-final.log"
  printf 'Full benchmark vela log: %s\n' "$log_dir/full-benchmark-vela.log"
  printf 'Full benchmark stock-chef log: %s\n' "$log_dir/full-benchmark-stock-chef.log"
  printf 'Final query benchmark log: %s\n' "$log_dir/query-final.log"
} > "$manifest_path"

printf '\nBenchmark loop complete.\n'
printf 'Manifest: %s\n' "$manifest_path"
printf 'Final loop report: %s\n' "$final_report_md"
printf 'Attempts: %s\n' "$attempts_dir"
