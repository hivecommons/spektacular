package workflow

// Data is the step-accessible key-value store.
// Steps may read and write arbitrary values.
// The underlying storage is an interface so it can be substituted (e.g. for testing).
type Data interface {
	Get(key string) (any, bool)
	Set(key string, value any)
}

// mapData implements Data over a persistent base map with a transient overlay.
// Overlay values (injected by the workflow at call time) take precedence for
// reads but are never written to the base and therefore never persisted.
type mapData struct {
	base    map[string]any
	overlay map[string]any
}

func newMapData(base map[string]any) *mapData {
	if base == nil {
		base = map[string]any{}
	}
	return &mapData{base: base, overlay: map[string]any{}}
}

func (d *mapData) Get(key string) (any, bool) {
	if v, ok := d.overlay[key]; ok {
		return v, true
	}
	v, ok := d.base[key]
	return v, ok
}

func (d *mapData) Set(key string, value any) {
	d.base[key] = value
}

func (d *mapData) Snapshot() map[string]any {
	snapshot := make(map[string]any, len(d.base)+len(d.overlay))
	for k, v := range d.base {
		snapshot[k] = v
	}
	for k, v := range d.overlay {
		snapshot[k] = v
	}
	return snapshot
}

func (d *mapData) setOverlay(key string, value any) {
	d.overlay[key] = value
}
