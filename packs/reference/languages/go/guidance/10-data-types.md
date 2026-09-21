## 10. Data types

- Structs with `json:"snake_case"` tags matching the schema field names byte for byte. The schema file is the source of truth; a struct field without a schema property is a bug. **[test: marshal the zero+sample value and validate against the embedded schema]**
- Enums: `type Action int` with `iota` starting at an `Unknown` zero value, a `String()` method, and `MarshalText`/`UnmarshalText` so JSON carries `"create"`, not `1`. Table-test the round trip.
- Severity, tier, layer, kind class, stage state and stage name are all typed enums, never bare strings compared with `==`.
- `time.Duration` for durations; `time.Time` never appears in a results struct (see §7).
- Slices returned from a package are the caller's to keep: return a fresh slice (`slices.Clone`) when the package retains a copy.
- `nil` slices marshal as `null`; when the schema says `array`, initialise with `[]T{}` in the constructor or use `omitempty` deliberately — pick one per struct and test it.

