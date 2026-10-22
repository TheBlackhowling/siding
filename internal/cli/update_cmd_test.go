package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/TheBlackhowling/siding/internal/paths"
	"github.com/TheBlackhowling/siding/internal/update"
)

func isolateInstallHome(t *testing.T) {
	t.Helper()
	home := isolateRoot(t)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("SIDING_DATA", home)
	t.Setenv("LOCALAPPDATA", home)
}

func stubReleaseAPI(t *testing.T, srv *httptest.Server) {
	t.Helper()
	oldL, oldT, oldC, oldD := update.LatestURL, update.TagsURLPrefix, update.HTTPClient, update.DownloadClient
	oldAllow := update.AllowAnyAssetHostForTest
	update.LatestURL = srv.URL + "/releases/latest"
	update.TagsURLPrefix = srv.URL + "/releases/tags"
	update.HTTPClient = srv.Client()
	update.DownloadClient = srv.Client()
	update.AllowAnyAssetHostForTest = true
	t.Cleanup(func() {
		update.LatestURL = oldL
		update.TagsURLPrefix = oldT
		update.HTTPClient = oldC
		update.DownloadClient = oldD
		update.AllowAnyAssetHostForTest = oldAllow
	})
}

func writeUpdateArchive(t *testing.T, goos, body string) (name string, data []byte) {
	t.Helper()
	name = update.AssetName("v0.1.4", goos, runtime.GOARCH)
	bin := update.BinaryName(goos)
	var buf bytes.Buffer
	if goos == "windows" {
		zw := zip.NewWriter(&buf)
		w, err := zw.Create(bin)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
	} else {
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		hdr := &tar.Header{Name: bin, Mode: 0755, Size: int64(len(body))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
		if err := tw.Close(); err != nil {
			t.Fatal(err)
		}
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return name, buf.Bytes()
}

func serveUpdateAssets(t *testing.T, checksumLine string, archive []byte, name string, latestOK, tagOK bool) *httptest.Server {
	t.Helper()
	checksums := []byte(checksumLine)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	restoreKey := update.SetChecksumSignPublicKeyForTest(pub)
	t.Cleanup(restoreKey)
	sig := ed25519.Sign(priv, checksums)

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	writeRel := func(w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name": "v0.1.4",
			"assets": []map[string]string{
				{"name": "checksums.txt", "browser_download_url": srv.URL + "/checksums.txt"},
				{"name": "checksums.txt.sig", "browser_download_url": srv.URL + "/checksums.txt.sig"},
				{"name": name, "browser_download_url": srv.URL + "/" + name},
			},
		})
	}
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		if !latestOK {
			t.Error("latest should not be fetched")
			http.NotFound(w, r)
			return
		}
		writeRel(w)
	})
	mux.HandleFunc("/releases/tags/v0.1.4", func(w http.ResponseWriter, r *http.Request) {
		if !tagOK {
			t.Error("tag should not be fetched")
			http.NotFound(w, r)
			return
		}
		writeRel(w)
	})
	mux.HandleFunc("/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(checksums)
	})
	mux.HandleFunc("/checksums.txt.sig", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(sig)
	})
	mux.HandleFunc("/"+name, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	})
	return srv
}

func TestUpdateCommandDownloadsAndInstalls(t *testing.T) {
	isolateInstallHome(t)
	stubVersion(t, "0.1.3")
	body := "#!/bin/sh\necho siding 0.1.4\n"
	name, archive := writeUpdateArchive(t, runtime.GOOS, body)
	sum := sha256.Sum256(archive)
	srv := serveUpdateAssets(t, hex.EncodeToString(sum[:])+"  "+name+"\n", archive, name, true, false)
	stubReleaseAPI(t, srv)

	out, err := execCLI(t, "update")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "updated v0.1.4") {
		t.Fatalf("output:\n%s", out)
	}
	dest, err := installedDest()
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil || string(got) != body {
		t.Fatalf("dest %q %v", got, err)
	}
}

func TestUpdateCommandChecksumLeavesDest(t *testing.T) {
	isolateInstallHome(t)
	stubVersion(t, "0.1.3")
	name, archive := writeUpdateArchive(t, runtime.GOOS, "NEW")
	dest, err := installedDest()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("OLD"), 0o755); err != nil {
		t.Fatal(err)
	}
	srv := serveUpdateAssets(t, "0000000000000000000000000000000000000000000000000000000000000000  "+name+"\n", archive, name, true, false)
	stubReleaseAPI(t, srv)
	if _, err := execCLI(t, "update"); err == nil {
		t.Fatal("checksum should fail")
	}
	got, err := os.ReadFile(dest)
	if err != nil || string(got) != "OLD" {
		t.Fatalf("dest changed: %q %v", got, err)
	}
}

func TestUpdateCommandAlreadyCurrent(t *testing.T) {
	isolateInstallHome(t)
	stubVersion(t, "0.1.4")
	var assets atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "checksums") || strings.HasSuffix(r.URL.Path, ".zip") || strings.HasSuffix(r.URL.Path, ".gz") {
			assets.Add(1)
		}
		if r.URL.Path == "/releases/latest" {
			_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": "v0.1.4"})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	stubReleaseAPI(t, srv)
	out, err := execCLI(t, "update")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "already current: v0.1.4") {
		t.Fatalf("output:\n%s", out)
	}
	if assets.Load() != 0 {
		t.Fatalf("downloaded assets %d", assets.Load())
	}
}

func TestUpdateCommandSpecificVersion(t *testing.T) {
	isolateInstallHome(t)
	stubVersion(t, "0.1.3")
	body := "tagged"
	name, archive := writeUpdateArchive(t, runtime.GOOS, body)
	sum := sha256.Sum256(archive)
	srv := serveUpdateAssets(t, hex.EncodeToString(sum[:])+"  "+name+"\n", archive, name, false, true)
	stubReleaseAPI(t, srv)
	out, err := execCLI(t, "update", "v0.1.4")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "updated v0.1.4") {
		t.Fatalf("output:\n%s", out)
	}
}

func TestUpdateCommandOfflineAndBadTag(t *testing.T) {
	isolateInstallHome(t)
	stubVersion(t, "0.1.3")
	oldL, oldC := update.LatestURL, update.HTTPClient
	update.LatestURL = "http://127.0.0.1:1"
	update.HTTPClient = &http.Client{}
	t.Cleanup(func() {
		update.LatestURL = oldL
		update.HTTPClient = oldC
	})
	if _, err := execCLI(t, "update"); err == nil {
		t.Fatal("offline")
	}
	if _, err := execCLI(t, "update", "nope"); err == nil {
		t.Fatal("bad tag")
	}
}

func TestUpdateCommandUsesCommandBinDir(t *testing.T) {
	isolateInstallHome(t)
	dir, err := paths.CommandBinDir()
	if err != nil {
		t.Fatal(err)
	}
	dest, err := installedDest()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(dest) != dir {
		t.Fatalf("dest %s dir %s", dest, dir)
	}
}
