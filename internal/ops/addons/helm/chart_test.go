package helm

import "testing"

func TestReadChartMeta(t *testing.T) {
	meta, err := readChartMeta("testdata/chart")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Name != "demo" || meta.Version != "0.1.0" || meta.APIVersion != "v2" {
		t.Errorf("readChartMeta = %+v, want name demo, version 0.1.0, apiVersion v2", meta)
	}
}

func TestReadChartMetaMissingFile(t *testing.T) {
	if _, err := readChartMeta(t.TempDir()); err == nil {
		t.Fatal("readChartMeta returned no error for a directory with no Chart.yaml")
	}
}

func TestFindValuesFile(t *testing.T) {
	if _, ok := findValuesFile("testdata/chart"); !ok {
		t.Error("findValuesFile(testdata/chart) = false, want true")
	}
	if _, ok := findValuesFile(t.TempDir()); ok {
		t.Error("findValuesFile(empty dir) = true, want false — a missing values.yaml must not disqualify a chart")
	}
}
