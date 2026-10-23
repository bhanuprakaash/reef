//go:build embed

package eval

import (
	"errors"
	"os"

	"github.com/bhanuprakaash/reef/memory"
)

// Without REEF_EMBED_KEY it returns nil, which skips the scenarios that need meaning search.
func newEmbedder() (memory.Embedder, error) {
	key := os.Getenv("REEF_EMBED_KEY")
	if key == "" {
		return nil, nil
	}
	provider := os.Getenv("REEF_EMBED_PROVIDER")
	if provider == "" {
		return nil, errors.New("REEF_EMBED_KEY is set but REEF_EMBED_PROVIDER is not")
	}
	return memory.NewEmbedder(memory.EmbedConfig{Provider: provider, Key: key}, nil)
}
