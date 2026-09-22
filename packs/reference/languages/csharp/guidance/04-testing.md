## Testing

1. `xUnit`'s `[Fact]`/`[Theory]` (`[Theory]` + `[InlineData]`/`[MemberData]` for a parametrized family of cases) rather than copy-pasted near-identical `[Fact]`s.
2. Test method names read as a sentence: `MethodName_Scenario_ExpectedBehavior` (`Withdraw_InsufficientFunds_ThrowsInsufficientFundsException`).
3. `Moq`/`NSubstitute` mocks the boundary (a repository interface, an external client) — never the class under test's own concrete internals.
4. Arrange/Act/Assert, visually separated (a blank line between each) so a reviewer can find the actual assertion immediately.
5. Async test methods return `Task`, never `async void` — an exception thrown from `async void` is unobservable by the test runner and reports as a false pass.
6. Integration tests that need a real dependency (a database, a queue) use `Testcontainers` or an in-memory equivalent — never a shared, stateful test environment other suites can leave dirty.
