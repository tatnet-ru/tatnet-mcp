# Supabase MCP validation

Implementation prepared in the `codex/supabase-mcp` branches of the local API,
MCP, CLI, Go SDK, frontend and Supabase Control checkouts. Nothing was deployed
or published. Customer databases and the source checkouts were not modified.

## Verified

- API: 195 tests passed across Supabase, IAM policy, API keys, OAuth consent,
  OAuth provider and the public OpenAPI snapshot. Real PostgreSQL 18 in a
  disposable Docker container; schema built by the repository's Goose migrations.
- Supabase Control: `go test ./...` and `go build ./...` passed. The SQL fence
  test ran against real PostgreSQL: ordinary reads, NULL vs empty text,
  row/JSON limits, CTE/function/DDL writes, transaction escapes, multi-statement
  SQL, system role/file-program access and backend cancellation after disabling
  `statement_timeout`. Private read and write routes reject unauthenticated calls.
- MCP: `go test ./...` and `go build ./...` passed. Tests use real HTTP MCP clients
  and a test OAuth issuer. They cover separate audiences (including cache hits),
  protected-resource metadata, exact database tools, grant headers without token
  forwarding, denied API calls, destructive/non-idempotent annotations, structured
  SQL errors and no retries after unknown write outcomes.
- CLI: `go test ./...` and `go build ./...` passed with its published SDK dependency.
  New commands require explicit confirmation for write SQL and do not retry.
  Its extended embedded contract preserves the SDK-backed operations unchanged.
- Go SDK: regenerated only Supabase additions; `go test ./...` and
  `go build ./...` passed. Unrelated existing SDK operations were preserved.
- Frontend: 16 auth consent tests and 27 shared policy tests passed. Auth and
  user TypeScript checks passed. Auth production build passed with Webpack.
- Playwright against the production auth build: explicit database selection,
  read default, write consequences, exact submitted database ids, account reset,
  retry after API error, keyboard input, loading, no-database/no-account/error
  states, English copy, dark theme/reduced motion and 390px no-overflow passed.
  No page errors. Browser plugin was not available; existing local Playwright
  was used. API replies were test fixtures, not real user sessions.

## Remaining release verification

Deploy in the order in the README, then perform a real ChatGPT OAuth connection
against a disposable managed database and verify revocation. No real ChatGPT,
production OAuth issuer, TLS ingress or managed Supabase host was exercised here.

The auth build used `bun run build --webpack`: Turbopack rejected external
node_modules symlinks in the isolated checkout. Existing installed dependencies
were reused without modifying the source checkouts. Static imports for the two
supported message locales also avoid Webpack's dynamic package-export lookup.

The premium static UI audit was run. The auth scope reported five pre-existing
findings in password-reset/error pages (button/form ownership); none was in the
changed consent component. The monorepo audit also reported existing ownership
findings in unrelated admin/user screens. Those screens were not changed.
Auth ESLint has zero errors and one existing internal-navigation warning.

Screenshots from the local flow were saved to `/tmp/tatnet-supabase-consent-*.png`.
Tests use their own accounts, grants and data. Temporary servers and the database
container are stopped after validation.

## Pre-release refactoring review

The review found three maintenance risks and fixed them before release:

- SQL read/write modes duplicated connection setup, NULL handling, truncation
  and response-budget accounting. `connection.go` now owns the constrained
  identity and cancellation; `results.go` owns the common collector. Read-only
  transaction setup remains separate from unrestricted script execution. The
  integration suite additionally verifies escaped credentials, identical UTF-8
  truncation/column metadata and a shared byte budget across script statements.
- MCP resource registration duplicated protected-resource metadata, host checks,
  tool registration and OAuth audiences. A single resource registry now drives
  all of them, including DCR. The token verifier looks up host + path uniformly;
  it no longer has a Supabase-specific authentication branch. Table-based tests
  cover both resources and every configured public hostname.
- CLI's standard sync command previously copied the pinned SDK contract over
  the new Supabase operations. The reproducible contract now composes that SDK
  with an explicit `supabase-extension.json`; conflicting paths or schemas fail
  the sync. The compatibility test compares the entire resulting document.
  This avoids weakening schema checks while awaiting a published SDK release.

The OAuth consent API also stopped querying app projects when displaying
Supabase databases. Its public response shape and permission model are unchanged.
The API suite (195 tests), all MCP/CLI/control Go suites, builds and the real
PostgreSQL fence tests were rerun. Frontend visuals and protocol contracts were
not redesigned. Production remains undeployed; real ChatGPT OAuth/TLS ingress
verification is still part of the documented rollout.
