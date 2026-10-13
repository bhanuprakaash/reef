# Go style

Condensed from the [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md). Where Uber and this file differ, this file wins. `gofmt` and `go vet` are the floor; everything below is on top of them.

## Guidelines

**Interfaces**
- Pass interfaces by value, not as pointers. An interface already holds a pointer when it needs one.
- Verify compliance at compile time: `var _ Agent = (*Claude)(nil)`.
- Methods on a value receiver are callable on pointers and values; pointer-receiver methods only on pointers. Pick one receiver kind per type.

**Mutexes**
- The zero value of `sync.Mutex` is valid; do not use a pointer to a mutex.
- Embed the mutex unexported in the struct; never embed it in an exported struct.

**Slices and maps**
- Copy slices and maps when you receive them and store them, and when you return internal state. Callers must not be able to mutate your state.

**Defer**
- Use `defer` to clean up files, locks and connections. The cost is negligible; the readability is not.

**Channels**
- Channel size is 1 or none. Any other size needs a written reason.

**Enums**
- Start enums at `iota + 1` unless the zero value is a meaningful default.

**Time**
- `time.Time` for instants, `time.Duration` for periods. Never `int` seconds.
- When talking to external systems (JSON, SQLite), use RFC 3339 strings for instants and `time.Duration` with explicit units (`timeoutMs`) for periods.

**Errors**
- Error types: `errors.New` for static messages; `fmt.Errorf` for dynamic; a custom type when callers need to match or extract fields; export a sentinel (`var ErrNotFound = errors.New(...)`) when callers need `errors.Is`.
- Wrap with `%w` and add context the caller lacks. Do not add "failed to"; the chain already reads as failure: `fmt.Errorf("open store: %w", err)`.
- Name exported sentinels `ErrX`, custom types `XError`.
- Handle each error once: log it or return it, never both.

**Type assertions**
- Always use the two-value form: `v, ok := x.(T)`.

**Panic**
- Do not panic in library code. Return errors. `main` may exit on fatal errors, once, via a single `run() error` called from `main`.

**Atomics**
- Use typed atomics from `sync/atomic` (`atomic.Int64`), never raw `atomic.AddInt64` on a plain int.

**Globals**
- No mutable globals. Inject dependencies into constructors.
- Unexported package-level vars and consts are prefixed `_` (`_defaultTimeout`), so a read inside a function shows the scope at a glance.

**Embedding**
- Do not embed types in public structs; it leaks the inner API. Embedding is for behaviour the outer type should fully expose.

**Built-in names**
- Never shadow `error`, `string`, `len`, `new`, `copy`, and so on.

**init**
- Avoid `init()`. Do work in constructors and in `main`. No goroutines, I/O or global state in `init`.

**Goroutines**
- No fire-and-forget. Every goroutine has a way to be stopped and waited for (`sync.WaitGroup` or `errgroup`, plus a context or a done channel).
- No goroutines in `init`.

**Marshaled structs**
- Every field of a struct that is encoded to JSON or another format carries an explicit tag.

## Performance

- `strconv` over `fmt` for number conversions.
- Convert a fixed string to `[]byte` once, not in a loop.
- Give `make` a capacity when you know it: `make([]T, 0, n)`, `make(map[K]V, n)`.

## Style

- Lines around 99 columns; break before it gets hard to read, not at a hard limit.
- Be consistent with the surrounding code before being right in the abstract.
- Group similar declarations: one `const (...)`, `var (...)`, `type (...)` block per related set. Do not group unrelated ones.
- Imports in two groups: standard library, then everything else. `goimports` does this.
- Package names: short, lowercase, one word, no underscores, not the name of a common variable. Not `util`, `common`, `helpers`.
- Function names: `MixedCaps`. Test functions may use underscores to separate the case: `TestStore_Create`.
- Alias an import only when the package name differs from the last path element, or two imports collide.
- In a file: types first, then constructors, then methods grouped by receiver. Exported before unexported. Plain utility functions at the bottom.
- Reduce nesting: handle the error or the edge case first and return; keep the main path at the lowest indent.
- No `else` after a branch that returns.
- Top-level `var` without an explicit type when the right-hand side makes it obvious: `var _s = F()`. Keep the type when it does not.
- Local variables: `:=` by default; `var x T` when the zero value is what you want.
- `nil` is a valid empty slice. Return `nil`, not `[]T{}`. Check emptiness with `len(s) == 0`.
- Declare variables in the smallest scope: `if err := f(); err != nil {`.
- No naked parameters when the meaning is unclear: `print(true /* isLocal */, true /* done */)` is worse than named constants or a small options struct.
- Raw string literals for strings with quotes or backslashes: `` `{"a":"b"}` ``.
- Initialize structs with field names. Omit fields with zero values. `var s T` for a zero-value struct. `&T{...}` not `new(T)`.
- Initialize maps with `make` when empty and sized, with a literal when filled in place.
- Build format strings outside `Printf` only as `const`; if a `Printf`-style function takes a format, end its name with `f` so `go vet` checks it.

## Patterns

**Table tests**
- Use `tests := []struct{ name string; give X; want Y }` with `for _, tt := range tests { t.Run(tt.name, ...) }`.
- Fields are `give` and `want`. Expected values are literals. One assertion idea per row.
- When the rows need different setup or control flow, write separate tests instead of a table with flags.
- Parallel subtests must capture `tt := tt` only on Go < 1.22; on current Go the loop variable is per-iteration.

**Functional options**
- Not used until a constructor has more than three optional settings. Until then, a config struct.

## Linting

`golangci-lint` with: `errcheck`, `goimports`, `govet`, `revive`, `staticcheck`, `unused`. Config lives in `.golangci.yml` at the repo root once the first Go package exists.
