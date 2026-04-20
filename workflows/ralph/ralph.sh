#!/usr/bin/env bash
set -euo pipefail

# Vela Ralph workflow runner
#
# Usage:
#   ./workflows/ralph/ralph.sh
#   ./workflows/ralph/ralph.sh --status
#   ./workflows/ralph/ralph.sh --all
#   ./workflows/ralph/ralph.sh --story 3
#   ./workflows/ralph/ralph.sh --tool opencode

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORKDIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
PRD="$SCRIPT_DIR/prd.json"
PROGRESS="$SCRIPT_DIR/progress.txt"
ARCHIVE_DIR="$SCRIPT_DIR/archive"

TOOL="auto"
RUN_ALL=0
STATUS_ONLY=0
STORY_PRIORITY=""

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

die()  { printf "%b\n" "${RED}ERROR:${NC} $*" >&2; exit 1; }
info() { printf "%b\n" "${YELLOW}[ralph]${NC} $*"; }
ok()   { printf "%b\n" "${GREEN}[ralph]${NC} $*"; }

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "'$1' not found. Install it first."
}

pick_tool() {
  if [[ "$TOOL" != "auto" ]]; then
    command -v "$TOOL" >/dev/null 2>&1 || die "requested tool '$TOOL' is not installed"
    printf "%s" "$TOOL"
    return
  fi

  if command -v opencode >/dev/null 2>&1; then
    printf "%s" "opencode"
    return
  fi
  if command -v claude >/dev/null 2>&1; then
    printf "%s" "claude"
    return
  fi
  if command -v amp >/dev/null 2>&1; then
    printf "%s" "amp"
    return
  fi

  die "no supported agent CLI found. Install one of: opencode, claude, amp"
}

show_usage() {
  cat <<'EOF'
Usage: ./workflows/ralph/ralph.sh [options]

Options:
  --status           Show story status only
  --all              Run all pending stories in order
  --story <priority> Run a specific story by priority number
  --tool <name>      Agent CLI to use: auto | opencode | claude | amp
  -h, --help         Show this help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --status)
      STATUS_ONLY=1
      shift
      ;;
    --all)
      RUN_ALL=1
      shift
      ;;
    --story)
      [[ -n "${2:-}" ]] || die "--story requires a priority number"
      STORY_PRIORITY="$2"
      shift 2
      ;;
    --tool)
      [[ -n "${2:-}" ]] || die "--tool requires a value"
      TOOL="$2"
      shift 2
      ;;
    -h|--help)
      show_usage
      exit 0
      ;;
    *)
      die "unknown argument: $1"
      ;;
  esac
done

require_cmd jq
[[ -f "$PRD" ]] || die "prd.json not found at $PRD"

show_status() {
  local total done pending
  total=$(jq '.userStories | length' "$PRD")
  done=$(jq '[.userStories[] | select(.passes == true)] | length' "$PRD")
  pending=$(jq '[.userStories[] | select(.passes == false)] | length' "$PRD")

  printf "\nProject : %s\n" "$(jq -r '.project' "$PRD")"
  printf "Branch  : %s\n" "$(jq -r '.branchName' "$PRD")"
  printf "Stories : %s total | %b%s passed%b | %b%s pending%b\n\n" "$total" "$GREEN" "$done" "$NC" "$YELLOW" "$pending" "$NC"
  jq -r '.userStories[] | "[\(.passes | if . then "✓" else "○" end)] US-\(.priority | tostring | if length == 1 then "00" + . elif length == 2 then "0" + . else . end): \(.title)"' "$PRD"
  printf "\n"
}

get_next_story_index() {
  jq 'first(.userStories | to_entries[] | select(.value.passes == false) | .key) // empty' "$PRD"
}

get_story_by_priority() {
  local priority="$1"
  jq ".userStories | to_entries[] | select(.value.priority == ${priority}) | .key" "$PRD"
}

ensure_branch() {
  local expected current
  expected=$(jq -r '.branchName' "$PRD")
  current=$(git rev-parse --abbrev-ref HEAD)

  if [[ "$current" == "$expected" ]]; then
    return
  fi

  if git show-ref --verify --quiet "refs/heads/$expected"; then
    info "checking out existing branch $expected"
    git checkout "$expected"
  else
    info "creating branch $expected from current HEAD"
    git checkout -b "$expected"
  fi
}

maybe_archive() {
  [[ -f "$PROGRESS" ]] || return
  if ! grep -q '^## ' "$PROGRESS" 2>/dev/null; then
    return
  fi

  local branch date_str feature archive_dir
  branch=$(jq -r '.branchName' "$PRD")
  date_str=$(date +%Y-%m-%d)
  feature=$(printf "%s" "$branch" | sed 's|/|-|g')
  archive_dir="$ARCHIVE_DIR/$date_str-$feature"
  mkdir -p "$archive_dir"
  cp "$PRD" "$archive_dir/prd.json"
  cp "$PROGRESS" "$archive_dir/progress.txt"
  info "archived previous Ralph progress to $archive_dir"
}

ensure_progress_file() {
  if [[ -f "$PROGRESS" ]]; then
    return
  fi

  cat > "$PROGRESS" <<EOF
# Ralph Progress - $(jq -r '.project' "$PRD")
# Branch: $(jq -r '.branchName' "$PRD")
# Started: $(date +%Y-%m-%d)

## Codebase Patterns
---
EOF
}

story_prompt() {
  local idx="$1"
  jq -r --argjson idx "$idx" '
    .userStories[$idx] as $s |
    "You are executing one Ralph workflow story for the Vela repository.\n\n" +
    "Repository: " + .project + "\n" +
    "Working directory: " + env.WORKDIR + "\n" +
    "Required branch: " + .branchName + "\n\n" +
    "Story: " + $s.id + " - " + $s.title + "\n\n" +
    "Description:\n" + $s.description + "\n\n" +
    "Acceptance Criteria:\n" + ($s.acceptanceCriteria | map("- " + .) | join("\n")) + "\n\n" +
    (if ($s.notes // "") != "" then "Notes:\n" + $s.notes + "\n\n" else "" end) +
     "Rules:\n" +
     "- Work on THIS story only.\n" +
     "- Read workflows/ralph/progress.txt before editing.\n" +
     "- Work synchronously in THIS session only. Do NOT delegate, launch sub-agents, or wait on background tasks.\n" +
     "- Do the code reading, editing, and testing yourself instead of handing off the work.\n" +
     "- Keep memory, contract, workspace, and repo responsibilities separate.\n" +
     "- Run relevant tests for the touched areas and then run go test ./... if feasible.\n" +
     "- Do not create a commit unless explicitly requested by the human operator.\n" +
     "- If the story passes, update workflows/ralph/prd.json to set passes=true for this story and append a progress entry to workflows/ralph/progress.txt.\n" +
     "- If the story does not pass, leave passes=false and append retry notes to workflows/ralph/progress.txt.\n\n" +
    "When you finish, print exactly one of:\n" +
    "STORY_COMPLETE: " + $s.id + "\n" +
    "or\n" +
    "STORY_BLOCKED: " + $s.id
  ' "$PRD"
}

run_agent() {
  local tool="$1"
  local prompt="$2"

  case "$tool" in
    opencode)
      opencode run --agent "G33N-RALPH" --model "openai/gpt-5.4" --dangerously-skip-permissions --dir "$WORKDIR" "$prompt" 2>&1
      ;;
    claude)
      printf "%s" "$prompt" | claude --print --dangerously-skip-permissions --add-dir "$WORKDIR" 2>&1
      ;;
    amp)
      printf "%s" "$prompt" | amp --dangerously-allow-all 2>&1
      ;;
    *)
      die "unsupported tool: $tool"
      ;;
  esac
}

append_progress() {
  local idx="$1"
  local status="$2"
  cat >> "$PROGRESS" <<EOF

## $(date '+%Y-%m-%d %H:%M:%S') - $(jq -r ".userStories[$idx].id" "$PRD")
- Status: $status
- Title: $(jq -r ".userStories[$idx].title" "$PRD")
---
EOF
}

mark_story_passed() {
  local idx="$1"
  local tmp
  tmp=$(mktemp)
  jq ".userStories[$idx].passes = true" "$PRD" > "$tmp"
  mv "$tmp" "$PRD"
}

run_story() {
  local idx="$1"
  local tool prompt output story_id
  tool=$(pick_tool)
  prompt=$(WORKDIR="$WORKDIR" story_prompt "$idx")
  story_id=$(jq -r ".userStories[$idx].id" "$PRD")

  info "running $story_id with $tool"
  output=$(run_agent "$tool" "$prompt" | tee -a "$PROGRESS") || true

  if grep -q "STORY_COMPLETE: $story_id" <<<"$output"; then
    mark_story_passed "$idx"
    append_progress "$idx" "passed"
    ok "$story_id marked as passed"
    return
  fi

  append_progress "$idx" "blocked"
  info "$story_id did not report completion; inspect progress.txt"
}

main() {
  ensure_branch
  ensure_progress_file

  if [[ "$STATUS_ONLY" -eq 1 ]]; then
    show_status
    return
  fi

  if [[ "$RUN_ALL" -eq 1 ]]; then
    while true; do
      local_idx=$(get_next_story_index)
      [[ -z "$local_idx" ]] && { ok "all stories complete"; show_status; return; }
      run_story "$local_idx"
    done
  fi

  if [[ -n "$STORY_PRIORITY" ]]; then
    local_idx=$(get_story_by_priority "$STORY_PRIORITY")
    [[ -n "$local_idx" ]] || die "no story found with priority $STORY_PRIORITY"
    run_story "$local_idx"
    return
  fi

  local_idx=$(get_next_story_index)
  [[ -n "$local_idx" ]] || { ok "all stories complete"; show_status; return; }
  run_story "$local_idx"
}

main
