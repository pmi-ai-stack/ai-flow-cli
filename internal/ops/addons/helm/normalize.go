package helm

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
// JSON values — identical to kubectl's own, needed here by the
// "template + kubectl diff" fallback path (dryrun.go), which reuses
// kubectl's own JSON-based get/apply comparison verbatim.
func decodeJSONStream(data []byte) ([]map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	var out []map[string]any
	for {
		var v map[string]any
		if err := dec.Decode(&v); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("helm: decode kubectl JSON output: %w", err)
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

var volatileMetadataKeys = []string{
	"resourceVersion", "uid", "generation", "creationTimestamp", "managedFields",
}

const lastAppliedConfigAnnotation = "kubectl.kubernetes.io/last-applied-configuration"

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
// in before or after whose values differ.
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

func indexByAddress(objs []map[string]any) map[string]map[string]any {
	out := make(map[string]map[string]any, len(objs))
	for _, o := range objs {
		out[objectAddress(o)] = o
	}
	return out
}

func objectAddress(o map[string]any) string {
	kind, _ := o["kind"].(string)
	var namespace, name string
	if meta, ok := o["metadata"].(map[string]any); ok {
		namespace, _ = meta["namespace"].(string)
		name, _ = meta["name"].(string)
	}
	return buildAddress(namespace, kind, name)
}
