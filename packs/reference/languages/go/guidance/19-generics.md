## 19. Generics

- Use type parameters for **containers and algorithms** only: a generic `Set[T]`, a `Ptr[T](v T) *T` helper, a sort comparator combinator. Do not reach for generics to fake OOP or to "deduplicate" two functions that merely *look* similar today — that duplication is cheaper than the wrong abstraction (see `17-anti-patterns.md`).
- Prefer the stdlib's generics over hand-rolled ones: `slices`, `maps`, `cmp`, `sync.OnceValue`. Check `slices`/`maps`/`cmp` before writing a generic helper — most of what this CLI needs (`slices.Sort`, `slices.SortFunc`, `slices.Contains`, `maps.Keys`) is already there.
- If a type parameter appears exactly once in a function signature, an interface was the right tool, not a type parameter.
- No exported generic type or function ships without a concrete, non-generic caller in the same PR — generics adopted "for future use" are exactly the YAGNI case this pack elsewhere warns against.
