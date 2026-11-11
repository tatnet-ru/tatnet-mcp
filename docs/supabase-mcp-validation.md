# Supabase MCP validation

Status: deployed and verified on 2026-11-07. The protected endpoint is
`https://mcp.tatnet.cloud/supabase/mcp`. This report contains no credentials,
customer query results, or database identifiers.

## Production verification

- ChatGPT discovered the protected resource, the TatNet OAuth issuer, OIDC, and
  dynamic client registration without manually entered OAuth credentials.
- A real ChatGPT OAuth connection selected one explicitly authorized database
  with owner-SQL access. `list_databases` returned exactly that one database.
- `query_database` returned `ok = 1` and `transaction_read_only = on` for a
  synthetic query that did not read application tables.
- `execute_sql` ran `BEGIN; SET LOCAL application_name = ...; SELECT ...;
  ROLLBACK;` and returned the expected session marker. No table, schema, or row
  mutations were used for the production test.
- The test grant was revoked in TatNet. The next ChatGPT tool request was denied
  and ChatGPT requested reauthentication.
- The independent API-key smoke also verified anonymous and invalid-key rejection,
  resource/issuer discovery, the exact three-tool surface, no superuser/create-role/
  create-database authority, read-mode escape and multi-statement rejection,
  query cancellation, row limits, and NULL versus empty-string fidelity.
- The production panel renders the endpoint and OAuth guide. Database-specific
  grant summaries show the selected database and permission level without
  rewriting their IAM policy. The scoped-name loader drops stale account results.
- The new Studio image was opened through normal Basic authentication on the
  selected database. **Connect → MCP** displayed the managed OAuth guide instead
  of upstream self-hosted tunnel setup. Escape closed the connection sheet.
- Existing Studio apps upgrade on opening from TatNet; PostgreSQL and customer
  schemas did not require changes for MCP.

## Merges and artifacts

| Component | Merged change / release |
| --- | --- |
| Public API | [Supabase API #1559](https://github.com/manzhikov/tatnet-api/pull/1559), [production promotion #1560](https://github.com/manzhikov/tatnet-api/pull/1560) |
| MCP server | [MCP #47](https://github.com/tatnet-ru/tatnet-mcp/pull/47) |
| Supabase Control | [Control #68](https://github.com/manzhikov/tatnet-supabase-control/pull/68); primary and both certificate-sync replicas updated and healthy |
| Console and consent | [Consent #1145](https://github.com/manzhikov/tatnet-frontend/pull/1145), [production #1146](https://github.com/manzhikov/tatnet-frontend/pull/1146), [connection UI #1147](https://github.com/manzhikov/tatnet-frontend/pull/1147), [production #1148](https://github.com/manzhikov/tatnet-frontend/pull/1148) |
| Supabase Studio | [Studio #20](https://github.com/tatnet-ru/supabase/pull/20) |
| CLI | [v0.18.0](https://github.com/tatnet-ru/tatnet-cli/releases/tag/v0.18.0), including npm `tatnet@0.18.0` |
| Go SDK | [v0.19.0](https://github.com/tatnet-ru/tatnet-go/releases/tag/v0.19.0) |

Deployed images:

- MCP: `ghcr.io/tatnet-ru/tatnet-mcp@sha256:87692af0d02a5aaec7b2b58c620de9c5b837a824b0225b10a252a50619413a27`.
- Studio: `ghcr.io/tatnet-ru/tatnet-studio@sha256:77e45587546696bbbe2ef877214d61d6d58a6148ddb502cb4d0ca503943b8591`.

Studio image source is commit `69c9092`; subsequent changes in the same PR
corrected only CI runner/analyzer configuration. The image workflow completed
successfully. The selected Studio build reached `status=success` and
`deploy_state=live` with the expected digest.

## Automated verification

- API: 195 focused tests for Supabase, IAM, API keys, OAuth consent/provider, and
  the public OpenAPI snapshot, using real PostgreSQL and repository migrations.
- Control: Go tests and builds, plus the PostgreSQL 15 CI race/integration suite.
  The SQL fence covers function/CTE/DDL writes, transaction escapes, scripts,
  system-role/file-program access, cancellation, result limits, escaped
  credentials, and shared script-result budgets. Private SQL routes reject
  unauthenticated requests.
- MCP: Go tests, vet and build; real HTTP MCP clients exercise resource audiences,
  cache hits, discovery/DCR, grant headers without token forwarding, annotations,
  structured errors, and no retry after an unknown write outcome.
- CLI and SDK: Go suites/builds, regenerated SDK contract, and a complete composed
  CLI contract snapshot check against the pinned SDK plus Supabase extension.
- Console: consent interaction coverage, 58 focused connection/policy tests, and
  clean-install CI lint, test, and production builds. Studio CI passed integration
  tests, lint, typecheck, knip, and CodeQL before merge.
- Consent browser fixtures additionally covered explicit database selection,
  read default, write consequences, account reset, retry/error/loading/empty
  states, English copy, keyboard input, dark theme/reduced motion, and 390px layout.
  These fixture checks are distinct from the real production OAuth checks above.

Temporary validation servers and the two task-owned PostgreSQL containers were
stopped. Original source checkouts were not modified.

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
PostgreSQL fence tests were rerun before release. Production verification is
recorded above.
