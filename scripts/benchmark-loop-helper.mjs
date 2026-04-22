#!/usr/bin/env node

import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { spawnSync } from "node:child_process";

const BASELINE_FULL_VELA = "/home/geen/Documents/personal/dep-eval/results/2026-04-22T19-47-51-916Z-vela-vela-vs-vela-next-sim-vs-graphify/report.json";
const BASELINE_FULL_STOCK = "/home/geen/Documents/personal/dep-eval/results/2026-04-22T21-01-05-592Z-stock-chef-vela-vs-vela-next-sim-vs-graphify/report.json";
const BASELINE_QUERY = "/home/geen/Documents/personal/dep-eval/results/2026-04-22T19-47-51-911Z-graph-query-code-v1-vela-vs-vela-next-sim-vs-graphify/graph-query-report.json";

const LOOP_TARGETS = {
  fullVela: {
    build_time_ms_max: 2000,
    file_coverage_ratio_min: 0.9897959183673469,
    symbol_coverage_ratio_min: 1.0,
    unresolved_reference_rate_max: 0,
    layer1_weighted_score_min: 0.4992,
  },
  fullStock: {
    layer1_weighted_score_min: 0.35,
    layer2_weighted_score_min: 0.868,
  },
  query: {
    weighted_score_min: 0.7585,
    reverse_lookup_mean_f1_min: 0.4533,
    path_node_precision_min: 0.5667,
    path_node_recall_min: 0.5378,
    path_tracing_path_node_recall_min: 0.8333,
  },
};

function parseArgs(argv) {
  const args = { _: [] };
  for (let i = 0; i < argv.length; i += 1) {
    const value = argv[i];
    if (!value.startsWith("--")) {
      args._.push(value);
      continue;
    }
    const key = value.slice(2).replace(/-/g, "_");
    const next = argv[i + 1];
    if (next === undefined || next.startsWith("--")) {
      args[key] = true;
      continue;
    }
    args[key] = next;
    i += 1;
  }
  return args;
}

function readJson(filePath) {
  return JSON.parse(fs.readFileSync(filePath, "utf8"));
}

function writeJson(filePath, value) {
  fs.mkdirSync(path.dirname(filePath), { recursive: true });
  fs.writeFileSync(filePath, `${JSON.stringify(value, null, 2)}\n`);
}

function writeText(filePath, value) {
  fs.mkdirSync(path.dirname(filePath), { recursive: true });
  fs.writeFileSync(filePath, value);
}

function loadHypotheses(filePath) {
  if (!filePath) {
    return [];
  }
  const hypotheses = readJson(filePath);
  if (!Array.isArray(hypotheses)) {
    throw new Error(`Expected an array of hypotheses in ${filePath}`);
  }
  return hypotheses;
}

function getHypothesis(hypotheses, id) {
  const hypothesis = hypotheses.find((item) => item.id === id);
  if (!hypothesis) {
    throw new Error(`Unknown hypothesis id: ${id}`);
  }
  return hypothesis;
}

function querySnapshot(reportPath) {
  const report = readJson(reportPath);
  const summary = report.adapter_summaries.vela;
  const families = report.family_summaries.vela;
  return {
    report_path: reportPath,
    weighted_score: summary.weighted_score,
    success_rate: summary.success_rate,
    unsupported_rate: summary.unsupported_rate,
    path_node_precision: summary.path_node_precision,
    path_node_recall: summary.path_node_recall,
    reverse_lookup_mean_f1: families["reverse-lookup"]?.mean_f1 ?? 0,
    reverse_lookup_mean_precision: families["reverse-lookup"]?.mean_precision ?? 0,
    reverse_lookup_mean_recall: families["reverse-lookup"]?.mean_recall ?? 0,
    path_tracing_path_node_recall: families["path-tracing"]?.path_node_recall ?? 0,
  };
}

function fullSnapshot(reportPath) {
  const report = readJson(reportPath);
  const layer1 = report.layer1.adapter_metrics.vela;
  const layer2 = report.layer2.per_task_type.vela;
  return {
    report_path: reportPath,
    build_success: layer1.build_success,
    build_time_ms: layer1.build_time_ms,
    file_coverage_ratio: layer1.file_coverage_ratio,
    symbol_coverage_ratio: layer1.symbol_coverage_ratio,
    unresolved_reference_rate: layer1.unresolved_reference_rate,
    incremental_update_score: report.layer1.normalized_metric_scores.incremental_update_score.vela,
    layer1_weighted_score: report.layer1.overall_scores.vela.weighted_score,
    layer2_weighted_score: report.layer2.overall_scores.vela.weighted_score,
    reverse_dependency_mean_f1: layer2.reverse_dependency?.mean_f1 ?? 0,
    path_mean_f1: layer2.path?.mean_f1 ?? 0,
  };
}

function metricFromQuerySnapshot(snapshot, key) {
  if (key.startsWith("summary.")) {
    return snapshot[key.slice("summary.".length)];
  }
  if (key.startsWith("family.reverse-lookup.")) {
    return snapshot[`reverse_lookup_${key.slice("family.reverse-lookup.".length)}`];
  }
  if (key === "family.path-tracing.path_node_recall") {
    return snapshot.path_tracing_path_node_recall;
  }
  throw new Error(`Unsupported focused gate metric: ${key}`);
}

function delta(current, baseline) {
  return Number((current - baseline).toFixed(6));
}

function noRegression(current, baseline) {
  return current >= baseline;
}

function buildDeficits(currentQuery) {
  return [
    {
      family: "reverse-lookup",
      severity: LOOP_TARGETS.query.reverse_lookup_mean_f1_min - currentQuery.reverse_lookup_mean_f1,
      reason: `reverse-lookup mean_f1 is ${currentQuery.reverse_lookup_mean_f1.toFixed(4)} vs target ${LOOP_TARGETS.query.reverse_lookup_mean_f1_min.toFixed(4)}`,
    },
    {
      family: "path-quality",
      severity: (LOOP_TARGETS.query.path_node_precision_min - currentQuery.path_node_precision) +
        (LOOP_TARGETS.query.path_node_recall_min - currentQuery.path_node_recall),
      reason: `path node precision/recall are ${currentQuery.path_node_precision.toFixed(4)} / ${currentQuery.path_node_recall.toFixed(4)} vs targets ${LOOP_TARGETS.query.path_node_precision_min.toFixed(4)} / ${LOOP_TARGETS.query.path_node_recall_min.toFixed(4)}`,
    },
    {
      family: "path-tracing",
      severity: LOOP_TARGETS.query.path_tracing_path_node_recall_min - currentQuery.path_tracing_path_node_recall,
      reason: `path-tracing node recall is ${currentQuery.path_tracing_path_node_recall.toFixed(4)} vs target ${LOOP_TARGETS.query.path_tracing_path_node_recall_min.toFixed(4)}`,
    },
  ].sort((a, b) => b.severity - a.severity);
}

function renderPlanMarkdown(plan) {
  const lines = [
    "# Benchmark Improvement Plan",
    "",
    "## Current Query Snapshot",
    `- Weighted score: ${plan.current_query.weighted_score.toFixed(4)}`,
    `- Reverse lookup mean_f1: ${plan.current_query.reverse_lookup_mean_f1.toFixed(4)}`,
    `- Path node precision / recall: ${plan.current_query.path_node_precision.toFixed(4)} / ${plan.current_query.path_node_recall.toFixed(4)}`,
    `- Path-tracing node recall: ${plan.current_query.path_tracing_path_node_recall.toFixed(4)}`,
    "",
    "## Ranked Gaps",
  ];
  for (const gap of plan.gaps) {
    lines.push(`- ${gap.family}: ${gap.reason}`);
  }
  lines.push("", "## Ranked Hypotheses");
  if (plan.ranked_hypotheses.length === 0) {
    lines.push("- No actionable hypotheses were provided.");
  }
  for (const item of plan.ranked_hypotheses) {
    const actionable = item.actionable ? "actionable" : "missing apply/revert";
    lines.push(`- ${item.id}: ${item.title} [${actionable}]`);
    lines.push(`  Family: ${item.family}`);
    lines.push(`  Why now: ${item.why_now}`);
    lines.push(`  Inspired by: ${item.inspired_by}`);
    lines.push(`  Risk: ${item.risk}`);
  }
  return `${lines.join("\n")}\n`;
}

function suggestCommand(args) {
  const currentQuery = querySnapshot(args.current_query_report);
  const hypotheses = loadHypotheses(args.hypotheses_file);
  const gaps = buildDeficits(currentQuery);
  const ranked = hypotheses.map((hypothesis) => {
    const matchingGap = gaps.find((gap) => gap.family === hypothesis.family) ?? gaps[gaps.length - 1];
    const actionable = Boolean(hypothesis.apply && hypothesis.revert && hypothesis.focused_gate);
    return {
      id: hypothesis.id,
      title: hypothesis.title,
      family: hypothesis.family,
      inspired_by: hypothesis.inspired_by,
      risk: hypothesis.risk,
      actionable,
      why_now: matchingGap.reason,
      sort_score: matchingGap.severity,
    };
  }).sort((a, b) => b.sort_score - a.sort_score);

  const plan = {
    generated_at: new Date().toISOString(),
    current_query: currentQuery,
    gaps,
    ranked_hypotheses: ranked,
    actionable_ids: ranked.filter((item) => item.actionable).map((item) => item.id),
  };

  if (args.out_json) {
    writeJson(args.out_json, plan);
  }
  if (args.out_md) {
    writeText(args.out_md, renderPlanMarkdown(plan));
  }
  process.stdout.write(`${JSON.stringify(plan, null, 2)}\n`);
}

function runCommandCommand(args) {
  const hypotheses = loadHypotheses(args.hypotheses_file);
  const hypothesis = getHypothesis(hypotheses, args.hypothesis_id);
  const command = hypothesis[args.mode];
  if (!command) {
    throw new Error(`Hypothesis ${hypothesis.id} is missing ${args.mode} command`);
  }
  const result = spawnSync("bash", ["-lc", command], {
    cwd: args.workdir,
    encoding: "utf8",
  });
  if (args.log_path) {
    writeText(args.log_path, `${result.stdout ?? ""}${result.stderr ?? ""}`);
  }
  if (result.status !== 0) {
    process.stderr.write(result.stderr ?? "");
    process.exit(result.status ?? 1);
  }
}

function focusedGateCommand(args) {
  const hypotheses = loadHypotheses(args.hypotheses_file);
  const hypothesis = getHypothesis(hypotheses, args.hypothesis_id);
  const baseline = querySnapshot(args.baseline_query_report);
  const current = querySnapshot(args.current_query_report);
  const gate = hypothesis.focused_gate;
  const metricResults = gate.metrics.map((metric) => {
    const baselineValue = metricFromQuerySnapshot(baseline, metric);
    const currentValue = metricFromQuerySnapshot(current, metric);
    return {
      metric,
      baseline: baselineValue,
      current: currentValue,
      delta: delta(currentValue, baselineValue),
      improved: currentValue > baselineValue,
    };
  });
  const improvedCount = metricResults.filter((item) => item.improved).length;
  const metricPass = gate.compare === "all-improve"
    ? improvedCount === metricResults.length
    : improvedCount >= 1;
  const summaryPass = noRegression(current.weighted_score, baseline.weighted_score) && current.success_rate === 1 && current.unsupported_rate === 0;
  const result = {
    hypothesis_id: hypothesis.id,
    title: hypothesis.title,
    family: hypothesis.family,
    inspired_by: hypothesis.inspired_by,
    risk: hypothesis.risk,
    status: metricPass && summaryPass ? "PASS" : "NOT PASS",
    baseline_query_report: args.baseline_query_report,
    current_query_report: args.current_query_report,
    checks: {
      metric_compare: gate.compare,
      metrics: metricResults,
      weighted_score_no_regression: {
        baseline: baseline.weighted_score,
        current: current.weighted_score,
        passed: noRegression(current.weighted_score, baseline.weighted_score),
      },
      success_rate_clean: current.success_rate === 1,
      unsupported_rate_clean: current.unsupported_rate === 0,
    },
  };
  writeJson(args.out_json, result);
  if (result.status !== "PASS") {
    process.exit(1);
  }
}

function recordAttemptCommand(args) {
  const hypotheses = loadHypotheses(args.hypotheses_file);
  const hypothesis = getHypothesis(hypotheses, args.hypothesis_id);
  writeJson(args.out_json, {
    hypothesis_id: hypothesis.id,
    title: hypothesis.title,
    family: hypothesis.family,
    inspired_by: hypothesis.inspired_by,
    risk: hypothesis.risk,
    status: args.status,
    reason: args.reason,
  });
}

function readAttempts(attemptsDir) {
  if (!fs.existsSync(attemptsDir)) {
    return [];
  }
  const dirs = fs.readdirSync(attemptsDir).sort();
  const attempts = [];
  for (const dirName of dirs) {
    const resultPath = path.join(attemptsDir, dirName, "result.json");
    if (!fs.existsSync(resultPath)) {
      continue;
    }
    attempts.push({
      dir: dirName,
      ...readJson(resultPath),
    });
  }
  return attempts;
}

function finalReportCommand(args) {
  const baselineFullVela = fullSnapshot(BASELINE_FULL_VELA);
  const baselineFullStock = fullSnapshot(BASELINE_FULL_STOCK);
  const baselineQuery = querySnapshot(BASELINE_QUERY);
  const currentFullVela = fullSnapshot(args.full_vela_report);
  const currentFullStock = fullSnapshot(args.full_stock_report);
  const currentQuery = querySnapshot(args.query_report);
  const attempts = readAttempts(args.attempts_dir);
  const reverseLookupImproved = currentQuery.reverse_lookup_mean_f1 > baselineQuery.reverse_lookup_mean_f1;
  const pathQualityImproved = currentQuery.path_node_precision > baselineQuery.path_node_precision ||
    currentQuery.path_node_recall > baselineQuery.path_node_recall ||
    currentQuery.path_tracing_path_node_recall > baselineQuery.path_tracing_path_node_recall;

  const gateChecks = [
    ["vela build_success", currentFullVela.build_success === true],
    ["vela build_time_ms <= 2000", currentFullVela.build_time_ms <= LOOP_TARGETS.fullVela.build_time_ms_max],
    ["vela file_coverage_ratio >= 0.9898", currentFullVela.file_coverage_ratio >= LOOP_TARGETS.fullVela.file_coverage_ratio_min],
    ["vela symbol_coverage_ratio == 1.0", currentFullVela.symbol_coverage_ratio >= LOOP_TARGETS.fullVela.symbol_coverage_ratio_min],
    ["vela unresolved_reference_rate == 0", currentFullVela.unresolved_reference_rate <= LOOP_TARGETS.fullVela.unresolved_reference_rate_max],
    ["vela layer1_weighted_score >= 0.4992", currentFullVela.layer1_weighted_score >= LOOP_TARGETS.fullVela.layer1_weighted_score_min],
    ["stock-chef layer1_weighted_score >= 0.3500", currentFullStock.layer1_weighted_score >= LOOP_TARGETS.fullStock.layer1_weighted_score_min],
    ["stock-chef layer2_weighted_score >= 0.8680", currentFullStock.layer2_weighted_score >= LOOP_TARGETS.fullStock.layer2_weighted_score_min],
    ["query weighted_score >= 0.7585", currentQuery.weighted_score >= LOOP_TARGETS.query.weighted_score_min],
    ["query reverse-lookup mean_f1 > 0.4533", currentQuery.reverse_lookup_mean_f1 > LOOP_TARGETS.query.reverse_lookup_mean_f1_min],
    ["query path_node_precision > 0.5667", currentQuery.path_node_precision > LOOP_TARGETS.query.path_node_precision_min],
    ["query path_node_recall > 0.5378", currentQuery.path_node_recall > LOOP_TARGETS.query.path_node_recall_min],
    ["reverse-lookup improved vs locked baseline", reverseLookupImproved],
    ["at least one path-quality metric improved vs locked baseline", pathQualityImproved],
  ];

  const balancedGate = gateChecks.every(([, pass]) => pass);

  const report = {
    generated_at: new Date().toISOString(),
    balanced_gate: balancedGate ? "PASS" : "NOT PASS",
    targets: LOOP_TARGETS,
    attempts,
    before_after: {
      full_vela: {
        baseline: baselineFullVela,
        current: currentFullVela,
        deltas: {
          build_time_ms: delta(currentFullVela.build_time_ms, baselineFullVela.build_time_ms),
          layer1_weighted_score: delta(currentFullVela.layer1_weighted_score, baselineFullVela.layer1_weighted_score),
          layer2_weighted_score: delta(currentFullVela.layer2_weighted_score, baselineFullVela.layer2_weighted_score),
        },
      },
      full_stock: {
        baseline: baselineFullStock,
        current: currentFullStock,
        deltas: {
          layer1_weighted_score: delta(currentFullStock.layer1_weighted_score, baselineFullStock.layer1_weighted_score),
          layer2_weighted_score: delta(currentFullStock.layer2_weighted_score, baselineFullStock.layer2_weighted_score),
        },
      },
      query: {
        baseline: baselineQuery,
        current: currentQuery,
        deltas: {
          weighted_score: delta(currentQuery.weighted_score, baselineQuery.weighted_score),
          reverse_lookup_mean_f1: delta(currentQuery.reverse_lookup_mean_f1, baselineQuery.reverse_lookup_mean_f1),
          path_node_precision: delta(currentQuery.path_node_precision, baselineQuery.path_node_precision),
          path_node_recall: delta(currentQuery.path_node_recall, baselineQuery.path_node_recall),
          path_tracing_path_node_recall: delta(currentQuery.path_tracing_path_node_recall, baselineQuery.path_tracing_path_node_recall),
        },
      },
    },
    gate_checks: gateChecks.map(([name, pass]) => ({ name, pass })),
  };

  const lines = [
    "# Benchmark Improvement Loop Report",
    "",
    `Balanced gate: ${report.balanced_gate}`,
    "",
    "## Final Gates",
  ];
  for (const check of report.gate_checks) {
    lines.push(`- ${check.pass ? "PASS" : "NOT PASS"}: ${check.name}`);
  }
  lines.push("", "## Hypotheses Attempted");
  if (attempts.length === 0) {
    lines.push("- No hypotheses were attempted.");
  }
  for (const attempt of attempts) {
    lines.push(`- ${attempt.hypothesis_id}: ${attempt.status}`);
  }
  lines.push("", "## Before / After");
  lines.push(`- Query weighted_score: ${baselineQuery.weighted_score.toFixed(4)} -> ${currentQuery.weighted_score.toFixed(4)} (${report.before_after.query.deltas.weighted_score >= 0 ? "+" : ""}${report.before_after.query.deltas.weighted_score.toFixed(4)})`);
  lines.push(`- Reverse lookup mean_f1: ${baselineQuery.reverse_lookup_mean_f1.toFixed(4)} -> ${currentQuery.reverse_lookup_mean_f1.toFixed(4)} (${report.before_after.query.deltas.reverse_lookup_mean_f1 >= 0 ? "+" : ""}${report.before_after.query.deltas.reverse_lookup_mean_f1.toFixed(4)})`);
  lines.push(`- Path node precision: ${baselineQuery.path_node_precision.toFixed(4)} -> ${currentQuery.path_node_precision.toFixed(4)} (${report.before_after.query.deltas.path_node_precision >= 0 ? "+" : ""}${report.before_after.query.deltas.path_node_precision.toFixed(4)})`);
  lines.push(`- Path node recall: ${baselineQuery.path_node_recall.toFixed(4)} -> ${currentQuery.path_node_recall.toFixed(4)} (${report.before_after.query.deltas.path_node_recall >= 0 ? "+" : ""}${report.before_after.query.deltas.path_node_recall.toFixed(4)})`);
  lines.push(`- Path-tracing node recall: ${baselineQuery.path_tracing_path_node_recall.toFixed(4)} -> ${currentQuery.path_tracing_path_node_recall.toFixed(4)} (${report.before_after.query.deltas.path_tracing_path_node_recall >= 0 ? "+" : ""}${report.before_after.query.deltas.path_tracing_path_node_recall.toFixed(4)})`);
  lines.push(`- Vela layer1 weighted_score: ${baselineFullVela.layer1_weighted_score.toFixed(4)} -> ${currentFullVela.layer1_weighted_score.toFixed(4)} (${report.before_after.full_vela.deltas.layer1_weighted_score >= 0 ? "+" : ""}${report.before_after.full_vela.deltas.layer1_weighted_score.toFixed(4)})`);
  lines.push(`- Stock-chef layer2 weighted_score: ${baselineFullStock.layer2_weighted_score.toFixed(4)} -> ${currentFullStock.layer2_weighted_score.toFixed(4)} (${report.before_after.full_stock.deltas.layer2_weighted_score >= 0 ? "+" : ""}${report.before_after.full_stock.deltas.layer2_weighted_score.toFixed(4)})`);

  writeJson(args.out_json, report);
  writeText(args.out_md, `${lines.join("\n")}\n`);
}

const args = parseArgs(process.argv.slice(2));
const command = args._[0];

switch (command) {
  case "suggest":
    suggestCommand(args);
    break;
  case "run-command":
    runCommandCommand(args);
    break;
  case "focused-gate":
    focusedGateCommand(args);
    break;
  case "record-attempt":
    recordAttemptCommand(args);
    break;
  case "final-report":
    finalReportCommand(args);
    break;
  default:
    throw new Error(`Unknown command: ${command}`);
}
