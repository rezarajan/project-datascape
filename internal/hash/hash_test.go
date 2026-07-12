package hash

import "testing"

func TestCanonicalHashStableForMapOrder(t *testing.T) {
	a, err := Canonical(map[string]any{"b": 2, "a": 1})
	if err != nil {
		t.Fatal(err)
	}
	b, err := Canonical(map[string]any{"a": 1, "b": 2})
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("hashes differ: %s != %s", a, b)
	}
}
