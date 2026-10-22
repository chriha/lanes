# Review criteria (shared)

Single source for the rules-compliance and test-quality review. Used by `/local-code-review` (agents 1, 2, 4,
validation in step 4) and by the implementer's Review step (`work-tickets/implementer.md`).

## Rules compliance (CLAUDE.md and `.claude/rules/*`)

Audit the changes for CLAUDE.md and rules compliance (when the repo has none, there is nothing to audit).
For a file, only consider CLAUDE.md files that share a file path with the file or its parents.

## Test quality

Review only test files (`*_test.go`) in the diff. For each test, flag violations of:
- **Goal-first:** tests must specify desired behavior or intent (requirements, edge cases, error handling) — not just exercise code paths.
- **Small & focused:** each test should check one behavior; flag tests that assert multiple unrelated things or have excessive setup.
- **Fast & deterministic:** flag tests that rely on I/O, randomness, timing (`time.Sleep`), the real clock, tmux, the network or other external dependencies without a fake.
- **Readable & maintainable:** flag unclear names, missing arrange/act/assert structure, or duplicated setup that should use helpers (`t.Helper()`) or table-driven cases.
- **Independent:** flag tests that depend on execution order or shared mutable state (package-level vars, env vars without `t.Setenv`, files outside `t.TempDir()`).
- **Right level of abstraction:** flag unit tests that should be integration tests or vice versa where the mismatch is obvious.
- **Coverage of cases:** flag obvious missing paths — especially missing error paths, boundary values, or invalid input cases — when the diff introduces logic that clearly warrants them.
- **Assert behavior, not implementation:** flag tests that assert on internal calls or unexported details when the observable outcome could be checked instead.

Skip this check if no test files appear in the diff.

## Signal threshold

**We only want HIGH and MEDIUM SIGNAL issues.** Flag issues where:
- The code will fail to compile or parse (syntax errors, type errors, missing imports, unresolved references)
- The code will definitely produce wrong results regardless of inputs (clear logic errors)
- Clear, unambiguous CLAUDE.md and rules (`.claude/rules/*`) violations where you can quote the exact rule being broken

Do NOT flag:
- Code style or quality concerns
- Potential issues that depend on specific inputs or state
- Subjective suggestions or improvements

If you are not certain an issue is real, do not flag it. False positives erode trust and waste reviewer time.

## Validation

Each issue is validated by a separate sonnet subagent that gets the author intent and the issue description and
must confirm with high confidence that the issue is real. For CLAUDE.md issues, confirm the rule is scoped for
this file and actually violated. Drop everything not validated.

## False positives (do NOT flag)

- Pre-existing issues
- Something that appears to be a bug but is actually correct
- Pedantic nitpicks that a senior engineer would not flag
- Issues that a linter will catch (`gofmt`, `go vet`; do not run the linter to verify)
- General code quality concerns (e.g., lack of test coverage, general security issues) unless explicitly required in CLAUDE.md
- Issues mentioned in CLAUDE.md but explicitly silenced in the code (e.g., via a `//nolint` comment)
