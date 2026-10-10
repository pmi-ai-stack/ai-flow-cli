package kubectl

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"reflect"
	"sort"
)

// decodeJSONStream decodes data as a sequence of concatenated top-level
// JSON values — kubectl's own output shape for multiple resources
// varies by verb ("apply -o json" against several manifests
// concatenates independent objects with no wrapping array or
// separator; "get -f <dir> -o json" against several resources wraps
// them in one {"kind":"List","items":[...]}" object) — one decoder
// loop handles both: json.Decoder.Decode consumes exactly one
// top-level value per call regardless of which shape is present, and
// any decoded List value is flattened into its own items. See
// .ai/phase-22/plan.md §0.8 — this shape is not recorded from a real
// binary (none is available in this environment) and is built from
// kubectl's publicly documented output conventions instead.
func decodeJSONStream(data []byte) ([]map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	var out []map[string]any
	for {
		var v map[string]any
		if err := dec.Decode(&v); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("kubectl: decode kubectl JSON output: %w", err)
		}
		if kind, _ := v["kind"].(string); kind == "List" {
			if items, ok := v["items"].([]any); ok {
				for _, item := range items {
					if m, ok := item.(map[string]any); ok {
						out = append(out, m)
					}
				}
				continue
			}
		}
		out = append(out, v)
	}
	return out, nil
}

// volatileMetadataKeys are the server-set metadata fields stripped
// before any comparison — fields no manifest ever declares, so
// comparing them would report drift/changes for information this
// ticket's own change never touched.
var volatileMetadataKeys = []string{
	"resourceVersion", "uid", "generation", "creationTimestamp", "managedFields",
}

const lastAppliedConfigAnnotation = "kubectl.kubernetes.io/last-applied-configuration"

// stripVolatile returns a shallow copy of obj with "status" and the
// known server-set metadata fields removed.
func stripVolatile(obj map[string]any) map[string]any {
	if obj == nil {
		return nil
	}
	out := make(map[string]any, len(obj))
	for k, v := range obj {
		if k == "status" {
			continue
		}
		out[k] = v
	}
	if meta, ok := out["metadata"].(map[string]any); ok {
		meta = stripVolatileMetadata(meta)
		out["metadata"] = meta
	}
	return out
}

func stripVolatileMetadata(meta map[string]any) map[string]any {
	out := make(map[string]any, len(meta))
	maps.Copy(out, meta)
	for _, k := range volatileMetadataKeys {
		delete(out, k)
	}
	if ann, ok := out["annotations"].(map[string]any); ok {
		a := make(map[string]any, len(ann))
		for k, v := range ann {
			if k == lastAppliedConfigAnnotation {
				continue
			}
			a[k] = v
		}
		out["annotations"] = a
	}
	return out
}

// topLevelDiff returns the sorted, deduplicated top-level keys present
// in before or after whose values differ (a key missing from one side
// and present in the other counts as changed) — the same "names only,
// never values" shape terraform's own changedFields already
// established (.ai/phase-21/plan.md design decision 4; this phase's
// own §1.2.2 records why it is not deepened here either).
func topLevelDiff(before, after map[string]any) []string {
	seen := map[string]bool{}
	for k := range before {
		seen[k] = true
	}
	for k := range after {
		seen[k] = true
	}

	var changed []string
	for k := range seen {
		bv, bok := before[k]
		av, aok := after[k]
		if bok != aok || !reflect.DeepEqual(bv, av) {
			changed = append(changed, k)
		}
	}
	sort.Strings(changed)
	return changed
}
