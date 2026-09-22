## Dependencies

- A BOM (`dependencyManagement` in Maven, a Gradle platform) pins every dependency's version once for the whole build — no per-module version drift.
- OWASP Dependency-Check (or equivalent) in CI; a `critical`/`high` CVE with a fix available blocks merge.
- License review for any new dependency outside the org's approved set (`dependencies.json`'s own `allowed_licenses`).
- Prefer the JDK's own primitives (`java.time`, `java.util.concurrent`) over a third-party package that duplicates them, unless there's a concrete, stated reason.
