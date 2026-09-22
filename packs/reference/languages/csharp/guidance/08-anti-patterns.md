## Anti-patterns

- `.Result`/`.Wait()` on an async call from synchronous code — the classic deadlock-on-`SynchronizationContext` trap, and a needless block even where it happens not to deadlock.
- `async void` methods outside a UI/framework event handler — an exception inside one crashes the process instead of being observable by a caller.
- A static, mutable singleton used as ambient state instead of dependency injection — untestable, and a hidden coupling between unrelated call sites.
- Catching `Exception` and logging it without rethrowing or converting, effectively hiding a failure the caller needed to know about.
- Overusing reflection or `dynamic` where a generic or an interface would give the compiler (and the reader) the same flexibility with actual type safety.
- Large, mutable DTOs passed by reference through many layers instead of small, immutable `record`s scoped to what each layer actually needs.
