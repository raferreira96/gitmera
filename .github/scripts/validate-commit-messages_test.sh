#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VALIDATOR="$SCRIPT_DIR/validate-commit-messages.sh"

pass=0
fail=0

assert_valid() {
  local subject="$1"
  if printf '%s\n' "$subject" | "$VALIDATOR" >/dev/null 2>&1; then
    pass=$((pass + 1))
  else
    echo "FAIL (expected valid): $subject"
    fail=$((fail + 1))
  fi
}

assert_invalid() {
  local subject="$1"
  if printf '%s\n' "$subject" | "$VALIDATOR" >/dev/null 2>&1; then
    echo "FAIL (expected invalid): $subject"
    fail=$((fail + 1))
  else
    pass=$((pass + 1))
  fi
}

# Valid: standard conventional-commit-with-scope messages
assert_valid "fix(git): synchronize execCommand against concurrent test overrides"
assert_valid "feat(cmd): surface underlying error for repositories reported as Missing"
assert_valid "chore(deps)!: bump go to 1.26"
assert_valid "docs(cmd,runner): fix stale comment and document RepoTask.Action"

# Invalid: missing scope, wrong format, or no space after colon
assert_invalid "fix: missing scope"
assert_invalid "update stuff"
assert_invalid "Fixed bug in git module"
assert_invalid "feat(cmd):no space after colon"

echo ""
echo "$pass passed, $fail failed"
[[ "$fail" -eq 0 ]]
