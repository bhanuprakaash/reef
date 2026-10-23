//go:build embed

package eval

import (
	"os"

	"github.com/bhanuprakaash/reef/memory"
)

// Without REEF_EMBED_KEY it returns nil, which skips the scenarios that need meaning search.
func newEmbedder() (memory.Embedder, error) {
	key := os.Getenv("REEF_EMBED_KEY")
	if key == "" {
		return nil, nil
	}
	return memory.NewEmbedder(memory.EmbedConfig{Provider: os.Getenv("REEF_EMBED_PROVIDER"), Key: key}, nil)
}
