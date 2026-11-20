#!/bin/sh
# Releases wj: checks, version bump, PR merged into main, tag v<version>, waits for the Release workflow.
usage() {
  cat <<'EOF'
Usage: .tools/release-new-wj-version/release-new-version.sh [-h] [-p] VERSION      e.g. .tools/release-new-wj-version/release-new-version.sh 0.2.12

Releases wj VERSION from the current branch (on main it first makes branch release-VERSION):
  1. needs a clean tree, gh signed in, a "## VERSION" entry in CHANGELOG.md, tag vVERSION not taken
  2. -p: refreshes prices.json (.tools/release-new-wj-version/refresh-model-prices-from-openrouter.sh) and commits it if it changed
  3. gofmt -l, go vet, go test (log in .tmp/release/)
  4. writes VERSION and .claude/wj-kit, commits "chore: release VERSION"
  5. pushes the branch, opens a PR into main with the CHANGELOG entry, merges it
  6. updates local main, tags vVERSION, pushes the tag: GitHub Actions "Release" builds and publishes
  7. waits for that run and prints the release URL and asset count
Prints one line per step; on failure the last line says what failed. Exit 0 = released.
EOF
}
set -eu
prices=0
while getopts hp opt; do
  case $opt in
    h) usage; exit 0 ;;
    p) prices=1 ;;
    *) usage >&2; exit 2 ;;
  esac
done
shift $((OPTIND - 1))
[ $# -eq 1 ] || { usage >&2; exit 2; }
ver=$1
cd "$(dirname "$0")/../.."
log=.tmp/release
mkdir -p "$log"
fail() { echo "FAIL: $*" >&2; exit 1; }
step() { echo "== $*"; }

step "check"
echo "$ver" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' || fail "version must look like 0.2.12, got '$ver'"
gh auth status >/dev/null 2>&1 || fail "gh is not signed in (run: gh auth login)"
[ -z "$(git status --porcelain)" ] || fail "working tree not clean: commit or stash first"
grep -q "^## $ver\$" CHANGELOG.md || fail "CHANGELOG.md has no '## $ver' entry"
git rev-parse -q --verify "refs/tags/v$ver" >/dev/null && fail "tag v$ver exists locally"
[ -z "$(git ls-remote --tags origin "v$ver")" ] || fail "tag v$ver exists on origin"
branch=$(git branch --show-current)
[ -n "$branch" ] || fail "detached HEAD: switch to a branch"
if [ "$branch" = main ]; then
  branch=release-$ver
  git switch -q -c "$branch"
  echo "on main: made branch $branch"
fi

if [ $prices -eq 1 ]; then
  step "prices"
  sh .tools/release-new-wj-version/refresh-model-prices-from-openrouter.sh
  if [ -n "$(git status --porcelain prices.json)" ]; then
    git commit -q -m "chore: refresh model prices" prices.json
  fi
fi

step "gofmt, vet, test"
bad=$(gofmt -l .)
[ -z "$bad" ] || fail "gofmt: $bad"
go vet ./... >"$log/vet.log" 2>&1 || fail "go vet, see $log/vet.log"
go test ./... >"$log/test.log" 2>&1 || fail "go test, see $log/test.log"

step "bump to $ver"
printf '%s\n' "$ver" > VERSION
printf '%s\n' "$ver" > .claude/wj-kit
git add VERSION .claude/wj-kit
git diff --cached --quiet || git commit -q -m "chore: release $ver"

step "PR into main"
git push -q -u origin "$branch" 2>"$log/push.log" || fail "git push, see $log/push.log"
awk -v h="## $ver" '$0 == h {on = 1; next} on && /^## / {exit} on' CHANGELOG.md > "$log/body.md"
if [ "$(gh pr view "$branch" --json state -q .state 2>/dev/null || true)" != OPEN ]; then
  gh pr create --base main --head "$branch" --title "Release $ver" --body-file "$log/body.md" >"$log/pr.log" 2>&1 \
    || fail "gh pr create, see $log/pr.log"
fi
gh pr merge "$branch" --merge --subject "Merge pull request: release $ver" >>"$log/pr.log" 2>&1 \
  || fail "gh pr merge, see $log/pr.log"
tail -n 1 "$log/pr.log"

step "tag v$ver"
git switch -q main
git pull -q --ff-only
[ "$(tr -d '\r\n' < VERSION)" = "$ver" ] || fail "main VERSION is not $ver after merge"
git tag -a "v$ver" -m "v$ver"
git push -q origin "v$ver"

step "wait for Release workflow"
run=""
for _ in 1 2 3 4 5 6 7 8 9 10 11 12; do
  run=$(gh run list --workflow release.yml --branch "v$ver" --limit 1 --json databaseId -q '.[0].databaseId' 2>/dev/null || true)
  [ -n "$run" ] && break
  sleep 5
done
[ -n "$run" ] || fail "no Release run found for v$ver (check: gh run list)"
gh run watch "$run" --exit-status >"$log/run.log" 2>&1 || fail "Release run $run failed, see $log/run.log or: gh run view $run --log-failed"
gh release view "v$ver" --json url,assets -q '"\(.url)  \(.assets | length) assets"'
rm -rf "$log"
