package update

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func serveRelease(t *testing.T, tag, goos, goarch, bin string) (*httptest.Server, []byte) {
	t.Helper()
	dir := t.TempDir()
	name := AssetName(tag, goos, goarch)
	archive := filepath.Join(dir, name)
	if goos == "windows" {
		writeZip(t, archive, BinaryName(goos), bin)
	} else {
		writeTarGz(t, archive, BinaryName(goos), bin)
	}
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	checksums := hex.EncodeToString(sum[:]) + "  " + name + "\n"
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	restoreKey := SetChecksumSignPublicKeyForTest(pub)
	t.Cleanup(restoreKey)
	sig := ed25519.Sign(priv, []byte(checksums))

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	releaseJSON := func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(githubRelease{
			TagName: tag,
			Assets: []githubAsset{
				{Name: "checksums.txt", BrowserDownloadURL: srv.URL + "/checksums.txt"},
				{Name: checksumSigAsset, BrowserDownloadURL: srv.URL + "/checksums.txt.sig"},
				{Name: name, BrowserDownloadURL: srv.URL + "/" + name},
			},
		})
	}
	mux.HandleFunc("/releases/latest", releaseJSON)
	mux.HandleFunc("/releases/tags/"+Display(tag), releaseJSON)
	mux.HandleFunc("/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(checksums))
	})
	mux.HandleFunc("/checksums.txt.sig", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(sig)
	})
	mux.HandleFunc("/"+name, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(data)
	})
	return srv, data
}

func TestDownloadBinaryHappyAndTagged(t *testing.T) {
	srv, _ := serveRelease(t, "v0.1.4", "linux", "amd64", "new-bin")
	opt := Options{
		Client:            srv.Client(),
		LatestURL:         srv.URL + "/releases/latest",
		TagURLPrefix:      srv.URL + "/releases/tags",
		GOOS:              "linux",
		GOARCH:            "amd64",
		WorkDir:           t.TempDir(),
		AllowAnyAssetHost: true,
	}
	rel, err := FetchRelease(context.Background(), opt)
	if err != nil || rel.Tag != "v0.1.4" {
		t.Fatalf("%+v %v", rel, err)
	}
	got, err := DownloadBinary(context.Background(), opt, rel)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(got)
	if err != nil || string(body) != "new-bin" {
		t.Fatalf("%s %v", body, err)
	}

	opt.Version = "v0.1.4"
	rel, err = FetchRelease(context.Background(), opt)
	if err != nil || rel.Tag != "v0.1.4" {
		t.Fatalf("tag: %+v %v", rel, err)
	}
}

func TestDownloadBinaryChecksumFail(t *testing.T) {
	dir := t.TempDir()
	writeTarGz(t, filepath.Join(dir, "siding_0.1.4_linux_amd64.tar.gz"), "siding", "payload")
	data, _ := os.ReadFile(filepath.Join(dir, "siding_0.1.4_linux_amd64.tar.gz"))
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	name := "siding_0.1.4_linux_amd64.tar.gz"
	checksums := "0000000000000000000000000000000000000000000000000000000000000000  " + name + "\n"
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	restoreKey := SetChecksumSignPublicKeyForTest(pub)
	t.Cleanup(restoreKey)
	sig := ed25519.Sign(priv, []byte(checksums))
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(githubRelease{
			TagName: "v0.1.4",
			Assets: []githubAsset{
				{Name: "checksums.txt", BrowserDownloadURL: srv.URL + "/checksums.txt"},
				{Name: checksumSigAsset, BrowserDownloadURL: srv.URL + "/checksums.txt.sig"},
				{Name: name, BrowserDownloadURL: srv.URL + "/" + name},
			},
		})
	})
	mux.HandleFunc("/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(checksums))
	})
	mux.HandleFunc("/checksums.txt.sig", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(sig)
	})
	mux.HandleFunc("/"+name, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(data)
	})
	opt := Options{
		Client:            srv.Client(),
		LatestURL:         srv.URL + "/releases/latest",
		GOOS:              "linux",
		GOARCH:            "amd64",
		WorkDir:           t.TempDir(),
		AllowAnyAssetHost: true,
	}
	rel, err := FetchRelease(context.Background(), opt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DownloadBinary(context.Background(), opt, rel); err == nil {
		t.Fatal("checksum")
	}
}

func TestFetchReleaseOffline(t *testing.T) {
	_, err := FetchRelease(context.Background(), Options{
		LatestURL: "http://127.0.0.1:1",
		Client:    &http.Client{},
	})
	if err == nil {
		t.Fatal("offline")
	}
}
