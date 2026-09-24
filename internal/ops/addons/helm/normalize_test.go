package helm

import (
	"reflect"
	"sort"
	"testing"
)

func TestDecodeJSONStreamListWrapped(t *testing.T) {
	got, err := decodeJSONStream([]byte(`{"kind":"List","items":[{"kind":"Deployment","metadata":{"name":"a"}},{"kind":"Service","metadata":{"name":"b"}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("decodeJSONStream = %v, want two flattened items", got)
	}
}

func TestStripVolatileRemovesServerSetFieldsOnly(t *testing.T) {
	obj := map[string]any{
		"status":   map[string]any{"phase": "Running"},
		"metadata": map[string]any{"name": "web", "resourceVersion": "123"},
		"spec":     map[string]any{"replicas": float64(2)},
	}
	got := stripVolatile(obj)
	if _, ok := got["status"]; ok {
		t.Error("stripVolatile kept status")
	}
	meta := got["metadata"].(map[string]any)
	if _, ok := meta["resourceVersion"]; ok {
		t.Error("stripVolatile kept metadata.resourceVersion")
	}
	if meta["name"] != "web" {
		t.Error("stripVolatile dropped metadata.name")
	}
}

func TestTopLevelDiffTreatsMissingKeyAsChanged(t *testing.T) {
	before := map[string]any{"spec": "x"}
	after := map[string]any{"spec": "x", "data": "y"}
	got := topLevelDiff(before, after)
	sort.Strings(got)
	want := []string{"data"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("topLevelDiff = %v, want %v", got, want)
	}
}
