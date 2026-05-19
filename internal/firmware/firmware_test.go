package firmware

import "testing"

func TestOpenAndInspectSynthetic(t *testing.T) {
	f, err := Open("testdata/synthetic-header.bin")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	if f.Size() == 0 {
		t.Error("Size() returned 0")
	}
	if got := f.Model(); got != "ZD621" {
		t.Errorf("Model() = %q, want ZD621", got)
	}
	if got := f.Version(); got != "V99.01.02Z" {
		t.Errorf("Version() = %q, want V99.01.02Z", got)
	}
}

func TestOpenNonexistent(t *testing.T) {
	if _, err := Open("testdata/does-not-exist.bin"); err == nil {
		t.Error("expected error opening missing file")
	}
}
