package proxy

import (
	"strings"
	"testing"
)

func TestComposePublishChanged(t *testing.T) {
	if composePublishChanged([]byte("same\n"), []byte("same")) {
		t.Fatal("trim should treat equal")
	}
	if !composePublishChanged([]byte("80:80"), []byte("8080:8080")) {
		t.Fatal("listen change must recreate")
	}
	if !composePublishChanged(nil, []byte("ports:\n")) {
		t.Fatal("new compose file is a change")
	}
}

func TestComposeUpArgsRecreates(t *testing.T) {
	t.Setenv("SIDING_PROXY_IMAGE", "")
	got := strings.Join(composeUpArgs(), " ")
	for _, want := range []string{"up", "-d", "--force-recreate", "--pull", "always"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %s", want, got)
		}
	}
	t.Setenv("SIDING_PROXY_IMAGE", "siding-proxy:local")
	got = strings.Join(composeUpArgs(), " ")
	if !strings.Contains(got, "--pull never") {
		t.Fatalf("local image must not pull: %s", got)
	}
}
