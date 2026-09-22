## Java: ten rules

1. `Optional<T>` for a return value that may legitimately be absent; never `null` for "not found" on a new API — existing `null`-returning APIs are not retrofitted mid-unrelated-change.
2. Immutable by default: `final` fields, records (`record Point(int x, int y) {}`) for plain data carriers, builders for anything with many optional construction parameters.
3. Constructor injection for dependencies (Spring, or manual DI) — no field injection (`@Autowired` on a field) and no static service locator reached from deep in a call stack.
4. Checked exceptions only for conditions a caller can genuinely recover from and is expected to handle; everything else is an unchecked `RuntimeException` subtype.
5. `try-with-resources` for anything `AutoCloseable` — streams, connections, locks — never a manual `finally { close(); }`.
6. Interfaces define behaviour; a class implementing more than one unrelated interface is a sign it's doing too much.
7. Streams (`Stream<T>`) for genuinely functional transformations; a `for` loop is still the right tool when there's meaningful side-effecting or early-exit logic — don't force a stream where readability suffers.
8. `equals()`/`hashCode()` (or a `record`) for any type used as a map key or set element; never rely on default identity equality there by accident.
9. Package-private visibility by default within a module; `public` only for the deliberate cross-package API.
10. `checkstyle`/the configured formatter gates CI; a style disagreement in review is a config PR, not a comment thread.
