## 20. Language gotchas that bite reviewers here specifically

- **Shadowing with `:=`.** `:=` may only *reuse* an existing variable when it was declared earlier in the *same* block and at least one name on the left is new — that's the redeclaration rule. Stepping into a new block (`if`/`for`/…) is a different scope, so `:=` there always declares fresh variables, even for names that already exist outside it; the outer ones are left untouched:

  ```go
  var client *tracker.Client
  if cfg.UseCache {
      client, err := newCachedClient(cfg) // NEW client, new scope; outer client stays nil
      _ = client
  }
  ```

  Use `=` when the intent is to assign the outer variable, or declare it above the block first. Nothing in this repo's current `.golangci.yml` catches this automatically — `govet`'s shadow analyzer is off by default and isn't enabled here; review the diff for it by eye. **[review]**

- **Preallocate when the size is known.** `make([]T, 0, len(targets))` before a loop that appends once per target; don't guess a magic number. This applies to the risk/standards finding slices before they're sorted (`07-determinism.md`), not just to marshaled output.

- **`nil` slice vs empty slice, in logic, not just marshaling.** A `nil` slice is safe to `range` and `append` to. Check emptiness with `len(s) == 0`. Slices are not comparable to each other at all — `a == b` for two slice variables is a compile error ("slice can only be compared to nil"); the *only* legal comparison is to the literal `nil`, and a `[]T{}` is not `== nil` while a `nil` slice is. (`10-data-types.md` covers the JSON-marshaling half: `nil` → `null`.)

- **Strings are immutable UTF-8 byte sequences.** `len(s)` counts bytes, not characters; `for i, r := range s` iterates runes; `s[i]` is a single byte. Don't index into a string expecting a character.
- Build strings with `strings.Builder` (or `fmt.Sprintf` for a one-shot), never `+=` in a loop — `+=` reallocates and copies on every iteration.
- `strings.TrimSuffix(s, ".yaml")` removes exactly that suffix; `strings.TrimRight(s, ".yaml")` removes any trailing run of the *characters* `.`, `y`, `a`, `m`, `l` — not the same thing, and a common cause of subtly wrong path handling. Use `TrimSuffix`/`TrimPrefix` unless you specifically want the character-set behavior.
