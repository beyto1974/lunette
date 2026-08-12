#!/usr/bin/env bash
#
# Everything worth checking before pushing to a public repository.
#
#   make audit
#
# The mechanical half of the checklist in CLAUDE.md: what a machine can decide.
# The half it cannot - whether a fixture carries somebody's real data, whether
# a screenshot shows a real catalogue - is listed there and stays a judgement.
#
# Exits non-zero if any check fails, so it can gate a push.
#
set -uo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root" || exit 1

failed=0

pass() { printf '  \033[32mok\033[0m    %s\n' "$1"; }
fail() {
  printf '  \033[31mFAIL\033[0m  %s\n' "$1"
  [ -n "${2:-}" ] && printf '%s\n' "$2" | sed 's/^/          /'
  failed=1
}

section() { printf '\n%s\n' "$1"; }

section "History"

# The claude.ai session link is meaningless to anyone else and permanent once
# pushed. .claude/settings.json stops it being written; this catches a commit
# made by a session that predates that file.
if trailers=$(git log --format=%B | grep -cE '^Claude-Session:'); [ "$trailers" -eq 0 ]; then
  pass "no session trailers in commit messages"
else
  fail "$trailers commit(s) carry a Claude-Session trailer" \
    "strip them before pushing: see the recipe in CLAUDE.md"
fi

authors=$(git log --format='%an <%ae>' | sort -u)
if printf '%s' "$authors" | grep -qv 'noreply'; then
  fail "an author or committer address is not a noreply address" "$authors"
else
  pass "every commit is authored by a noreply address"
fi

section "Working tree"

if [ -z "$(git status --porcelain)" ]; then
  pass "tree is clean"
else
  fail "uncommitted changes" "$(git status --short)"
fi

# Secret-shaped strings. Prose about tokens and the workflow's own
# secrets.GITHUB_TOKEN reference are not findings, and this script is excluded
# because it contains the patterns it searches for.
secrets=$(git grep -nEi \
  '(api[_-]?key|secret[_-]?key|passwd|password|bearer [A-Za-z0-9._-]{16,}|BEGIN [A-Z ]*PRIVATE KEY|AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9]{20,}|xox[baprs]-)' \
  -- . ':!scripts/audit.sh' 2>/dev/null | grep -vE 'secrets\.GITHUB_TOKEN|\$\{\{' || true)
if [ -z "$secrets" ]; then
  pass "no secret-shaped strings in tracked files"
else
  fail "possible secrets" "$secrets"
fi

emails=$(git grep -Eoh '[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}' -- . 2>/dev/null |
  grep -v 'noreply' | sort -u || true)
if [ -z "$emails" ]; then
  pass "no email addresses in tracked files"
else
  fail "email addresses in tracked files" "$emails"
fi

paths=$(git grep -nE '(/home/[a-z]+|/Users/[a-z]+|/opt/claude|/tmp/claude)' \
  -- . ':!scripts/audit.sh' 2>/dev/null || true)
if [ -z "$paths" ]; then
  pass "no local filesystem paths"
else
  fail "local paths would be published" "$paths"
fi

# A file nobody meant to commit is usually a big one.
big=$(git ls-files -z | xargs -0 du -k 2>/dev/null | awk '$1 > 512 {print $1 "KB  " $2}' || true)
if [ -z "$big" ]; then
  pass "no tracked file over 512KB"
else
  fail "large files - check they belong in the repository" "$big"
fi

section "Code"

if [ -z "$(gofmt -l .)" ]; then
  pass "gofmt"
else
  fail "unformatted files" "$(gofmt -l .)"
fi

if go vet ./... 2>/dev/null; then pass "go vet"; else fail "go vet"; fi
if go test ./... >/dev/null 2>&1; then pass "tests"; else fail "tests - run make test"; fi

if command -v shellcheck >/dev/null 2>&1; then
  if shellcheck install.sh scripts/*.sh >/dev/null 2>&1; then
    pass "shellcheck"
  else
    fail "shellcheck" "$(shellcheck install.sh scripts/*.sh 2>&1 | head -20)"
  fi
else
  printf '  skip  shellcheck is not installed\n'
fi

section "Dependencies and release"

if command -v govulncheck >/dev/null 2>&1; then
  if govulncheck ./... >/dev/null 2>&1; then
    pass "govulncheck: nothing reachable"
  else
    fail "govulncheck found something this code can reach" \
      "$(govulncheck ./... 2>&1 | grep -A3 Vulnerability | head -20)"
  fi
else
  printf '  skip  govulncheck is not installed (go install golang.org/x/vuln/cmd/govulncheck@latest)\n'
fi

if command -v goreleaser >/dev/null 2>&1; then
  if goreleaser check >/dev/null 2>&1; then
    pass "release configuration"
  else
    fail "goreleaser check" "$(goreleaser check 2>&1 | tail -3)"
  fi
else
  printf '  skip  goreleaser is not installed\n'
fi

section "Documentation"

missing=""
while read -r link; do
  [ -e "$link" ] || missing="$missing$link"$'\n'
done < <(grep -oE '\]\(([^)h][^)]*)\)' README.md | tr -d '](' | tr -d ')')
if [ -z "$missing" ]; then
  pass "every relative link in the README resolves"
else
  fail "broken README links" "$missing"
fi

printf '\n'
if [ "$failed" -eq 0 ]; then
  printf 'audit passed\n'
else
  printf 'audit failed - see above\n'
fi
exit "$failed"
