## Anti-patterns

- `any` sprayed across a function signature to silence the compiler instead of fixing the actual type mismatch.
- A giant `useEffect` (or lifecycle hook) doing three unrelated things instead of three small, independently readable ones.
- Prop drilling five components deep instead of context, a store, or restructuring the component tree.
- Mutating a parameter or a shared object in place when the caller expects a pure function — surprising callers who assumed the input was untouched.
- `JSON.parse(JSON.stringify(x))` as a "deep clone" — silently drops `Date`, `Map`, `Set`, functions and `undefined` values.
- A `switch` over a union type with no `default` asserting `never` — the compiler can't catch a missed case added later without it.
