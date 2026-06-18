package audit

import (
	"encoding/json"
	"maps"
	"testing"
	"time"
)

func baseEvent() Event {
	return Event{
		SchemaVersion: SchemaVersion,
		Seq:           1,
		Timestamp:     time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC),
		Event:         "get",
		App:           "myapp",
		Env:           "prod",
		Fields:        map[string]any{"key": "DATABASE_URL"},
		Prev:          initialPrev(),
	}
}

func TestComputeHash_Deterministic(t *testing.T) {
	e := baseEvent()
	h1 := computeHash(e.Prev, e)
	h2 := computeHash(e.Prev, e)
	if h1 != h2 {
		t.Fatalf("computeHash not deterministic: %s vs %s", h1, h2)
	}
	if len(h1) != 64 {
		t.Fatalf("hash should be 64 hex chars, got %d", len(h1))
	}
	for _, r := range h1 {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			t.Fatalf("hash contains non-hex char %q", r)
		}
	}
}

func TestComputeHash_ChangesOnAnyField(t *testing.T) {
	orig := baseEvent()
	origHash := computeHash(orig.Prev, orig)

	cases := []struct {
		name   string
		mutate func(*Event)
	}{
		{"app", func(e *Event) { e.App = "other" }},
		{"env", func(e *Event) { e.Env = "staging" }},
		{"event", func(e *Event) { e.Event = "set" }},
		{"timestamp", func(e *Event) { e.Timestamp = e.Timestamp.Add(time.Second) }},
		{"seq", func(e *Event) { e.Seq = 2 }},
		{"schema_version", func(e *Event) { e.SchemaVersion = 2 }},
		{"fields_key", func(e *Event) { e.Fields = map[string]any{"different": "DATABASE_URL"} }},
		{"fields_value", func(e *Event) { e.Fields = map[string]any{"key": "OTHER_URL"} }},
		{"local_only", func(e *Event) { e.LocalOnly = true }},
		{"prev", func(e *Event) { e.Prev = "ff" + e.Prev[2:] }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := orig
			// Defensive copy of Fields since the struct shares the map header.
			if orig.Fields != nil {
				e.Fields = make(map[string]any, len(orig.Fields))
				maps.Copy(e.Fields, orig.Fields)
			}
			tc.mutate(&e)
			h := computeHash(e.Prev, e)
			if h == origHash {
				t.Fatalf("expected hash to change after mutating %s", tc.name)
			}
		})
	}
}

func TestComputeHash_FieldsMapOrderIndependent(t *testing.T) {
	a := baseEvent()
	a.Fields = map[string]any{"alpha": 1, "bravo": 2, "charlie": 3}

	b := baseEvent()
	b.Fields = map[string]any{}
	for _, k := range []string{"charlie", "bravo", "alpha"} {
		b.Fields[k] = map[string]int{"alpha": 1, "bravo": 2, "charlie": 3}[k]
	}

	if computeHash(a.Prev, a) != computeHash(b.Prev, b) {
		t.Fatal("hash should be insensitive to Fields map insertion order")
	}
}

// Confirms encoding/json sorts map keys for map[string]X types. This is the
// load-bearing assumption behind using stdlib json.Marshal as our canonical
// encoder. See https://pkg.go.dev/encoding/json#Marshal — "Map values encode
// as JSON objects. The map's key type must... the keys are sorted...".
func TestJSONMarshal_SortsMapKeys(t *testing.T) {
	m := map[string]any{"zeta": 1, "alpha": 2, "mu": 3}
	got, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"alpha":2,"mu":3,"zeta":1}`
	if string(got) != want {
		t.Fatalf("json.Marshal map ordering changed: got %s want %s", got, want)
	}
}

func TestComputeHash_ClearsHashField(t *testing.T) {
	e := baseEvent()
	h1 := computeHash(e.Prev, e)

	e.Hash = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	h2 := computeHash(e.Prev, e)
	if h1 != h2 {
		t.Fatal("computeHash should ignore the existing Hash field")
	}
}
