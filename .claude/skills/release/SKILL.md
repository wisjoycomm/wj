---
name: release
description: Release a new wj version - checks, version bump, PR merged into main, tag, GitHub Actions build and publish. Use only when the user asks to release, or invokes /release.
argument-hint: <version, e.g. 0.2.12>
disable-model-invocation: true
---

# Release wj

1. Pick the version: the next patch after `VERSION` unless the user names one.
2. Make sure `CHANGELOG.md` has a `## <version>` entry at the top, a few lines on what changed for users. Write
   it from `git log --oneline v<last>..HEAD` if missing, and commit it on the current branch.
3. Run `sh .tools/release.sh <version>`. Add `-p` to refresh `prices.json` first. `-h` lists every step.

Output: one `== <step>` line per step, then the release URL and asset count (the last release had 23). Exit 0 means
the release is published. A `FAIL: ...` line names the step and the log in `.tmp/release/` to search; fix the cause
and run it again. Steps already done (commit, PR) are skipped or fail loudly, never repeated silently: if it failed
after the tag was pushed, don't re-run, check `gh run list` instead.

Details and how to undo: `.docs/release.md`.
