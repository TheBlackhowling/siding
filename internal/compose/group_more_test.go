package compose

import "testing"

func TestSanitizeAndUniqueModeName(t *testing.T) {
	if SanitizeModeName("Prod DB!") != "prod-db" {
		t.Fatalf("%q", SanitizeModeName("Prod DB!"))
	}
	if SanitizeModeName("   ") != "test" {
		t.Fatal("empty")
	}
	used := map[string]struct{}{}
	if uniqueModeName("test", used) != "test" {
		t.Fatal("first")
	}
	if uniqueModeName("test", used) != "test-2" {
		t.Fatal("second")
	}
	if uniqueModeName("", map[string]struct{}{}) != "test" {
		t.Fatal("blank")
	}
}
