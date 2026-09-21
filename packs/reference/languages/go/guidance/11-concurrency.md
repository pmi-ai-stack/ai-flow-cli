## 11. Concurrency

The CLI is mostly sequential. Parallelize only when a stage runs several independent targets and it is measurably slow.

- The one pattern: `errgroup.WithContext` + `SetLimit(n)`, each goroutine writes its own index of a preallocated slice, results sorted afterwards so output stays byte-stable. **[review]**
- Every goroutine has an owner that waits for it and a `ctx` that stops it. No `go f()` fire-and-forget. No goroutines in `init()`.
- `sync.Mutex` as a named field `mu` above the fields it guards; never copy a struct containing one. **[tool: `go vet copylocks`]**
- No `time.Sleep` in production code; backoff for tracker/host retries uses a ticker or `context` deadline with jitter and a max attempt count from config.
- `go test -race` always. **[tool]**

