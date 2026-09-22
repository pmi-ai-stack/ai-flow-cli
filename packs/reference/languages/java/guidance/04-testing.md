## Testing

1. `JUnit 5`'s `@Test`, `@ParameterizedTest` + `@ValueSource`/`@MethodSource` for a family of similar cases instead of copy-pasted near-identical `@Test` methods.
2. Test method names read as a sentence (`shouldThrowWhenBalanceInsufficient`, or `@DisplayName` when the method name alone can't say it cleanly).
3. `Mockito` mocks the boundary (a repository, an external client) — never the class under test's own internals; over-mocking a collaborator that's cheap to construct for real is a smell.
4. Given/When/Then (or Arrange/Act/Assert), visually separated, so a reviewer finds the actual assertion immediately.
5. `AssertJ`'s fluent assertions over raw `assertEquals` chains for anything beyond a single scalar comparison — the failure messages are dramatically more useful.
6. Integration tests needing a real dependency (a database, a broker) use `Testcontainers` — never a shared, stateful test environment other suites can leave dirty.
