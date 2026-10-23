//go:build embed

package eval

import (
	"os"

	"github.com/bhanuprakaash/reef/memory"
)

// newEmbedder reads REEF_EMBED_PROVIDER and REEF_EMBED_KEY. Without a key it
// returns nil, so the scenarios that need meaning search are skipped.
func newEmbedder() (memory.Embedder, error) {
	key := os.Getenv("REEF_EMBED_KEY")
	if key == "" {
		return nil, nil
	}
	return memory.NewEmbedder(memory.EmbedConfig{Provider: os.Getenv("REEF_EMBED_PROVIDER"), Key: key}, nil)
}
