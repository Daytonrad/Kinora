//go:build windows

package server

import (
	"io/fs"
	"os"
	"strings"
	"testing"
)

// The walk starts at the drive root: f:\, f:\filmek – never \f: (CreateFile syntax error).
func TestPermProblemsWalksFromDriveRoot(t *testing.T) {
	var seen []string
	p := permProbe{
		lstat:   func(p string) (fs.FileInfo, error) { seen = append(seen, p); return fakeInfo{p}, nil },
		readDir: func(string) ([]fs.DirEntry, error) { return nil, nil },
		open:    func(string) (*os.File, error) { return nil, fs.ErrNotExist },
	}
	if _, err := findPermProblems(p, `f:\filmek\Új`); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(seen, "|"); got != `f:\filmek|f:\filmek\Új` {
		t.Fatalf("lstat walk %s", got)
	}
}

func TestNativeRootsAreDrives(t *testing.T) {
	roots := nativeRoots()
	if len(roots) == 0 {
		t.Fatal("no drive found")
	}
	for _, r := range roots {
		if len(r) != 3 || r[1:] != `:\` {
			t.Fatalf("not a drive root: %q", r)
		}
	}
}
