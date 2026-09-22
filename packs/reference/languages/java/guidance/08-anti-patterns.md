## Anti-patterns

- Returning `null` for "not found" from a newly written API instead of `Optional<T>` — pushes a `NullPointerException` risk onto every caller.
- `catch (Exception e) { e.printStackTrace(); }` — neither logged properly nor handled; the failure is effectively invisible in production.
- A "god" `Utils`/`Helper` class that accumulates unrelated static methods instead of behaviour living on the type it belongs to.
- Field injection (`@Autowired` directly on a field) instead of constructor injection — makes a class's real dependencies invisible and untestable without the DI framework running.
- Mutable static state used for anything beyond a genuine constant — a hidden coupling between unrelated call sites and a thread-safety hazard.
- Overriding `equals()` without `hashCode()` (or vice versa) — silently breaks the type in any hash-based collection.
