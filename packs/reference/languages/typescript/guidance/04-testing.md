## Testing

1. `describe`/`it` names read as a sentence: `describe("InvoiceService")` → `it("rejects a negative amount")`.
2. Mock at the boundary (network, filesystem, clock), never the module under test's own internals — a test that mocks its own subject proves nothing.
3. `async`/`await` in every async test; a dangling unresolved promise in a test silently passes in some runners and must not be relied on.
4. Snapshot tests are for stable, reviewed output (rendered markup, serialized config) — never for anything that legitimately changes often, where they just become noise to blindly re-approve.
5. Fake timers (`jest.useFakeTimers()` or equivalent) for anything involving `setTimeout`/`setInterval` — a real-time sleep in a test is a flaky test waiting to happen.
6. `e2e`/Playwright suites assert user-visible outcomes (text on screen, navigation, a persisted record) — not implementation details like a specific internal function having been called.
