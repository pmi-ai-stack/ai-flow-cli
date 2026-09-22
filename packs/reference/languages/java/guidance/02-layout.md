## Layout

```
src/main/java/<group>/<artifact>/<area>/Thing.java
src/test/java/<group>/<artifact>/<area>/ThingTest.java
```

- The Maven/Gradle standard directory layout, unmodified — `src/main/java`, `src/test/java` — so every tool in the ecosystem finds code where it expects it.
- A test class mirrors its subject class's package and name, suffixed `Test` (`OrderServiceTest` for `OrderService`).
- A package that accumulates unrelated classes splits along the same seams the classes already suggest — group by feature/domain area, not by technical layer alone (`billing/`, not a project-wide `services/`, `models/`, `utils/` split).
