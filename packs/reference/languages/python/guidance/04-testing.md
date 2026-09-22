## Testing

1. `pytest` fixtures for setup/teardown, not a hand-rolled `setUp`/`tearDown` class pair.
2. `pytest.mark.parametrize` for a family of similar cases instead of copy-pasted near-identical test functions.
3. `monkeypatch`/`unittest.mock` at the boundary (network, filesystem, clock, environment) only — never patch the function under test itself to make an assertion trivially true.
4. `freezegun` (or an injected clock) for anything time-dependent; a test that's flaky near midnight or DST boundaries is a test bug.
5. Property-based tests (`hypothesis`) for functions with a large, well-defined input space (parsers, serializers, numeric code) where example-based tests would only ever cover a handful of cases.
6. `pytest.raises` asserts both the exception type and, where it matters, the message — a test that only checks "something raised" doesn't prove the right thing failed.
