package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/TheBlackhowling/siding/internal/update"
	"github.com/TheBlackhowling/siding/internal/version"
)

func enableUpdateCheck(t *testing.T) {
	t.Helper()
	t.Setenv("SIDING_NO_UPDATE_CHECK", "")
}

func stubVersion(t *testing.T, v string) {
	t.Helper()
	old := version.Version
	version.Version = v
	t.Cleanup(func() { version.Version = old })
}

func stubLatestRelease(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	oldURL, oldClient := update.LatestURL, update.HTTPClient
	update.LatestURL = srv.URL
	update.HTTPClient = srv.Client()
	t.Cleanup(func() {
		update.LatestURL = oldURL
		update.HTTPClient = oldClient
	})
}

func TestUpdateNoticeAfterSuccessfulCommand(t *testing.T) {
	isolateRoot(t)
	enableUpdateCheck(t)
	stubVersion(t, "0.1.3")
	var hits atomic.Int32
	stubLatestRelease(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": "v0.1.4"})
	})

	out, err := execCLI(t, "version")
	if err != nil {
		t.Fatal(err)
	}
	want := "update available: v0.1.4 (this binary is v0.1.3). Run: siding update"
	if !strings.Contains(out, want) {
		t.Fatalf("missing notice:\n%s", out)
	}
	if hits.Load() != 1 {
		t.Fatalf("hits %d", hits.Load())
	}

	out, err = execCLI(t, "version")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, want) {
		t.Fatalf("cached notice:\n%s", out)
	}
	if hits.Load() != 1 {
		t.Fatalf("cache should skip HTTP, hits %d", hits.Load())
	}
}

func TestUpdateNoticeSilentWhenCurrent(t *testing.T) {
	isolateRoot(t)
	enableUpdateCheck(t)
	stubVersion(t, "0.1.3")
	stubLatestRelease(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": "v0.1.3"})
	})
	out, err := execCLI(t, "version")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "update available") {
		t.Fatalf("same tag must be silent:\n%s", out)
	}
}

func TestUpdateNoticeHTTPErrorSilent(t *testing.T) {
	isolateRoot(t)
	enableUpdateCheck(t)
	stubVersion(t, "0.1.3")
	stubLatestRelease(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	})
	out, err := execCLI(t, "version")
	if err != nil {
		t.Fatalf("command must still succeed: %v\n%s", err, out)
	}
	if strings.Contains(out, "update available") {
		t.Fatalf("404 must be silent:\n%s", out)
	}
}

func TestUpdateNoticeSkippedForDevAndEnv(t *testing.T) {
	t.Run("dev", func(t *testing.T) {
		isolateRoot(t)
		enableUpdateCheck(t)
		stubLatestRelease(t, func(http.ResponseWriter, *http.Request) {
			t.Fatal("dev must not fetch")
		})
		if _, err := execCLI(t, "version"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("env", func(t *testing.T) {
		isolateRoot(t)
		stubVersion(t, "0.1.3")
		stubLatestRelease(t, func(http.ResponseWriter, *http.Request) {
			t.Fatal("SIDING_NO_UPDATE_CHECK must not fetch")
		})
		if _, err := execCLI(t, "version"); err != nil {
			t.Fatal(err)
		}
	})
}

func TestUpdateNoticeSkippedForJSONAndFailedCommand(t *testing.T) {
	root := isolateRoot(t)
	enableUpdateCheck(t)
	stubVersion(t, "0.1.3")
	var hits atomic.Int32
	stubLatestRelease(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": "v0.1.4"})
	})
	hello := helloRepo(t)

	out, err := execCLI(t, "--root", root, "plan", "--path", hello, "--json")
	if err != nil {
		t.Fatalf("plan json: %v\n%s", err, out)
	}
	if strings.Contains(out, "update available") {
		t.Fatalf("json must not mix in a notice:\n%s", out)
	}
	if hits.Load() != 0 {
		t.Fatalf("json hit GitHub %d times", hits.Load())
	}

	_, err = execCLI(t, "--root", root, "doctor", "--path", hello)
	if err == nil {
		t.Fatal("doctor without docker should fail")
	}
	if hits.Load() != 0 {
		t.Fatalf("failed command hit GitHub %d times", hits.Load())
	}
}

func TestUpdateNoticeSkippedForHelp(t *testing.T) {
	isolateRoot(t)
	enableUpdateCheck(t)
	stubVersion(t, "0.1.3")
	stubLatestRelease(t, func(http.ResponseWriter, *http.Request) {
		t.Fatal("help must not fetch")
	})
	if _, err := execCLI(t, "help"); err != nil {
		t.Fatal(err)
	}
}
