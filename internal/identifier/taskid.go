package identifier

import (
	"crypto/rand"
	"fmt"
	"sort"
	"strings"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/output"
)

// CodeTaskIDProviderUnknown refuses a plan.task_id.provider this build does
// not implement.
const CodeTaskIDProviderUnknown = "task_id_provider_unknown"

// TaskIDProvider issues one new plan task identifier. Consumers treat ids as
// opaque strings.
//
// A provider that creates something external when it issues an id (a ticket,
// a record in another system) must tolerate ids issued for tasks that are
// later discarded: an author may request an id and never save the task.
type TaskIDProvider func() (string, error)

// taskIDProviders maps a plan.task_id.provider name to its implementation.
// Adding a provider is adding an entry; the plan format and export are
// unaffected.
var taskIDProviders = map[string]TaskIDProvider{
	config.DefaultTaskIDProvider: newUUIDv4,
}

// TaskIDProviderFor returns the provider configured under name. An empty name
// is the default, uuid. An unknown name is refused here, when an id is
// requested, rather than when config loads, so a bad setting blocks only id
// requests and not every command.
func TaskIDProviderFor(name string) (TaskIDProvider, error) {
	if name == "" {
		name = config.DefaultTaskIDProvider
	}
	if p, ok := taskIDProviders[name]; ok {
		return p, nil
	}
	available := make([]string, 0, len(taskIDProviders))
	for n := range taskIDProviders {
		available = append(available, n)
	}
	sort.Strings(available)
	return nil, output.NewError(CodeTaskIDProviderUnknown,
		fmt.Sprintf("task id provider %q does not exist", name)).
		WithResource(name).
		WithNextAction("set plan.task_id.provider in .spektacular/config.yaml to one of: " + strings.Join(available, ", "))
}

// newUUIDv4 returns a random (version 4) UUID, RFC 4122 §4.4: 16 random bytes
// with the version nibble set to 4 and the variant bits to 10.
func newUUIDv4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generating task id: %w", err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
