package agent

const _tailKeep = 4 << 10

// Tail is an io.Writer that keeps the last 4 KB written to it, for the stderr of a backend.
// Its zero value is ready to use.
type Tail struct{ b []byte }

func (t *Tail) Write(p []byte) (int, error) {
	t.b = append(t.b, p...)
	if len(t.b) > _tailKeep {
		t.b = t.b[len(t.b)-_tailKeep:]
	}
	return len(p), nil
}

// Bytes returns what is kept.
func (t *Tail) Bytes() []byte { return t.b }
