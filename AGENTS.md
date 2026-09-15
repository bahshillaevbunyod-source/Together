# Together Engineering Rules

## Scope and source of truth

- The source of truth is `C:\Together` only.
- Never use or inspect `D:\Together-Agent-Test`.
- Work one small, scoped step at a time.
- Inspect before editing.
- Do not change unrelated files.

## Engineering rules

- The backend stack is Go + PostgreSQL.
- Use parameterized SQL only.
- Preserve existing authentication, block, privacy, visibility, and ownership rules.
- Use deterministic keyset pagination where appropriate.
- Do not use destructive database operations.
- Never print or expose `.env` values, database credentials, API keys, session cookies, tokens, presigned URLs, or other secrets.
- Do not modify the frontend unless the task explicitly asks for frontend work.
- Do not introduce mock or fake production data.
- Do not redesign the architecture unless explicitly requested.
- Prefer the smallest correct fix over broad refactors.

## Verification

- After backend changes, run:

  ```text
  go test ./...
  go vet ./...
  ```

- After backend API changes:
  - Restart the backend using the existing development launcher.
  - Verify `/health` returns HTTP 200.
  - Verify `/ready` returns HTTP 200.
- Be aware of stale backend binaries and processes.

## Git rules

- Inspect `git status` before staging.
- Do not push unless explicitly requested.
- Do not force-push.
- Do not rebase.
- Do not amend commits unless explicitly requested.
- Keep feature and fix commits separated.
- Stage only files belonging to the current step.

## Quality order

1. Correctness
2. Reliability
3. Product quality

## Completed-step report

For every completed step, report:

- Exact files changed
- What changed
- Tests: PASS/FAIL
- Live/runtime verification
- Remaining bugs
- Commit hash if committed
- Working tree clean: YES/NO
