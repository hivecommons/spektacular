package design

import (
	"errors"
	"path/filepath"

	"github.com/jumppad-labs/spektacular/internal/config"
	"github.com/jumppad-labs/spektacular/internal/store"
)

// providerFile is the one design storage backend this release ships. It is an
// alias for the shared config constant so the error text and the provider
// switch cannot drift apart.
const providerFile = config.ProviderFile

// isAbs, joinPath and isNotFound keep the filepath and sentinel details out of
// the error constructors, which are otherwise pure message text.

func isAbs(path string) bool { return filepath.IsAbs(path) }

func joinPath(location, path string) string {
	return filepath.Join(location, filepath.FromSlash(path))
}

func isNotFound(err error) bool { return errors.Is(err, store.ErrNotFound) }
