package canonical

import "testing"

func TestJSONBytesSortsObjectKeys(t *testing.T) {
	got, err := JSONBytes([]byte(`{"b":2,"a":{"d":4,"c":3}}`))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"a":{"c":3,"d":4},"b":2}`
	if string(got) != want {
		t.Fatalf("canonical JSON mismatch\nwant %s\ngot  %s", want, got)
	}
}

func TestNormalizeText(t *testing.T) {
	got := string(NormalizeText([]byte("a\r\nb\rc\n")))
	want := "a\nb\nc\n"
	if got != want {
		t.Fatalf("newline normalization mismatch: %q", got)
	}
}

func FuzzJSONBytes(f *testing.F) {
	f.Add([]byte(`{"b":2,"a":1}`))
	f.Add([]byte(`[{"x":1},{"y":2}]`))
	f.Fuzz(func(t *testing.T, input []byte) {
		got, err := JSONBytes(input)
		if err != nil {
			return
		}
		if _, err := JSONBytes(got); err != nil {
			t.Fatalf("canonical output must be valid input: %v", err)
		}
	})
}
