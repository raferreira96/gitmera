#!/usr/bin/env bash
set -euo pipefail

readonly COMMIT_REGEX='^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)\([a-zA-Z0-9_/.,-]+\)!?: .+$'

invalid_count=0

while IFS= read -r subject || [[ -n "$subject" ]]; do
  [[ -z "$subject" ]] && continue
  if [[ ! "$subject" =~ $COMMIT_REGEX ]]; then
    echo "Invalid commit message: \"$subject\"" >&2
    invalid_count=$((invalid_count + 1))
  fi
done

if [[ "$invalid_count" -gt 0 ]]; then
  echo "" >&2
  echo "$invalid_count commit message(s) do not follow Conventional Commits with a required scope." >&2
  echo "Expected format: type(scope): description   (e.g. fix(git): handle nil pointer)" >&2
  echo "Allowed types: feat fix docs style refactor perf test build ci chore revert" >&2
  exit 1
fi

exit 0
