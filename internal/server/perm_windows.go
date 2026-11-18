//go:build windows

package server

import "os"

// ownerGroup has no Unix group on Windows.
func ownerGroup(string) string { return "" }

// nativeRoots are the drives (C:\, D:\, …) that exist, so the folder picker can switch between them.
// A: and B: are skipped: probing an empty floppy drive can stall.
func nativeRoots() []string {
	var out []string
	for d := 'C'; d <= 'Z'; d++ {
		if root := string(d) + `:\`; isDir(root) {
			out = append(out, root)
		}
	}
	return out
}

// browseRoot is where the picker opens without a path and a media root: the system drive.
func browseRoot() string {
	if d := os.Getenv("SystemDrive"); d != "" {
		return d + `\`
	}
	return `C:\`
}
