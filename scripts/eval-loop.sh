#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOOP_FILE="$SCRIPT_DIR/eval-loop.json"
STATE_TEMPLATE="$SCRIPT_DIR/phase-state.template.json"
STATE_FILE="$SCRIPT_DIR/phase-state.json"
INNER_LOOP="$SCRIPT_DIR/run-benchmark-loop.sh"

usage() {
  cat <<'EOF'
Usage:
  ./scripts/eval-loop.sh init
  ./scripts/eval-loop.sh status
  ./scripts/eval-loop.sh run
  ./scripts/eval-loop.sh require-human <reason>
  ./scripts/eval-loop.sh clear-human

Commands:
  init            Create phase-state.json from the template if missing.
  status          Show campaign cycle, current phase, iteration, and terminal state.
  run             Run the bounded benchmark campaign until all phases pass or exhaust.
  require-human   Pause the loop and require human direction.
  clear-human     Clear the human-decision-required flag.
EOF
}

require_file() {
  local file="$1"
  if [[ ! -f "$file" ]]; then
    printf 'Missing required file: %s\n' "$file" >&2
    exit 1
  fi
}

require_state() {
  require_file "$STATE_FILE"
}

json_get() {
  local file="$1"
  local expr="$2"
  jq -r "$expr" "$file"
}

json_update() {
  local file="$1"
  shift
  local expr="${@: -1}"
  set -- "${@:1:$(($#-1))}"
  local tmp
  tmp="$(mktemp)"
  jq "$@" "$expr" "$file" > "$tmp"
  mv "$tmp" "$file"
}

init_state() {
  require_file "$STATE_TEMPLATE"
  require_file "$LOOP_FILE"
  if [[ -f "$STATE_FILE" ]]; then
    printf 'State already exists: %s\n' "$STATE_FILE"
    return 0
  fi
  cp "$STATE_TEMPLATE" "$STATE_FILE"
  printf 'Initialized state: %s\n' "$STATE_FILE"
}

current_phase_id() {
  json_get "$STATE_FILE" '.currentPhaseId'
}

current_iteration() {
  json_get "$STATE_FILE" '.currentIteration'
}

campaign_cycle() {
  json_get "$STATE_FILE" '.campaignCycle // 1'
}

human_required() {
  json_get "$STATE_FILE" '.humanDecisionRequired'
}

phase_best_report_path() {
  local phase_id="$1"
  jq -r --arg phase "$phase_id" '.phaseBestResults[$phase].reportPath // ""' "$STATE_FILE"
}

phase_iteration() {
  local phase_id="$1"
  jq -r --arg phase "$phase_id" '
    .phaseProgress[$phase].nextIteration //
    (if .currentPhaseId == $phase and (.currentIteration // 0) > 0 then
       .currentIteration
     elif (.phaseBestResults[$phase].iteration // 0) > 0 then
       (.phaseBestResults[$phase].iteration + 1)
     else
       1
     end)
  ' "$STATE_FILE"
}

phase_exhausted() {
  local phase_id="$1"
  jq -r --arg phase "$phase_id" '.phaseProgress[$phase].exhausted // false' "$STATE_FILE"
}

phase_passed() {
  local phase_id="$1"
  jq -r --arg phase "$phase_id" '.approvedBaselines.phases[$phase].approved // false' "$STATE_FILE"
}

sync_current_phase_cursor() {
  local next_phase next_iteration
  next_phase=""

  while IFS= read -r phase_id; do
    [[ -z "$phase_id" ]] && continue
    if [[ "$(phase_passed "$phase_id")" != "true" && "$(phase_exhausted "$phase_id")" != "true" ]]; then
      next_phase="$phase_id"
      break
    fi
  done < <(jq -r '.phaseOrder[]' "$LOOP_FILE")

  if [[ -n "$next_phase" ]]; then
    next_iteration="$(phase_iteration "$next_phase")"
    json_update "$STATE_FILE" \
      --arg phase "$next_phase" \
      --argjson iteration "$next_iteration" \
      '.currentPhaseId = $phase | .currentIteration = $iteration | if .status != "waiting_human" then .status = "idle" else . end'
  else
    json_update "$STATE_FILE" '.status = "complete"'
  fi
}

ensure_state_shape() {
  require_state
  json_update "$STATE_FILE" '.campaignCycle = (.campaignCycle // 1) | .phaseProgress = (.phaseProgress // {})'

  while IFS= read -r phase_id; do
    [[ -z "$phase_id" ]] && continue

    local existing_iteration default_iteration final_outcome final_report
    existing_iteration="$(jq -r --arg phase "$phase_id" '.phaseProgress[$phase].nextIteration // empty' "$STATE_FILE")"
    if [[ -z "$existing_iteration" || "$existing_iteration" == "null" ]]; then
      if [[ "$(phase_passed "$phase_id")" == "true" ]]; then
        default_iteration="$(jq -r --arg phase "$phase_id" 'if (.phaseBestResults[$phase].iteration // 0) > 0 then .phaseBestResults[$phase].iteration else 1 end' "$STATE_FILE")"
      elif [[ "$(current_phase_id)" == "$phase_id" && "$(current_iteration)" != "0" ]]; then
        default_iteration="$(current_iteration)"
      else
        default_iteration="$(jq -r --arg phase "$phase_id" 'if (.phaseBestResults[$phase].iteration // 0) > 0 then (.phaseBestResults[$phase].iteration + 1) else 1 end' "$STATE_FILE")"
      fi
      json_update "$STATE_FILE" --arg phase "$phase_id" --argjson iteration "$default_iteration" '.phaseProgress[$phase].nextIteration = $iteration'
    fi

    json_update "$STATE_FILE" --arg phase "$phase_id" '
      .phaseProgress[$phase].exhausted = (.phaseProgress[$phase].exhausted // false) |
      .phaseProgress[$phase].exhaustedAt = (.phaseProgress[$phase].exhaustedAt // "") |
      .phaseProgress[$phase].finalOutcome = (.phaseProgress[$phase].finalOutcome // "") |
      .phaseProgress[$phase].finalReportPath = (.phaseProgress[$phase].finalReportPath // "")
    '

    if [[ "$(phase_passed "$phase_id")" == "true" ]]; then
      final_outcome="$(jq -r --arg phase "$phase_id" '.phaseProgress[$phase].finalOutcome // ""' "$STATE_FILE")"
      final_report="$(jq -r --arg phase "$phase_id" '.phaseProgress[$phase].finalReportPath // ""' "$STATE_FILE")"
      if [[ -z "$final_outcome" || "$final_outcome" == "null" ]]; then
        json_update "$STATE_FILE" --arg phase "$phase_id" '.phaseProgress[$phase].finalOutcome = "passed"'
      fi
      if [[ -z "$final_report" || "$final_report" == "null" ]]; then
        json_update "$STATE_FILE" --arg phase "$phase_id" --arg report "$(jq -r --arg phase "$phase_id" '.approvedBaselines.phases[$phase].reportPath // ""' "$STATE_FILE")" '.phaseProgress[$phase].finalReportPath = $report'
      fi
    fi
  done < <(jq -r '.phaseOrder[]' "$LOOP_FILE")

  sync_current_phase_cursor
}

phase_goal_keys() {
  local phase_id="$1"
  jq -r --arg phase "$phase_id" '.phases[] | select(.id == $phase) | .goalMetrics | keys[]' "$LOOP_FILE"
}

metric_rule() {
  local metric="$1"
  case "$metric" in
    balanced_gate_pass|reverse_lookup_improved_vs_baseline|path_quality_improved_vs_baseline|full_vela.build_success)
      echo "bool_true"
      ;;
    full_vela.build_time_ms|full_vela.unresolved_reference_rate)
      echo "max"
      ;;
    *)
      echo "min"
      ;;
  esac
}

metric_value() {
  local report_path="$1"
  local metric="$2"
  case "$metric" in
    balanced_gate_pass)
      jq -r '(.balanced_gate == "PASS")' "$report_path"
      ;;
    reverse_lookup_improved_vs_baseline)
      jq -r '(.before_after.query.deltas.reverse_lookup_mean_f1 // 0) > 0' "$report_path"
      ;;
    path_quality_improved_vs_baseline)
      jq -r '((.before_after.query.deltas.path_node_precision // 0) > 0) or ((.before_after.query.deltas.path_node_recall // 0) > 0) or ((.before_after.query.deltas.path_tracing_path_node_recall // 0) > 0)' "$report_path"
      ;;
    query.weighted_score)
      jq -r '.before_after.query.current.weighted_score' "$report_path"
      ;;
    query.path_node_precision)
      jq -r '.before_after.query.current.path_node_precision' "$report_path"
      ;;
    query.path_node_recall)
      jq -r '.before_after.query.current.path_node_recall' "$report_path"
      ;;
    full_vela.build_success)
      jq -r '.before_after.full_vela.current.build_success' "$report_path"
      ;;
    full_vela.build_time_ms)
      jq -r '.before_after.full_vela.current.build_time_ms' "$report_path"
      ;;
    full_vela.file_coverage_ratio)
      jq -r '.before_after.full_vela.current.file_coverage_ratio' "$report_path"
      ;;
    full_vela.symbol_coverage_ratio)
      jq -r '.before_after.full_vela.current.symbol_coverage_ratio' "$report_path"
      ;;
    full_vela.unresolved_reference_rate)
      jq -r '.before_after.full_vela.current.unresolved_reference_rate' "$report_path"
      ;;
    full_vela.layer1_weighted_score)
      jq -r '.before_after.full_vela.current.layer1_weighted_score' "$report_path"
      ;;
    full_stock.layer1_weighted_score)
      jq -r '.before_after.full_stock.current.layer1_weighted_score' "$report_path"
      ;;
    full_stock.layer2_weighted_score)
      jq -r '.before_after.full_stock.current.layer2_weighted_score' "$report_path"
      ;;
    *)
      printf 'Unsupported metric: %s\n' "$metric" >&2
      return 1
      ;;
  esac
}

compare_metric() {
  local rule="$1"
  local actual="$2"
  local target="$3"
  case "$rule" in
    min)
      awk -v a="$actual" -v t="$target" 'BEGIN { exit !(a + 0 >= t + 0) }'
      ;;
    max)
      awk -v a="$actual" -v t="$target" 'BEGIN { exit !(a + 0 <= t + 0) }'
      ;;
    bool_true)
      [[ "$actual" == "true" ]]
      ;;
    *)
      return 2
      ;;
  esac
}

metric_progress() {
  local rule="$1"
  local actual="$2"
  local target="$3"
  case "$rule" in
    min)
      awk -v a="$actual" -v t="$target" 'BEGIN {
        if (a + 0 >= t + 0) {
          print 1
        } else if (t + 0 == 0) {
          print 0
        } else {
          p = (a + 0) / (t + 0)
          if (p < 0) p = 0
          printf "%.12f", p
        }
      }'
      ;;
    max)
      awk -v a="$actual" -v t="$target" 'BEGIN {
        if (a + 0 <= t + 0) {
          print 1
        } else if (a + 0 == 0) {
          print 1
        } else {
          p = (t + 0) / (a + 0)
          if (p < 0) p = 0
          printf "%.12f", p
        }
      }'
      ;;
    bool_true)
      if [[ "$actual" == "true" ]]; then
        echo "1"
      else
        echo "0"
      fi
      ;;
    *)
      echo "0"
      ;;
  esac
}

json_value_literal() {
  local value="$1"
  if [[ "$value" == "true" || "$value" == "false" ]]; then
    printf '%s' "$value"
  elif [[ "$value" =~ ^-?[0-9]+([.][0-9]+)?$ ]]; then
    printf '%s' "$value"
  else
    jq -Rn --arg value "$value" '$value'
  fi
}

set_human_required() {
  local reason="$1"
  json_update "$STATE_FILE" --arg reason "$reason" '.humanDecisionRequired = true | .status = "waiting_human" | .humanDecisionReason = $reason'
  printf 'Human decision required: %s\n' "$reason"
}

clear_human_required() {
  require_state
  json_update "$STATE_FILE" '.humanDecisionRequired = false | .humanDecisionReason = "" | .status = "idle"'
  printf 'Human-decision flag cleared.\n'
}

record_history() {
  local phase_id="$1"
  local iteration="$2"
  local report_path="$3"
  local manifest_path="$4"
  local outcome="$5"
  local tmp
  tmp="$(mktemp)"
  jq \
    --arg phase "$phase_id" \
    --argjson iteration "$iteration" \
    --arg reportPath "$report_path" \
    --arg manifestPath "$manifest_path" \
    --arg outcome "$outcome" \
    --arg timestamp "$(date -Iseconds)" \
    '.history += [{phaseId: $phase, iteration: $iteration, reportPath: $reportPath, manifestPath: $manifestPath, outcome: $outcome, timestamp: $timestamp}]' \
    "$STATE_FILE" > "$tmp"
  mv "$tmp" "$STATE_FILE"
}

build_phase_result_summary() {
  local phase_id="$1"
  local report_path="$2"
  local goals_total=0 goals_met=0
  local score="0"
  local metrics='{}'

  while IFS= read -r metric; do
    [[ -z "$metric" ]] && continue
    local rule target actual progress value_json
    rule="$(metric_rule "$metric")"
    target="$(jq -r --arg phase "$phase_id" --arg metric "$metric" '.phases[] | select(.id == $phase) | .goalMetrics[$metric] // empty' "$LOOP_FILE")"
    actual="$(metric_value "$report_path" "$metric")"
    value_json="$(json_value_literal "$actual")"
    metrics="$(jq -cn --argjson obj "$metrics" --arg key "$metric" --argjson value "$value_json" '$obj + {($key): $value}')"
    ((goals_total += 1))
    if compare_metric "$rule" "$actual" "$target"; then
      ((goals_met += 1))
      progress="1"
    else
      progress="$(metric_progress "$rule" "$actual" "$target")"
    fi
    score="$(awk -v s="$score" -v p="$progress" 'BEGIN { printf "%.12f", s + p }')"
  done < <(phase_goal_keys "$phase_id")

  jq -cn \
    --argjson metrics "$metrics" \
    --argjson goalsMet "$goals_met" \
    --argjson goalsTotal "$goals_total" \
    --argjson score "$score" \
    '{metrics: $metrics, goalsMetCount: $goalsMet, goalsTotal: $goalsTotal, score: $score}'
}

phase_best_improved() {
  local phase_id="$1"
  local candidate_summary="$2"
  local best_report_path best_goals_met best_score candidate_goals_met candidate_score

  best_report_path="$(phase_best_report_path "$phase_id")"
  if [[ -z "$best_report_path" ]]; then
    return 0
  fi

  best_goals_met="$(jq -r --arg phase "$phase_id" '.phaseBestResults[$phase].goalsMetCount // 0' "$STATE_FILE")"
  best_score="$(jq -r --arg phase "$phase_id" '.phaseBestResults[$phase].score // 0' "$STATE_FILE")"
  candidate_goals_met="$(jq -r '.goalsMetCount' <<<"$candidate_summary")"
  candidate_score="$(jq -r '.score' <<<"$candidate_summary")"

  if (( candidate_goals_met > best_goals_met )); then
    return 0
  fi
  if (( candidate_goals_met < best_goals_met )); then
    return 1
  fi
  if awk -v c="$candidate_score" -v b="$best_score" 'BEGIN { exit !(c > b + 1e-9) }'; then
    return 0
  fi
  return 1
}

update_phase_best() {
  local phase_id="$1"
  local iteration="$2"
  local report_path="$3"
  local manifest_path="$4"
  local summary_json="$5"
  local now
  now="$(date -Iseconds)"
  json_update "$STATE_FILE" \
    --arg phase "$phase_id" \
    --arg report "$report_path" \
    --arg manifest "$manifest_path" \
    --argjson iteration "$iteration" \
    --arg now "$now" \
    --argjson summary "$summary_json" '
      .phaseBestResults[$phase] = {
        reportPath: $report,
        manifestPath: $manifest,
        iteration: $iteration,
        updatedAt: $now,
        goalsMetCount: $summary.goalsMetCount,
        goalsTotal: $summary.goalsTotal,
        score: $summary.score,
        metrics: $summary.metrics
      }
    '
}

snapshot_overall_baseline() {
  local report_path="$1"
  json_update "$STATE_FILE" --arg report "$report_path" '.approvedBaselines.overall.reportPath = $report'
}

mark_phase_passed() {
  local phase_id="$1"
  local iteration="$2"
  local report_path="$3"
  local now
  now="$(date -Iseconds)"
  snapshot_overall_baseline "$report_path"
  json_update "$STATE_FILE" \
    --arg phase "$phase_id" \
    --arg report "$report_path" \
    --arg now "$now" \
    --argjson iteration "$iteration" '
      .approvedBaselines.phases[$phase].approved = true |
      .approvedBaselines.phases[$phase].approvedAt = $now |
      .approvedBaselines.phases[$phase].reportPath = $report |
      .phaseProgress[$phase].nextIteration = $iteration |
      .phaseProgress[$phase].exhausted = false |
      .phaseProgress[$phase].exhaustedAt = "" |
      .phaseProgress[$phase].finalOutcome = "passed" |
      .phaseProgress[$phase].finalReportPath = $report |
      .humanDecisionRequired = false |
      .humanDecisionReason = "" |
      .status = "approved"
    '
  printf 'Phase %s passed.\n' "$phase_id"
}

mark_phase_exhausted() {
  local phase_id="$1"
  local iteration="$2"
  local best_report="$3"
  local now
  now="$(date -Iseconds)"
  json_update "$STATE_FILE" \
    --arg phase "$phase_id" \
    --arg now "$now" \
    --arg report "$best_report" \
    --argjson iteration "$iteration" '
      .phaseProgress[$phase].nextIteration = $iteration |
      .phaseProgress[$phase].exhausted = true |
      .phaseProgress[$phase].exhaustedAt = $now |
      .phaseProgress[$phase].finalOutcome = "exhausted" |
      .phaseProgress[$phase].finalReportPath = $report |
      .status = "idle"
    '
}

set_phase_next_iteration() {
  local phase_id="$1"
  local iteration="$2"
  json_update "$STATE_FILE" \
    --arg phase "$phase_id" \
    --argjson iteration "$iteration" '
      .phaseProgress[$phase].nextIteration = $iteration |
      if .currentPhaseId == $phase then .currentIteration = $iteration else . end |
      .status = "idle"
    '
}

increment_campaign_cycle() {
  json_update "$STATE_FILE" '.campaignCycle = ((.campaignCycle // 1) + 1)'
}

collect_phase_floor_ids() {
  local phase_id="$1"
  jq -r --arg phase "$phase_id" --slurpfile state "$STATE_FILE" '
    .phases[]
    | select(.id == $phase)
    | (.inheritsFloorsFrom // [])[]?
    | select($state[0].approvedBaselines.phases[.].approved == true)
  ' "$LOOP_FILE"
}

run_phase_benchmark() {
  local phase_id="$1"
  local hypotheses_file max_hypotheses results_base output manifest_path report_path
  hypotheses_file="$(jq -r --arg phase "$phase_id" '.phases[] | select(.id == $phase) | .hypothesesFile // empty' "$LOOP_FILE")"
  max_hypotheses="$(jq -r --arg phase "$phase_id" '.phases[] | select(.id == $phase) | .maxHypotheses // 10' "$LOOP_FILE")"
  results_base="$(jq -r '.resultsBase' "$LOOP_FILE")/$phase_id"

  if [[ -z "$hypotheses_file" || "$hypotheses_file" == "null" ]]; then
    set_human_required "Phase $phase_id is missing hypothesesFile in eval-loop.json."
    return 110
  fi
  if [[ ! -f "$hypotheses_file" ]]; then
    set_human_required "Phase $phase_id hypotheses file does not exist: $hypotheses_file"
    return 111
  fi

  printf 'Running inner benchmark loop for %s using %s\n' "$phase_id" "$hypotheses_file" >&2
  output="$("$INNER_LOOP" --results-base "$results_base" --hypotheses-file "$hypotheses_file" --max-hypotheses "$max_hypotheses" 2>&1)"
  printf '%s\n' "$output" >&2

  manifest_path="$(printf '%s\n' "$output" | awk -F': ' '/^Manifest:/ {print $2}' | tail -n1)"
  report_path="$(printf '%s\n' "$output" | awk -F': ' '/^Final loop report:/ {print $2}' | tail -n1)"

  if [[ -z "$manifest_path" || -z "$report_path" ]]; then
    printf 'Could not parse manifest/report paths from inner loop output for %s.\n' "$phase_id" >&2
    return 112
  fi

  report_path="${report_path%.md}.json"
  require_file "$manifest_path"
  require_file "$report_path"
  json_update "$STATE_FILE" --arg phase "$phase_id" --arg report "$report_path" --arg manifest "$manifest_path" --arg ts "$(date -Iseconds)" '.lastRun = {phaseId: $phase, reportPath: $report, manifestPath: $manifest, timestamp: $ts}'
  printf '%s|%s\n' "$manifest_path" "$report_path"
}

evaluate_current_phase() {
  require_state
  local phase_id="$1"
  local iteration run_info manifest_path report_path summary_json best_updated=false
  phase_id="$1"
  iteration="$(phase_iteration "$phase_id")"
  json_update "$STATE_FILE" --arg phase "$phase_id" --argjson iteration "$iteration" '.currentPhaseId = $phase | .currentIteration = $iteration | .status = "running"'

  if [[ "$(human_required)" == "true" ]]; then
    printf 'Cannot run while human decision is required.\n' >&2
    return 100
  fi

  run_info="$(run_phase_benchmark "$phase_id")"
  manifest_path="${run_info%%|*}"
  report_path="${run_info#*|}"

  while IFS= read -r floor_phase; do
    [[ -z "$floor_phase" ]] && continue
    while IFS= read -r metric; do
      [[ -z "$metric" ]] && continue
      local rule target actual
      rule="$(metric_rule "$metric")"
      target="$(jq -r --arg phase "$floor_phase" --arg metric "$metric" '.phases[] | select(.id == $phase) | .preservedMetricFloors[$metric] // empty' "$LOOP_FILE")"
      actual="$(metric_value "$report_path" "$metric")"
      if [[ -z "$actual" || "$actual" == "null" ]]; then
        set_human_required "Missing value for preserved floor metric '$metric' in $floor_phase."
        record_history "$phase_id" "$iteration" "$report_path" "$manifest_path" "stopped_missing_floor_metric"
        return 101
      fi
      if ! compare_metric "$rule" "$actual" "$target"; then
        set_human_required "Preserved floor regression: metric '$metric' from $floor_phase regressed during $phase_id."
        record_history "$phase_id" "$iteration" "$report_path" "$manifest_path" "stopped_preserved_floor_regression"
        return 102
      fi
    done < <(jq -r --arg phase "$floor_phase" '.phases[] | select(.id == $phase) | .preservedMetricFloors | keys[]' "$LOOP_FILE")
  done < <(collect_phase_floor_ids "$phase_id")

  local goals_ok=true
  while IFS= read -r metric; do
    [[ -z "$metric" ]] && continue
    local rule target actual
    rule="$(metric_rule "$metric")"
    target="$(jq -r --arg phase "$phase_id" --arg metric "$metric" '.phases[] | select(.id == $phase) | .goalMetrics[$metric] // empty' "$LOOP_FILE")"
    actual="$(metric_value "$report_path" "$metric")"
    if [[ -z "$actual" || "$actual" == "null" ]]; then
      set_human_required "Missing value for phase goal metric '$metric' in $phase_id."
      record_history "$phase_id" "$iteration" "$report_path" "$manifest_path" "stopped_missing_goal_metric"
      return 103
    fi
    if ! compare_metric "$rule" "$actual" "$target"; then
      goals_ok=false
    fi
  done < <(phase_goal_keys "$phase_id")

  summary_json="$(build_phase_result_summary "$phase_id" "$report_path")"
  if phase_best_improved "$phase_id" "$summary_json"; then
    update_phase_best "$phase_id" "$iteration" "$report_path" "$manifest_path" "$summary_json"
    best_updated=true
  fi

  if [[ "$goals_ok" == "true" ]]; then
    if [[ "$best_updated" != "true" ]]; then
      update_phase_best "$phase_id" "$iteration" "$report_path" "$manifest_path" "$summary_json"
    fi
    mark_phase_passed "$phase_id" "$iteration" "$report_path"
    record_history "$phase_id" "$iteration" "$report_path" "$manifest_path" "phase_passed_auto_advanced"
    return 0
  fi

  local max_iterations best_report
  max_iterations="$(jq -r --arg phase "$phase_id" '.phases[] | select(.id == $phase) | .iterationPolicy.maxIterations' "$LOOP_FILE")"
  if (( iteration >= max_iterations )); then
    best_report="$(phase_best_report_path "$phase_id")"
    mark_phase_exhausted "$phase_id" "$iteration" "$best_report"
    record_history "$phase_id" "$iteration" "$report_path" "$manifest_path" "phase_exhausted_preserved_phase_best"
    printf 'Phase %s exhausted at iteration %s / %s. Preserved best report: %s\n' "$phase_id" "$iteration" "$max_iterations" "$best_report"
    return 21
  fi

  set_phase_next_iteration "$phase_id" "$((iteration + 1))"
  if [[ "$best_updated" == "true" ]]; then
    record_history "$phase_id" "$iteration" "$report_path" "$manifest_path" "iteration_complete_repeat_phase_updated_best"
  else
    record_history "$phase_id" "$iteration" "$report_path" "$manifest_path" "iteration_complete_repeat_phase_kept_best"
  fi
  printf 'Phase goals not reached for %s. Next iteration for this phase: %s.\n' "$phase_id" "$((iteration + 1))"
  return 20
}

print_status() {
  require_state
  ensure_state_shape
  local phase_id reason
  phase_id="$(current_phase_id)"
  reason="$(json_get "$STATE_FILE" '.humanDecisionReason')"
  printf 'State file: %s\n' "$STATE_FILE"
  printf 'Campaign cycle: %s\n' "$(campaign_cycle)"
  printf 'Current phase: %s\n' "$phase_id"
  printf 'Current iteration: %s / %s\n' "$(phase_iteration "$phase_id")" "$(json_get "$STATE_FILE" '.maxIterationsPerPhase')"
  printf 'Status: %s\n' "$(json_get "$STATE_FILE" '.status')"
  printf 'Human decision required: %s\n' "$(human_required)"
  if [[ "$reason" != "" && "$reason" != "null" ]]; then
    printf 'Reason: %s\n' "$reason"
  fi
  printf 'Current phase best report: %s\n' "$(phase_best_report_path "$phase_id")"
  printf 'Phase terminal state:\n'
  jq -r --slurpfile loop "$LOOP_FILE" '
    $loop[0].phaseOrder[] as $phase |
    "  " + $phase + ": approved=" + ((.approvedBaselines.phases[$phase].approved // false) | tostring) +
    ", exhausted=" + ((.phaseProgress[$phase].exhausted // false) | tostring) +
    ", nextIteration=" + ((.phaseProgress[$phase].nextIteration // 1) | tostring)
  ' "$STATE_FILE"
}

print_final_comparison() {
  local original_phase final_phase original_report final_report
  original_phase="$(jq -r '.phaseOrder[0]' "$LOOP_FILE")"
  original_report="$(jq -r --arg phase "$original_phase" '.approvedBaselines.phases[$phase].reportPath // empty' "$STATE_FILE")"
  final_phase=""

  while IFS= read -r phase_id; do
    [[ -z "$phase_id" ]] && continue
    if [[ "$(phase_passed "$phase_id")" == "true" ]]; then
      final_phase="$phase_id"
    elif [[ "$(phase_exhausted "$phase_id")" == "true" && -n "$(phase_best_report_path "$phase_id")" ]]; then
      final_phase="$phase_id"
    fi
  done < <(jq -r '.phaseOrder[]' "$LOOP_FILE")

  if [[ -n "$final_phase" && "$(phase_passed "$final_phase")" == "true" ]]; then
    final_report="$(jq -r --arg phase "$final_phase" '.approvedBaselines.phases[$phase].reportPath // empty' "$STATE_FILE")"
  elif [[ -n "$final_phase" ]]; then
    final_report="$(phase_best_report_path "$final_phase")"
  else
    final_report=""
  fi

  if [[ -z "$original_report" || -z "$final_report" || ! -f "$original_report" || ! -f "$final_report" ]]; then
    return 0
  fi

  printf '\nFinal campaign comparison (%s -> %s):\n' "$original_phase" "$final_phase"
  jq -r '
    [
      ["balanced_gate", .balanced_gate, input.balanced_gate],
      ["query.weighted_score", (.before_after.query.current.weighted_score | tostring), (input.before_after.query.current.weighted_score | tostring)],
      ["query.reverse_lookup_mean_f1", (.before_after.query.current.reverse_lookup_mean_f1 | tostring), (input.before_after.query.current.reverse_lookup_mean_f1 | tostring)],
      ["query.path_node_precision", (.before_after.query.current.path_node_precision | tostring), (input.before_after.query.current.path_node_precision | tostring)],
      ["query.path_node_recall", (.before_after.query.current.path_node_recall | tostring), (input.before_after.query.current.path_node_recall | tostring)],
      ["full_vela.layer1_weighted_score", (.before_after.full_vela.current.layer1_weighted_score | tostring), (input.before_after.full_vela.current.layer1_weighted_score | tostring)],
      ["full_stock.layer2_weighted_score", (.before_after.full_stock.current.layer2_weighted_score | tostring), (input.before_after.full_stock.current.layer2_weighted_score | tostring)]
    ][] | "  " + .[0] + ": " + .[1] + " -> " + .[2]
  ' "$original_report" "$final_report"
}

run_loop() {
  require_state
  ensure_state_shape
  if [[ "$(human_required)" == "true" ]]; then
    printf 'Cannot run while human decision is required.\n' >&2
    exit 1
  fi

  while true; do
    if [[ "$(json_get "$STATE_FILE" '.status')" == "complete" ]]; then
      print_final_comparison
      exit 0
    fi

    local cycle rc ran_phase=false phase_id
    cycle="$(campaign_cycle)"
    printf '\nStarting campaign cycle %s\n' "$cycle"

    while IFS= read -r phase_id; do
      [[ -z "$phase_id" ]] && continue
      if [[ "$(phase_passed "$phase_id")" == "true" ]]; then
        printf 'Skipping %s: already passed.\n' "$phase_id"
        continue
      fi
      if [[ "$(phase_exhausted "$phase_id")" == "true" ]]; then
        printf 'Skipping %s: already exhausted with preserved best result.\n' "$phase_id"
        continue
      fi

      ran_phase=true
      if evaluate_current_phase "$phase_id"; then
        rc=0
      else
        rc=$?
      fi

      case "$rc" in
        0)
          printf 'Finished %s for cycle %s: passed.\n' "$phase_id" "$cycle"
          ;;
        20)
          printf 'Finished %s for cycle %s: iteration recorded.\n' "$phase_id" "$cycle"
          ;;
        21)
          printf 'Finished %s for cycle %s: exhausted.\n' "$phase_id" "$cycle"
          ;;
        *)
          exit "$rc"
          ;;
      esac
    done < <(jq -r '.phaseOrder[]' "$LOOP_FILE")

    sync_current_phase_cursor
    if [[ "$(json_get "$STATE_FILE" '.status')" == "complete" ]]; then
      printf '\nCampaign complete: all phases are passed or exhausted.\n'
      print_final_comparison
      exit 0
    fi
    if [[ "$ran_phase" != "true" ]]; then
      printf 'No runnable phases remain.\n'
      print_final_comparison
      exit 0
    fi

    increment_campaign_cycle
    sync_current_phase_cursor
  done
}

cmd="${1:-}"
case "$cmd" in
  init)
    init_state
    ;;
  status)
    print_status
    ;;
  run)
    run_loop
    ;;
  require-human)
    shift
    require_state
    set_human_required "$*"
    ;;
  clear-human)
    clear_human_required
    ;;
  *)
    usage
    exit 1
    ;;
esac
