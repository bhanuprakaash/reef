//go:build !embed

package eval

import "github.com/bhanuprakaash/reef/memory"

func newEmbedder() (memory.Embedder, error) { return nil, nil }
