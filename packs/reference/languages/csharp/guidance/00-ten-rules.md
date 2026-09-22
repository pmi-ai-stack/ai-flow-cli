## C#: ten rules

1. Nullable reference types enabled (`<Nullable>enable</Nullable>`) project-wide; a `!` null-forgiving operator carries a comment saying why the compiler's doubt is wrong.
2. `async`/`await` all the way up the call chain; never `.Result`/`.Wait()` on a `Task` outside a narrow, documented synchronous entry point — both risk deadlocking on a captured `SynchronizationContext`.
3. `record` types for immutable data carried across boundaries; `class` for anything with real identity or behaviour.
4. Dependency injection via the built-in container (or the framework's own), constructor injection only — no static service locator reached from deep inside a call stack.
5. `IDisposable`/`await using` for anything holding an unmanaged or pooled resource (connections, streams, `SemaphoreSlim`).
6. `CancellationToken` threaded through every async method that can meaningfully be cancelled — accepted as the last parameter, forwarded, never silently dropped.
7. LINQ for readability, not cleverness — a LINQ chain a reviewer has to trace through three `.Select()`s to understand is a `foreach` loop that would have been clearer.
8. `sealed` by default for classes not designed for inheritance; an open-for-extension class states so deliberately.
9. Configuration via `IOptions<T>`, strongly typed — never `IConfiguration["Some:Key"]` string-indexed lookups scattered through business logic.
10. `dotnet format --verify-no-changes` gates CI; a formatting disagreement in review is a `.editorconfig` PR, not a comment thread.
