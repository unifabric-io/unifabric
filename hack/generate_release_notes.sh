#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  generate_release_notes.sh --repo <owner/repo> --current-tag <tag> --output <file> \
    --gh-token <token> --copilot-token <token> [--error-log-file <file>] [--allow-ai-fail]

Required flags:
  --repo: Target repository (owner/repo format)
  --current-tag: Tag to generate release notes for
  --output: Output file path
  --gh-token: GitHub token for API access
  --copilot-token: GitHub Copilot token for AI summaries

Notes:
  - This script uses only GitHub API (gh cli), no local git repository required.
  - Stable: pick the previous stable tag (skip -rcN/-rN).
  - RC:
      - vX.Y.Z-rcN -> prefer vX.Y.Z-rc(N-1) (also accepts -r(N-1))
      - if not found (or rc0): fallback to latest stable tag with lower minor (same major) or lower major.
EOF
}

CURRENT_TAG=""
OUTPUT=""
ERROR_LOG_FILE=""
ALLOW_AI_FAIL=0
REPO=""
GH_TOKEN=""
COPILOT_TOKEN=""
MODEL=""
INITIAL_RELEASE_MARKER="__INITIAL_RELEASE__"

log() {
  echo "[generate_release_notes] $*" >&2
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo)
      REPO="$2"
      shift 2
      ;;
    --current-tag)
      CURRENT_TAG="$2"
      shift 2
      ;;
    --output)
      OUTPUT="$2"
      shift 2
      ;;
    --gh-token)
      GH_TOKEN="$2"
      shift 2
      ;;
    --copilot-token)
      COPILOT_TOKEN="$2"
      shift 2
      ;;
    --model)
      MODEL="$2"
      shift 2
      ;;
    --error-log-file)
      ERROR_LOG_FILE="$2"
      shift 2
      ;;
    --allow-ai-fail)
      ALLOW_AI_FAIL=1
      shift 1
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage
      exit 2
      ;;
  esac
done

if [[ -z "$REPO" ]]; then
  echo "--repo is required" >&2
  exit 2
fi

if [[ -z "$CURRENT_TAG" ]]; then
  echo "--current-tag is required" >&2
  exit 2
fi

if [[ -z "$OUTPUT" ]]; then
  echo "--output is required" >&2
  exit 2
fi

if [[ -z "$GH_TOKEN" ]]; then
  echo "--gh-token is required" >&2
  exit 2
fi

if [[ -z "$COPILOT_TOKEN" ]]; then
  echo "--copilot-token is required" >&2
  exit 2
fi

if [[ -z "$ERROR_LOG_FILE" ]]; then
  ERROR_LOG_FILE="${OUTPUT}.err"
fi

if [[ -z "$MODEL" ]]; then
  MODEL="gpt-5.1-codex-max"
fi

rm -f "$ERROR_LOG_FILE" || true
touch "$ERROR_LOG_FILE" || true

log "repo=${REPO} current_tag=${CURRENT_TAG} output=${OUTPUT} ERROR_LOG_FILE=${ERROR_LOG_FILE} MODEL=${MODEL}"

check_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "$1 is required on the runner" >&2
    exit 1
  fi
}

# Need jq for JSON parsing
# Need gh for GitHub API
check_command jq
check_command gh

# Check copilot and install if missing
if ! command -v copilot >/dev/null 2>&1; then
  echo "copilot is not installed, install now..." >&2
  npm install -g @github/copilot 2>&1 | tee -a "$ERROR_LOG_FILE" >&2
  if ! command -v copilot >/dev/null 2>&1; then
    echo "copilot installation failed" >&2
    exit 1
  fi
  echo "copilot installed" >&2
fi

copilot_generate() {
  local prompt="$1"

  # Use copilot without specifying model to use default available model
  # Capture both stdout and stderr to ERROR_LOG_FILE for debugging
  local result
  local rc=0
  result=$(GH_TOKEN="$COPILOT_TOKEN" copilot --allow-all-paths --allow-all-urls --allow-all-tools --silent --model "$MODEL" -p "$prompt" 2>&1) || rc=$?
  
  if (( rc != 0 )); then
    echo "[copilot_generate] rc=$rc" >>"$ERROR_LOG_FILE"
    echo "$result" >>"$ERROR_LOG_FILE"
    return $rc
  fi
  
  echo "$result"
}

# Fetch all tags from GitHub API (sorted by version)
fetch_all_tags() {
  local -a all_tags=()
  local page=1
  local per_page=100
  while true; do
    local tags_json
    if ! tags_json=$(GH_TOKEN="$GH_TOKEN" gh api "repos/${REPO}/tags?per_page=${per_page}&page=${page}" 2>>"$ERROR_LOG_FILE"); then
      echo "failed to fetch tags from GitHub API for repo ${REPO}" >&2
      return 1
    fi
    if [[ -z "$tags_json" ]]; then
      break
    fi
    local count
    count=$(printf '%s' "$tags_json" | jq -r 'length')
    if (( count == 0 )); then
      break
    fi
    while IFS= read -r tag_name; do
      [[ -z "$tag_name" ]] && continue
      all_tags+=("$tag_name")
    done < <(printf '%s' "$tags_json" | jq -r '.[].name // empty')
    if (( count < per_page )); then
      break
    fi
    page=$((page + 1))
  done
  printf '%s\n' "${all_tags[@]}"
}

# Sort tags by semantic version
sort_tags_by_version() {
  # Filter v* tags and sort by version
  grep '^v' | sort -t. -k1,1V -k2,2V -k3,3V
}

# Verify tag exists via GitHub API
verify_tag_exists() {
  local tag="$1"
  GH_TOKEN="$GH_TOKEN" gh api "repos/${REPO}/git/refs/tags/${tag}" >/dev/null 2>>"$ERROR_LOG_FILE"
}

# Ensure current tag exists
if ! verify_tag_exists "${CURRENT_TAG}"; then
  echo "current tag ${CURRENT_TAG} not found in repository" >&2
  exit 1
fi

compute_prev_tag() {
  local tag="$1"

  local -a tags
  mapfile -t tags < <(fetch_all_tags | sort_tags_by_version)

  log "found ${#tags[@]} tags via GitHub API"

  if [[ ${#tags[@]} -eq 0 ]]; then
    echo "no tags matched pattern v*" >&2
    return 1
  fi

  local -A tag_set=()
  local i
  for i in "${tags[@]}"; do
    tag_set["$i"]=1
  done

  local idx=-1
  for ((i=0; i<${#tags[@]}; i++)); do
    if [[ "${tags[$i]}" == "$tag" ]]; then
      idx=$i
      break
    fi
  done
  if (( idx < 0 )); then
    echo "current tag ${tag} not found in sorted tag list" >&2
    return 1
  fi

  is_prerelease() {
    local t="$1"
    [[ "$t" =~ -rc[0-9]+$ || "$t" =~ -r[0-9]+$ ]]
  }

  parse_version() {
    local t="$1"
    # print: major minor patch
    if [[ "$t" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)($|-.*$) ]]; then
      echo "${BASH_REMATCH[1]} ${BASH_REMATCH[2]} ${BASH_REMATCH[3]}"
      return 0
    fi
    return 1
  }

  # RC handling
  if [[ "$tag" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)-(rc|r)([0-9]+)$ ]]; then
    local maj="${BASH_REMATCH[1]}" min="${BASH_REMATCH[2]}" pat="${BASH_REMATCH[3]}" rcnum="${BASH_REMATCH[5]}"

    if (( rcnum > 0 )); then
      local prev_rc="v${maj}.${min}.${pat}-rc$((rcnum-1))"
      local prev_r="v${maj}.${min}.${pat}-r$((rcnum-1))"
      if [[ -n "${tag_set[$prev_rc]:-}" ]]; then
        echo "$prev_rc"
        return 0
      fi
      if [[ -n "${tag_set[$prev_r]:-}" ]]; then
        echo "$prev_r"
        return 0
      fi
      # if not found, fall through to stable-base / branch fallback
    fi

    # If base stable tag exists (vX.Y.Z), prefer using its previous stable tag.
    local base_stable="v${maj}.${min}.${pat}"
    if [[ -n "${tag_set[$base_stable]:-}" ]]; then
      local base_idx=-1
      for ((i=0; i<${#tags[@]}; i++)); do
        if [[ "${tags[$i]}" == "$base_stable" ]]; then
          base_idx=$i
          break
        fi
      done

      if (( base_idx >= 0 )); then
        for ((i=base_idx-1; i>=0; i--)); do
          local t="${tags[$i]}"
          if is_prerelease "$t"; then
            continue
          fi
          echo "$t"
          return 0
        done
      fi
    fi

    # Fallback: find latest stable tag with (same major and minor < current minor) OR (major < current major)
    for ((i=idx-1; i>=0; i--)); do
      local t="${tags[$i]}"
      if is_prerelease "$t"; then
        continue
      fi
      local vmj vmi _vpa
      read -r vmj vmi _vpa < <(parse_version "$t") || continue
      if (( vmj < maj )) || (( vmj == maj && vmi < min )); then
        echo "$t"
        return 0
      fi
    done

    echo "no previous stable tag found for ${tag}; treating as initial release" >&2
    echo "$INITIAL_RELEASE_MARKER"
    return 0
  fi

  # Stable handling: find previous stable tag in sorted order.
  if [[ "$tag" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
    for ((i=idx-1; i>=0; i--)); do
      local t="${tags[$i]}"
      if is_prerelease "$t"; then
        continue
      fi
      echo "$t"
      return 0
    done
    echo "$INITIAL_RELEASE_MARKER"
    return 0
  fi

  echo "invalid tag format: ${tag}" >&2
  return 1
}

PREV_TAG="$(compute_prev_tag "$CURRENT_TAG")"

if [[ -z "$PREV_TAG" ]]; then
  echo "previous tag is empty" >&2
  exit 1
fi

if [[ "$PREV_TAG" == "$CURRENT_TAG" ]]; then
  echo "previous tag equals current tag: ${PREV_TAG}" >&2
  exit 1
fi

if [[ "$PREV_TAG" != "$INITIAL_RELEASE_MARKER" ]] && ! verify_tag_exists "${PREV_TAG}"; then
  echo "previous tag ${PREV_TAG} not found in repository" >&2
  exit 1
fi

if [[ "$PREV_TAG" == "$INITIAL_RELEASE_MARKER" ]]; then
  log "previous_tag=<none> initial_release=1"
else
  log "previous_tag=${PREV_TAG}"
fi

# Collect commits between tags using GitHub compare API
if [[ "$PREV_TAG" == "$INITIAL_RELEASE_MARKER" ]]; then
  log "fetching commits reachable from ${CURRENT_TAG} via GitHub API..."
else
  log "fetching commits between ${PREV_TAG}..${CURRENT_TAG} via GitHub API..."
fi

fetch_compare_commits() {
  local base="$1"
  local head="$2"
  local -a all_commits=()
  local page=1
  local per_page=100
  while true; do
    local compare_json
    compare_json=$(GH_TOKEN="$GH_TOKEN" gh api "repos/${REPO}/compare/${base}...${head}?per_page=${per_page}&page=${page}" 2>/dev/null || true)
    if [[ -z "$compare_json" ]]; then
      break
    fi
    local commits_count
    commits_count=$(printf '%s' "$compare_json" | jq -r '.commits | length')
    if (( commits_count == 0 )); then
      break
    fi
    while IFS= read -r sha; do
      [[ -z "$sha" ]] && continue
      all_commits+=("$sha")
    done < <(printf '%s' "$compare_json" | jq -r '.commits[].sha // empty')
    # GitHub compare API returns all commits in one response (up to 250)
    # If we got less than expected or this is the first page, we're done
    break
  done
  printf '%s\n' "${all_commits[@]}"
}

fetch_initial_commits() {
  local head="$1"
  local -a all_commits=()
  local page=1
  local per_page=100
  while true; do
    local commits_json
    commits_json=$(GH_TOKEN="$GH_TOKEN" gh api "repos/${REPO}/commits?sha=${head}&per_page=${per_page}&page=${page}" 2>/dev/null || true)
    if [[ -z "$commits_json" ]]; then
      break
    fi
    local commits_count
    commits_count=$(printf '%s' "$commits_json" | jq -r 'length')
    if (( commits_count == 0 )); then
      break
    fi
    while IFS= read -r sha; do
      [[ -z "$sha" ]] && continue
      all_commits+=("$sha")
    done < <(printf '%s' "$commits_json" | jq -r '.[].sha // empty')
    if (( commits_count < per_page )); then
      break
    fi
    page=$((page + 1))
  done
  printf '%s\n' "${all_commits[@]}"
}

if [[ "$PREV_TAG" == "$INITIAL_RELEASE_MARKER" ]]; then
  mapfile -t SHAS < <(fetch_initial_commits "${CURRENT_TAG}")
  log "commit_range=<initial>..${CURRENT_TAG} commits=${#SHAS[@]}"
else
  mapfile -t SHAS < <(fetch_compare_commits "${PREV_TAG}" "${CURRENT_TAG}")
  log "commit_range=${PREV_TAG}..${CURRENT_TAG} commits=${#SHAS[@]}"
fi


# Resolve PR numbers from commit messages (fast path)
declare -A PR_SET=()
for sha in "${SHAS[@]}"; do
  [[ -z "$sha" ]] && continue
  commit_json=$(GH_TOKEN="$GH_TOKEN" gh api "repos/${REPO}/commits/${sha}" 2>/dev/null || true)
  if [[ -n "$commit_json" ]]; then
    subj=$(printf '%s' "$commit_json" | jq -r '.commit.message // ""' | head -1)
    if [[ "$subj" =~ Merge\ pull\ request\ \#([0-9]+) ]]; then
      PR_SET["${BASH_REMATCH[1]}"]=1
    elif [[ "$subj" =~ \(\#([0-9]+)\) ]]; then
      PR_SET["${BASH_REMATCH[1]}"]=1
    fi
  fi
done

log "prs_from_subjects=${#PR_SET[@]}"

# Resolve PR numbers from commits via GitHub API (robust path, supports squash)
# Using groot preview accept header for commits/{sha}/pulls.
commit_total=${#SHAS[@]}
commit_i=0
for sha in "${SHAS[@]}"; do
  [[ -z "$sha" ]] && continue
  commit_i=$((commit_i+1))
  if (( commit_i == 1 || commit_i % 50 == 0 || commit_i == commit_total )); then
    log "resolving prs from commits... processed=${commit_i}/${commit_total}" 
  fi
  # shellcheck disable=SC2016
  prs_json=$(GH_TOKEN="$GH_TOKEN" gh api -H 'Accept: application/vnd.github.groot-preview+json' "repos/${REPO}/commits/${sha}/pulls" 2>/dev/null || true)
  [[ -z "$prs_json" ]] && continue
  while IFS= read -r num; do
    [[ -z "$num" ]] && continue
    PR_SET["$num"]=1
  done < <(printf '%s' "$prs_json" | jq -r '.[].number // empty')
done

log "prs_total_after_commit_mapping=${#PR_SET[@]}"

# Prepare groups
FEATURES=""
FIXES=""
TESTS=""
DEPS=""
OTHER=""

get_pr_details() {
  local pr_num="$1"
  
  # Get PR basic info
  local pr_json
  pr_json=$(GH_TOKEN="$GH_TOKEN" gh api "repos/${REPO}/pulls/${pr_num}" 2>/dev/null || true)
  if [[ -z "$pr_json" ]]; then
    echo ""
    return 1
  fi
  
  # Extract PR details
  local title
  local body
  local author
  title=$(printf '%s' "$pr_json" | jq -r '.title // ""')
  body=$(printf '%s' "$pr_json" | jq -r '.body // ""')
  author=$(printf '%s' "$pr_json" | jq -r '.user.login // "unknown"')
  
  # Get commits
  local commits_json
  commits_json=$(GH_TOKEN="$GH_TOKEN" gh api "repos/${REPO}/pulls/${pr_num}/commits" 2>/dev/null || true)
  local commits=""
  if [[ -n "$commits_json" ]]; then
    commits=$(printf '%s' "$commits_json" | jq -r '.[] | "- \(.sha): \(.commit.message | split("\n")[0])"' | head -20)
  fi
  
  # Get changed files
  local files_json
  files_json=$(GH_TOKEN="$GH_TOKEN" gh api "repos/${REPO}/pulls/${pr_num}/files" 2>/dev/null || true)
  local files=""
  if [[ -n "$files_json" ]]; then
    files=$(printf '%s' "$files_json" | jq -r '.[] | "- \(.filename): \(.status) (+\(.additions)/-\(.deletions))"' | head -50)
  fi
  
  # Build complete PR information
  local pr_info
  pr_info="PR #${pr_num}: ${title}
Author: ${author}

Description:
${body}

Commits:
${commits}

Changed Files:
${files}"
  
  echo "$pr_info"
}

get_pr_code_changes() {
  local pr_num="$1"

  # Prefer PR-level unified diff. The per-file .patch field from pulls/{pr}/files
  # can be empty or truncated for large diffs.
  local pr_diff=""
  pr_diff=$(GH_TOKEN="$GH_TOKEN" gh api -H 'Accept: application/vnd.github.v3.diff' "repos/${REPO}/pulls/${pr_num}" 2>/dev/null || true)
  if [[ -n "$pr_diff" ]]; then
    # Filter vendor/ changes at diff-block level.
    # A diff block begins with: diff --git a/<path> b/<path>
    local filtered
    filtered=$(printf '%s\n' "$pr_diff" | awk '
      BEGIN { skip=0 }
      /^diff --git a\// {
        path=$0
        sub(/^diff --git a\//, "", path)
        sub(/ b\/.*/, "", path)
        skip = (path ~ /^vendor\//) || (path ~ /\/vendor\//)
      }
      { if (!skip) print }
    ')

    if [[ -n "$filtered" ]]; then
      local max_chars=60000
      local total_chars=${#filtered}
      if (( total_chars > max_chars )); then
        filtered="${filtered:0:max_chars}"
        filtered+=$'\n\n[TRUNCATED]\n'
      fi
      echo "$filtered"
      return 0
    fi
  fi

  local files_json
  files_json=$(GH_TOKEN="$GH_TOKEN" gh api "repos/${REPO}/pulls/${pr_num}/files" 2>/dev/null || true)
  if [[ -z "$files_json" ]]; then
    echo ""
    return 1
  fi

  local changes=""
  local filename patch
  while IFS= read -r filename; do
    [[ -z "$filename" ]] && continue

    if [[ "$filename" == vendor/* || "$filename" == */vendor/* ]]; then
      continue
    fi

    patch=$(printf '%s' "$files_json" | jq -r --arg f "$filename" '.[] | select(.filename == $f) | .patch // empty' | head -1)
    if [[ -z "$patch" ]]; then
      continue
    fi

    changes+=$'File: '
    changes+="$filename"
    changes+=$'\n'
    changes+=$'Patch:\n'
    changes+="$patch"
    changes+=$'\n\n'
  done < <(printf '%s' "$files_json" | jq -r '.[].filename // empty')

  if [[ -z "$changes" ]]; then
    echo ""
    return 1
  fi

  local max_chars=60000
  local total_chars=${#changes}
  if (( total_chars > max_chars )); then
    changes="${changes:0:max_chars}"
    changes+=$'\n\n[TRUNCATED]\n'
  fi

  echo "$changes"
}

generate_pr_summary_with_ai() {
  local pr_num="$1"
  local pr_code_changes="$2"
  
  if [[ -z "$pr_code_changes" ]]; then
    echo "PR #${pr_num}: no code changes found (or only vendor/binary), cannot generate summary" >&2
    return 2
  fi
  
  # Build prompt for qwen with code changes only
  local prompt
  prompt="You are generating release notes.

Task:
- Read ONLY the following code changes (diff/patch) and infer what changed.
- Ignore PR title/description/labels/authors and any non-code context.
- Ignore any changes under vendor/ (they have been filtered out already).
- Produce a ONE-sentence English summary suitable for release notes.

Constraints:
- Start with an action verb (e.g., Add, Introduce, Fix, Update, Remove, Improve, Implement, Support, Optimize, Refactor).
- Do not prefix with 'This pull request'.
- Do not mention the patch, just focus on the changes.
- Output ONLY the summary sentence.

Code changes:
${pr_code_changes}"
  
  local summary
  summary=$(copilot_generate "${prompt}" 2>>"$ERROR_LOG_FILE") || return $?
  
  if [[ -z "$summary" ]]; then
    return 1
  fi
  
  # Clean up summary
  summary="$(printf '%s' "$summary" | sed -e 's/^\s\+//; s/\s\+$//' | tr '\n' ' ' | sed -E 's/[[:space:]]+/ /g')"
  echo "$summary"
}

classify() {
  local labels="$1"
  # labels is newline-separated list
  if grep -qxE 'kind/feature|feature|release/feature-new' <<<"$labels"; then
    echo features
    return
  fi
  if grep -qxE 'kind/bug|kind/fix|fix|bug|release/fix|release/bug' <<<"$labels"; then
    echo fixes
    return
  fi
  if grep -qxE 'kind/test|test|release/test' <<<"$labels"; then
    echo tests
    return
  fi
  if grep -qxE 'kind/dependencies|dependencies|release/dependencies' <<<"$labels"; then
    echo dependencies
    return
  fi
  echo other
}

append_item() {
  local group="$1"
  local item="$2"
  case "$group" in
    features) FEATURES+="$item"$'\n' ;;
    fixes) FIXES+="$item"$'\n' ;;
    tests) TESTS+="$item"$'\n' ;;
    dependencies) DEPS+="$item"$'\n' ;;
    other) OTHER+="$item"$'\n' ;;
  esac
}

# Iterate PRs in numeric order for stable output
mapfile -t PR_NUMS < <(printf '%s\n' "${!PR_SET[@]}" | sort -n)

PR_COUNT=${#PR_NUMS[@]}

log "fetching PR details... prs=${PR_COUNT}"
pr_i=0

for n in "${PR_NUMS[@]}"; do
  [[ -z "$n" ]] && continue

  pr_i=$((pr_i+1))
  if (( pr_i == 1 || pr_i % 10 == 0 || pr_i == PR_COUNT )); then
    log "processing PR... ${pr_i}/${PR_COUNT}" 
  fi

  # Fetch PR basic info for URL and labels
  pr_json=$(GH_TOKEN="$GH_TOKEN" gh api "repos/${REPO}/pulls/${n}" 2>/dev/null || true)
  if [[ -z "$pr_json" ]]; then
    log "WARN failed to fetch PR #${n}"
    continue
  fi

  url=$(printf '%s' "$pr_json" | jq -r '.html_url // ""')
  author=$(printf '%s' "$pr_json" | jq -r '.user.login // "unknown"')
  labels=$(printf '%s' "$pr_json" | jq -r '.labels[].name // empty')
  title=$(printf '%s' "$pr_json" | jq -r '.title // ""')

  # Fetch PR code changes (patches), excluding vendor/
  log "fetching code changes for PR #${n}..."
  pr_code_changes=$(get_pr_code_changes "$n")
  if [[ -z "$pr_code_changes" ]]; then
    log "ERROR: PR #${n}: failed to fetch code changes (or only vendor/binary), skipping"
    exit 1
  fi

  log "generating AI summary for PR #${n}..."

  err_size_before=0
  if [[ -f "$ERROR_LOG_FILE" ]]; then
    err_size_before=$(wc -c <"$ERROR_LOG_FILE" 2>/dev/null || echo 0)
  fi
  note=""
  note=$(generate_pr_summary_with_ai "$n" "$pr_code_changes" 2>>"$ERROR_LOG_FILE") || true
  if [[ -z "$note" ]]; then
    err_size_after=0
    if [[ -f "$ERROR_LOG_FILE" ]]; then
      err_size_after=$(wc -c <"$ERROR_LOG_FILE" 2>/dev/null || echo 0)
    fi

    {
      echo "----- PR #${n} Copilot error log (tail) -----" >&2
      tail -n 200 "$ERROR_LOG_FILE" 2>/dev/null >&2 || true
      echo "----- end PR #${n} Copilot error log (tail) -----" >&2
    } || true

    if (( ALLOW_AI_FAIL == 1 )); then
      if [[ -n "$title" ]]; then
        note="Update: ${title}"
      else
        note="Update changes in PR #${n}"
      fi
      log "WARN: PR #${n}: AI summary failed, using fallback summary"
    else
      log "ERROR: PR #${n}: failed to generate AI summary"
      exit 1
    fi
  fi

  pr_link="[#${n}](${url})"
  item="- ${note} (${pr_link}, @${author})"
  grp=$(classify "$labels")
  append_item "$grp" "$item"
done

{
  echo "# Welcome to Unifabric ${CURRENT_TAG}"
  echo
  if [[ "$PREV_TAG" == "$INITIAL_RELEASE_MARKER" ]]; then
    echo "This initial release includes ${PR_COUNT} pull requests up to ${CURRENT_TAG}:"
  else
    echo "This release includes ${PR_COUNT} pull requests from ${PREV_TAG} to ${CURRENT_TAG}:"
  fi
  echo
  if [[ -n "$FEATURES" ]]; then
    echo "##  🚀  Features"
    echo
    printf '%s' "$FEATURES"
    echo
  fi

  if [[ -n "$FIXES" ]]; then
    echo "##  🐛  Fixes"
    echo
    printf '%s' "$FIXES"
    echo
  fi

  if [[ -n "$TESTS" ]]; then
    echo "##  🧪  Tests"
    echo
    printf '%s' "$TESTS"
    echo
  fi

  if [[ -n "$DEPS" ]]; then
    echo "##  📦  Dependencies"
    echo
    printf '%s' "$DEPS"
    echo
  fi

  if [[ -n "$OTHER" ]]; then
    echo "##  💬  Other"
    echo
    printf '%s' "$OTHER"
    echo
  fi

  if [[ -z "$FEATURES" && -z "$FIXES" && -z "$TESTS" && -z "$DEPS" && -z "$OTHER" ]]; then
    echo "##  💬  Other"
    echo
    echo "- No changes"
  fi
} >"$OUTPUT"
