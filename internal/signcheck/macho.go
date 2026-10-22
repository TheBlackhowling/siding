// Package signcheck inspects Mach-O binaries for an Apple Developer ID
// signature. Go's own darwin builds already embed an ad-hoc
// LC_CODE_SIGNATURE; we require a CMS blob (slot 0x10000).
package signcheck

import (
	"bytes"
	"debug/macho"
	"encoding/binary"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	lcCodeSignature macho.LoadCmd = 0x1d
	csMagicEmbedded uint32        = 0xfade0cc0
	csSlotCMS       uint32        = 0x10000
)

// HasCodeSignature reports whether path is a Mach-O binary with a Developer
// ID CMS signature on every slice (if fat). Ad-hoc Go signatures do not count.
func HasCodeSignature(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	if f, err := macho.NewFile(bytes.NewReader(data)); err == nil {
		return thinHasCMS(data, 0, f), nil
	}
	fat, err := macho.NewFatFile(bytes.NewReader(data))
	if err != nil {
		return false, fmt.Errorf("open macho %s: %w", path, err)
	}
	if len(fat.Arches) == 0 {
		return false, fmt.Errorf("fat macho %s has no arches", path)
	}
	for i := range fat.Arches {
		a := fat.Arches[i]
		if a.File == nil || !thinHasCMS(data, int64(a.Offset), a.File) {
			return false, nil
		}
	}
	return true, nil
}

func thinHasCMS(file []byte, base int64, f *macho.File) bool {
	for _, l := range f.Loads {
		raw := l.Raw()
		if loadCmd(raw) != lcCodeSignature || len(raw) < 16 {
			continue
		}
		dataoff := binary.LittleEndian.Uint32(raw[8:12])
		datasize := binary.LittleEndian.Uint32(raw[12:16])
		start := int(base) + int(dataoff)
		end := start + int(datasize)
		if start < 0 || end > len(file) || datasize < 12 {
			return false
		}
		return superBlobHasCMS(file[start:end])
	}
	return false
}

func loadCmd(raw []byte) macho.LoadCmd {
	if len(raw) < 4 {
		return 0
	}
	return macho.LoadCmd(binary.LittleEndian.Uint32(raw[:4]))
}

func superBlobHasCMS(blob []byte) bool {
	if len(blob) < 12 || binary.BigEndian.Uint32(blob[0:4]) != csMagicEmbedded {
		return false
	}
	count := binary.BigEndian.Uint32(blob[8:12])
	off := 12
	for i := uint32(0); i < count; i++ {
		if off+8 > len(blob) {
			return false
		}
		typ := binary.BigEndian.Uint32(blob[off : off+4])
		blobOff := binary.BigEndian.Uint32(blob[off+4 : off+8])
		if typ == csSlotCMS && int(blobOff)+8 <= len(blob) {
			blen := binary.BigEndian.Uint32(blob[int(blobOff)+4 : int(blobOff)+8])
			return blen > 8
		}
		off += 8
	}
	return false
}

// DarwinBinaries walks dist for Mach-O files named siding (release layout).
func DarwinBinaries(dist string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dist, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Base(path) != "siding" {
			return nil
		}
		if _, sigErr := HasCodeSignature(path); sigErr != nil {
			return nil
		}
		out = append(out, path)
		return nil
	})
	return out, err
}

// RequireSigned returns an error if any path is missing a Developer ID CMS signature.
func RequireSigned(paths []string) error {
	if len(paths) == 0 {
		return fmt.Errorf("no darwin binaries to check")
	}
	var unsigned []string
	for _, p := range paths {
		ok, err := HasCodeSignature(p)
		if err != nil {
			return err
		}
		if !ok {
			unsigned = append(unsigned, p)
		}
	}
	if len(unsigned) > 0 {
		return fmt.Errorf("unsigned darwin binaries: %s", strings.Join(unsigned, ", "))
	}
	return nil
}

// DistExists is a small helper for tests and CI.
func DistExists(dist string) bool {
	st, err := os.Stat(dist)
	return err == nil && st.IsDir()
}

// ResolveDist returns dist if it is a directory. A relative path also
// tries repoRoot, because `go test` runs with cwd set to the package dir.
func ResolveDist(dist, repoRoot string) (string, bool) {
	if DistExists(dist) {
		return dist, true
	}
	if repoRoot != "" && !filepath.IsAbs(dist) {
		rooted := filepath.Join(repoRoot, dist)
		if DistExists(rooted) {
			return rooted, true
		}
	}
	return "", false
}
