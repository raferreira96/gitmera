#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="$ROOT/bin/gitmera"
WORKDIR="$(mktemp -d)"

pass() {
  echo "PASS: $1"
}

fail() {
  echo "FAIL: $1"
  exit 1
}

assert_contains() {
  local file="$1"
  local expected="$2"

  if ! grep -q "$expected" "$file"; then
    echo "Expected to find: $expected"
    echo "File content:"
    cat "$file"
    fail "missing expected output"
  fi
}

echo "Smoke workspace: $WORKDIR"

cd "$ROOT"

echo "Building gitmera..."
go build -o "$BIN" .
pass "go build"

echo "Running targeted init tests..."
go test ./cmd/... -run 'TestPromptProject|TestInitCmd' -v
pass "targeted init tests"

echo "Testing init with multiple repositories..."
mkdir -p "$WORKDIR/multiple-repos"
cd "$WORKDIR/multiple-repos"

printf 'api\nhttps://github.com/example/api.git\n./api\ny\nweb\nhttps://github.com/example/web.git\n./web\nn\n' | "$BIN" init > output.txt 2>&1

assert_contains output.txt "Successfully initialized configuration"
assert_contains .gitmera.yaml "api:"
assert_contains .gitmera.yaml "web:"
assert_contains .gitmera.yaml "https://github.com/example/api.git"
assert_contains .gitmera.yaml "https://github.com/example/web.git"

"$BIN" > validate.txt 2>&1
assert_contains validate.txt "Found and validated configuration file"
pass "interactive init creates multiple repositories"

echo "Testing duplicate project name retry..."
mkdir -p "$WORKDIR/duplicate-name"
cd "$WORKDIR/duplicate-name"

printf 'api\nhttps://github.com/example/api.git\n./api\ny\napi\nweb\nhttps://github.com/example/web.git\n./web\nn\n' | "$BIN" init > output.txt 2>&1

assert_contains output.txt "Project name \"api\" is already used"
assert_contains .gitmera.yaml "api:"
assert_contains .gitmera.yaml "web:"

"$BIN" > validate.txt 2>&1
assert_contains validate.txt "Found and validated configuration file"
pass "duplicate project name is rejected"

echo "Testing empty repository URL retry..."
mkdir -p "$WORKDIR/empty-repo-url"
cd "$WORKDIR/empty-repo-url"

printf 'api\n\nhttps://github.com/example/api.git\n./api\nn\n' | "$BIN" init > output.txt 2>&1

assert_contains output.txt "Git Repository URL cannot be empty"
assert_contains output.txt "Project added to configuration"
assert_contains .gitmera.yaml "https://github.com/example/api.git"

"$BIN" > validate.txt 2>&1
assert_contains validate.txt "Found and validated configuration file"
pass "empty repository URL retries before project is added"

echo "Testing non-interactive init..."
mkdir -p "$WORKDIR/non-interactive"
cd "$WORKDIR/non-interactive"

"$BIN" init --non-interactive > output.txt 2>&1

assert_contains output.txt "Successfully initialized configuration"
assert_contains .gitmera.yaml "example:"
assert_contains .gitmera.yaml "git@github.com:example/repo.git"
assert_contains .gitmera.yaml "path: ./example"
pass "non-interactive init remains unchanged"

echo "Checking known follow-up: invalid repository URL format..."
mkdir -p "$WORKDIR/invalid-repo-url"
cd "$WORKDIR/invalid-repo-url"

set +e
printf 'api\neee\n./api\nn\n' | "$BIN" init > output.txt 2>&1
invalid_exit_code=$?
set -e

if grep -q "Project added to configuration" output.txt && grep -q "invalid Git URI format" output.txt; then
  echo "WARN: invalid repository URL still shows project added before final validation."
  echo "WARN: treating as known follow-up, not blocking this smoke."
elif [ "$invalid_exit_code" -eq 0 ]; then
  fail "invalid repository URL unexpectedly succeeded"
else
  pass "invalid repository URL is rejected"
fi

echo "All blocking smoke checks passed."
