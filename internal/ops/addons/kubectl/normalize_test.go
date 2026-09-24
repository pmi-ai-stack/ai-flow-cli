package kubectl

import (
	"reflect"
	"sort"
	"testing"
)

func TestDecodeJSONStreamSingleObject(t *testing.T) {
	got, err := decodeJSONStream([]byte(`{"kind":"Deployment","metadata":{"name":"web"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0]["kind"] != "Deployment" {
		t.Fatalf("decodeJSONStream = %v, want one Deployment object", got)
	}
}

func TestDecodeJSONStreamListWrapped(t *testing.T) {
	got, err := decodeJSONStream([]byte(`{"kind":"List","items":[{"kind":"Deployment","metadata":{"name":"a"}},{"kind":"Service","metadata":{"name":"b"}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("decodeJSONStream = %v, want two flattened items", got)
	}
}

func TestDecodeJSONStreamConcatenatedObjects(t *testing.T) {
	got, err := decodeJSONStream([]byte(`{"kind":"Deployment","metadata":{"name":"a"}}{"kind":"Service","metadata":{"name":"b"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("decodeJSONStream = %v, want two concatenated objects", got)
	}
}

func TestDecodeJSONStreamEmpty(t *testing.T) {
	got, err := decodeJSONStream([]byte(``))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("decodeJSONStream(\"\") = %v, want none", got)
	}
}

func TestStripVolatileRemovesServerSetFieldsOnly(t *testing.T) {
	obj := map[string]any{
		"status": map[string]any{"phase": "Running"},
		"metadata": map[string]any{
			"name":              "web",
			"resourceVersion":   "123",
			"uid":               "abc",
			"generation":        float64(2),
			"creationTimestamp": "2024-01-01T00:00:00Z",
			"managedFields":     []any{"x"},
			"annotations": map[string]any{
				"kubectl.kubernetes.io/last-applied-configuration": "{}",
				"team": "platform",
			},
		},
		"spec": map[string]any{"replicas": float64(2)},
	}

	got := stripVolatile(obj)
	if _, ok := got["status"]; ok {
		t.Error("stripVolatile kept status")
	}
	meta := got["metadata"].(map[string]any)
	for _, k := range []string{"resourceVersion", "uid", "generation", "creationTimestamp", "managedFields"} {
		if _, ok := meta[k]; ok {
			t.Errorf("stripVolatile kept metadata.%s", k)
		}
	}
	if meta["name"] != "web" {
		t.Error("stripVolatile dropped metadata.name, which it must keep")
	}
	ann := meta["annotations"].(map[string]any)
	if _, ok := ann["kubectl.kubernetes.io/last-applied-configuration"]; ok {
		t.Error("stripVolatile kept the last-applied-configuration annotation")
	}
	if ann["team"] != "platform" {
		t.Error("stripVolatile dropped a real annotation it must keep")
	}
	if got["spec"] == nil {
		t.Error("stripVolatile dropped spec, which it must keep")
	}
}

func TestTopLevelDiffNamesOnlyChangedKeys(t *testing.T) {
	before := map[string]any{"spec": map[string]any{"replicas": float64(2)}, "metadata": map[string]any{"name": "web"}}
	after := map[string]any{"spec": map[string]any{"replicas": float64(3)}, "metadata": map[string]any{"name": "web"}}

	got := topLevelDiff(before, after)
	if len(got) != 1 || got[0] != "spec" {
		t.Fatalf("topLevelDiff = %v, want [spec]", got)
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

func TestTopLevelDiffEmptyWhenIdentical(t *testing.T) {
	obj := map[string]any{"spec": map[string]any{"replicas": float64(2)}}
	if got := topLevelDiff(obj, obj); len(got) != 0 {
		t.Fatalf("topLevelDiff(x, x) = %v, want none", got)
	}
}
