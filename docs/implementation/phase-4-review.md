# Phase 4 follow-up review — 2026-10-30

Baseline: `2d6f0e9...d252e64`. The review used separate Standards and Spec agents, followed by browser reproductions and targeted fixes. Findings below are fixed in local commits; no remote review state was changed.

## Standards

1. **Fixed — asynchronous import discarded active edits.** The import callback captured an old clean-editor state. A delayed-response browser check changed the current graph from two to three nodes; before the fix the response replaced it with the imported two-node graph. Import now only refreshes the workflow list. The same check preserves all three unsaved nodes. Commit `f80bfe4`.
2. **Fixed — the editor could not reopen some of its own exports.** A valid 99-action export was 264837 bytes when pretty-printed, over the 256 KiB file limit. Compact download was 242854 bytes and reopened successfully through the actual file input with all 99 slots. Transport regression tests cover this boundary. Commit `a1e3831`.
3. **Fixed — later credentials were unreachable.** The picker loaded only the first 100 records. ID-cursor pagination and a Load more control now reach subsequent pages. Both PostgreSQL and browser checks selected an active credential behind 100 revoked records. Commit `a1e3831`.

No smell-based refactor was justified. The highest-impact Standards finding was unsaved-edit loss; it is fixed.

## Spec

1. **Fixed — import violated the requirement to preserve pending edits.** Same reproduced import race and fix as the Standards finding.
2. **Fixed — browser parsing bypassed duplicate-key rejection.** Parsed objects were serialized before sending, hiding duplicates from the strict server decoder. The editor now submits the original reviewed JSON inside the bounded request envelope. Browser import of duplicate `name` fields now returns 400; transport tests preserve nested duplicate fields for server rejection. Commit `a1e3831`.
3. **Fixed — the binding UI could not select all matching credentials.** Same pagination finding and fix as the Standards review.

No material scope creep was found. The highest-impact Spec finding was unsaved-edit loss; it is fixed. N-01 independent node-author acceptance remains open.

## Additional browser finding

**Fixed — long imported node IDs widened narrow layouts.** The large fixture exposed a 547-pixel document inside a 390-pixel viewport. Node-list and selected-node labels now shrink and wrap. The document remains 390 pixels wide with a selected 64-character ID. Commit `8d10572`.

## Verification and limits

- `make test check`: Go race checks, 43 worker tests, three browser-transport tests, type checks, format checks, catalog consistency, vet, and whitespace checks passed.
- `make integration-phase4`: all four live groups and isolated PostgreSQL tests passed. [Runtime evidence](evidence/phase-4-after-review.json).
- `python3 tests/integration/phase2.py`: all seven cases passed. [Regression evidence](evidence/phase-2-after-review.json).
- OpenAPI validation passed with `openapi-spec-validator==0.7.2`.
- Browser verification used the T3 collaborative browser against owned local PostgreSQL/API/editor services. Fixture data only. Services were stopped; their data was retained.
- Runtime evidence predates the final CSS-only wrapping fix; that fix passed browser inspection and `pnpm check:web` afterward.

There were no open repository PRs/MRs or thread-linked PRs to merge. No push, deployment, provider message, or real-user identity operation occurred.
