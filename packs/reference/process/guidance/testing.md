## Testing

1. Red first. Write the failing test against the unmet requirement before the code that satisfies it — a test written after the implementation tends to test what the code does, not what it's supposed to do.
2. Never edit an existing test to make it pass again unless the *requirement* it encodes actually changed — if the code is wrong, fix the code.
3. A test's name states the behaviour under test and the condition, not the function under test: `RejectsExpiredToken`, not `TestValidate2`.
4. Unit tests are fast, isolated and deterministic — no network, no real clock, no shared mutable state between tests, no dependence on run order.
5. Test the boundary, not just the happy path: empty input, the maximum, one past the maximum, the error path, concurrent access where it's relevant.
6. A mutation score below threshold is a conversation about test adequacy, not a number to explain away — `mutation summarize`'s `add-tests`/`accept` choice is a judgement call with a written reason, never a rubber stamp.
