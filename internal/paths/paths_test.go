package paths

import "testing"

func TestBasename(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{`C:\source\acme-betty`, "acme-betty"},
		{"/source/acme-betty/", "acme-betty"},
		{"hello", "hello"},
	}
	for _, tc := range tests {
		if got := Basename(tc.in); got != tc.want {
			t.Fatalf("Basename(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeLockPath(t *testing.T) {
	a := NormalizeLockPath(`C:\source\acme-betty`)
	b := NormalizeLockPath(`c:/source/acme-betty`)
	if a != b {
		t.Fatalf("expected case-normalized paths to match: %q vs %q", a, b)
	}
}
