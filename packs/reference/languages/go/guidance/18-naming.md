## 18. Naming, doc comments, control flow

- **`MixedCaps`, never `snake_case` or `SCREAMING_CONST`.** `maxRetries`, not `MAX_RETRIES`. **[tool: `gofmt`/`revive`]**
- **Initialisms stay uppercase**: `userID`, `HTTPClient`, `parseURL`, `ServeHTTP` — never `UserId`, `HttpClient`.
- **Scope-proportional names.** `i`, `n`, `err`, `ctx`, `r`, `w` in a five-line function; a descriptive name (`targetID`, `changesetPath`) once the scope spans a screen. Receivers are 1–2 letters, consistent across every method of the type (`func (c *Client) ...`), never `this`/`self`.
- **No stutter, no `Get`.** `risk.Score`, not `risk.RiskScore`. `cfg.Timeout()`, not `cfg.GetTimeout()`. (Also in `02-layout.md` §2; repeated here because it's a naming rule, not a layout rule.)
- **One-method interfaces end in `-er`**: `Renderer`, `Resolver`, `CheckRunner`. Booleans read as predicates: `isReady`, `dryRun`, `hasWaiver` — never `ready` or `dry`.
- **Struct literals name their fields**: `Row{Tool: "terraform", Verb: "plan", Mutating: false}`, never positional. Omit zero-valued fields.
- **Enums start at an `Unknown`/zero value** via `iota`, so the zero value is never silently a valid state: `type Stage int; const ( StageUnknown Stage = iota; StagePlan; StageApply )`. (Cross-reference: `10-data-types.md` covers the `String()`/`MarshalText` half of this.)

### Doc comments

- Every exported identifier gets a doc comment: a full sentence, starting with the identifier's own name. `// Score computes the deterministic risk score of a changeset.`, not `// computes risk score` or `// This function computes...`. **[tool: `revive` exported-comment check]**
- One package comment per package, in `doc.go` or the file holding the package's main type: `// Package risk computes the deterministic risk score of a changeset.` (See `02-layout.md` §2.)

### Control flow: early return / line of sight

Handle the error, return, and keep the happy path un-indented. No `else` after a `return`.

```go
// bad — happy path buried inside the else
if err == nil {
    if cfg.Enabled {
        render()
    }
} else {
    return err
}

// good — line of sight
if err != nil {
    return err
}
if !cfg.Enabled {
    return nil
}
render()
```

A function that needs more than two levels of nesting to read is a signal to extract a helper, not to keep indenting. **[review]**
