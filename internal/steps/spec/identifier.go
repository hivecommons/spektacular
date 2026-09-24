package spec

import (
	"errors"
	"fmt"
	"time"

	"github.com/hivecommons/spektacular/internal/identifier"
	"github.com/hivecommons/spektacular/internal/store"
)

// IdentifierRequest describes the data needed to resolve a canonical spec name.
type IdentifierRequest struct {
	Name    string
	ID      string
	Method  string
	SpecDir string // configured spec directory; defaults to config.DefaultSpecDir when empty
	Store   store.Store
	Now     func() time.Time
}

// IdentifierResult is the canonical spec name.
type IdentifierResult struct {
	Name string
}

// ResolveIdentifier turns a requested spec name plus optional id into a
// canonical spec name. All ID rules (normalization, timestamp/counter/
// external minting, existence-check retry) live in internal/identifier;
// this function supplies only what is spec-specific — SpecFilePath as the
// path shape existence checks resolve against.
func ResolveIdentifier(req IdentifierRequest) (IdentifierResult, error) {
	resolved, err := identifier.Resolve(identifier.Request{
		Name:     req.Name,
		ID:       req.ID,
		Method:   req.Method,
		Dir:      req.SpecDir,
		Store:    req.Store,
		PathFunc: SpecFilePath,
		Now:      req.Now,
	})
	if err != nil {
		var unsupported *identifier.UnsupportedMethodError
		if errors.As(err, &unsupported) {
			return IdentifierResult{}, fmt.Errorf("spec.id_method must be one of %q, %q, or %q", identifier.MethodTimestamp, identifier.MethodCounter, identifier.MethodExternal)
		}
		return IdentifierResult{}, err
	}
	return IdentifierResult{Name: resolved.Name}, nil
}
