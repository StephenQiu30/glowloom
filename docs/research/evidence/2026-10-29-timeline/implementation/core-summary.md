# Timeline core implementation verification

Implemented the contract from docs/design/31-时间线即梦一致性设计.md.

- Mixed image/video primary visual lane; legacy image lanes remain readable.
- Import-only command and independent imported_assets identities; deduplicated, capped at 1000 including legacy clip assets; assets retained after clip deletion.
- Visual trim/delete/resize closes the lane; visual move inserts/reorders; audio placement stays absolute.
- Edge resize clamps source ranges, neighbours and the 100 ms minimum.
- Preview time uses MM:SS:centiseconds and ruler uses MM:SS.
- Paused seeks exact; refreshed URLs restore source position; auto ratio uses first visual source; visual end remains black while longer audio continues.
- Media failures isolated per asset, expiry timers independent; explicit retry clears prior URLs before replacing their timer generation.
- Backend validates mixed lanes and imported identities, checks ownership/readiness, protects unused imports from deletion, remaps imports during project copy.
- Yjs timeline fingerprints normalize legacy imports to avoid false CAS conflicts.

Validation:
- Initial focused frontend red run: 11 failed / 11 passed before implementation.
- Core model/preview/media plus complete yjs-live-session: 4 files, 83 tests PASS before final pending-retry case.
- Final media regression run: 6 tests PASS, including pending explicit retry authorization expiry.
- tsc --noEmit: PASS (core-tsc.log).
- ESLint core implementation/test files: PASS.
- Go -race with isolated PostgreSQL18: all TestTimeline*, TestProjectCopyRemaps*, and retained-config reference guard PASS (core-go-test.log).
- Database tests prove shelf persistence without clips, foreign/missing asset denial, unused imported reference protection, source-compatible aspect persistence, timeline freeze and copy mapping.

The database lives only in a dedicated temporary Docker Compose project on loopback port15547, with synthetic fixtures. Compose was retained for the export agent's HTTP/media integration tests. Main project services/data were untouched. No commits or generated API edits performed by this agent.

Read-only workspace review was sent to parent: queue keys and target persistence, missing drop/trim epoch checks, result identity after save, gesture cancellation and volume undo boundaries. Parent owns UI fixes and visual acceptance.
