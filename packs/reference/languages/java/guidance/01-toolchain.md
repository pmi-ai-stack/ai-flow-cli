## Toolchain

- LTS JDK version pinned in the build file (Maven's `maven.compiler.release`, Gradle's `toolchain`); CI provisions exactly that version.
- Maven or Gradle, one per repository — not both competing for the same build.
- `JUnit 5` as the one test framework for new code; `JUnit 4` suites inherited from an older codebase are fine to keep, never newly written.
