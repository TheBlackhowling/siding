package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func writeTarGz(t *testing.T, dest, name, body string) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{Name: name, Mode: 0755, Size: int64(len(body))}
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
	if err := os.WriteFile(dest, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeZip(t *testing.T, dest, name, body string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestExtractTarAndZip(t *testing.T) {
	dir := t.TempDir()
	tg := filepath.Join(dir, "siding_0.1.4_linux_amd64.tar.gz")
	writeTarGz(t, tg, "siding", "linux-bin")
	got, err := ExtractBinary(tg, filepath.Join(dir, "out-linux"), "linux")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(got)
	if err != nil || string(data) != "linux-bin" {
		t.Fatalf("%s %v", data, err)
	}

	zp := filepath.Join(dir, "siding_0.1.4_windows_amd64.zip")
	writeZip(t, zp, "siding.exe", "win-bin")
	got, err = ExtractBinary(zp, filepath.Join(dir, "out-win"), "windows")
	if err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(got)
	if err != nil || string(data) != "win-bin" {
		t.Fatalf("%s %v", data, err)
	}
}

func TestExtractRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	zp := filepath.Join(dir, "bad.zip")
	writeZip(t, zp, "../siding.exe", "nope")
	if _, err := ExtractBinary(zp, filepath.Join(dir, "out"), "windows"); err == nil {
		t.Fatal("zip slip")
	}
}
