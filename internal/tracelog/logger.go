package tracelog

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hivecommons/spektacular/internal/output"
)

// ProviderFile is the name of the file logger, the only logger that ships.
const ProviderFile = "file"

// Logger writes one record for one run. Write never returns an error and
// never panics outward: a logging failure can never change a command.
type Logger interface {
	Write(run Run, rec Record)
}

// FileOptions configures the file logger: the absolute directory logs go
// to, and how many recent runs (and, separately, days of standalone
// commands) to keep.
type FileOptions struct {
	Directory string
	Retention int
}

// providers maps a log.provider name to its constructor. Adding a logger is
// adding an entry; the recorder is unaffected.
var providers = map[string]func(FileOptions) Logger{
	ProviderFile: newFileLogger,
}

// New returns the logger configured under provider. An unknown name is a
// configuration error naming the accepted loggers.
func New(provider string, opts FileOptions) (Logger, error) {
	if ctor, ok := providers[provider]; ok {
		return ctor(opts), nil
	}
	return nil, output.NewError("config_invalid",
		fmt.Sprintf("log.provider %q is not a known logger", provider)).
		WithResource(provider).
		WithNextAction("set log.provider in .spektacular/config.yaml to one of: " + strings.Join(Providers(), ", "))
}

// Providers returns the accepted logger names, sorted.
func Providers() []string {
	names := make([]string, 0, len(providers))
	for n := range providers {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
