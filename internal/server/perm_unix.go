//go:build !windows

package server

import (
	"errors"
	"os"
	"os/user"
	"strconv"
	"syscall"
)

// ownerGroup returns the group name owning path (for the "add to group" alternative).
func ownerGroup(path string) string {
	fi, err := os.Stat(path)
	if err != nil {
		return ""
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}
	if g, err := user.LookupGroupId(strconv.Itoa(int(st.Gid))); err == nil {
		return g.Name
	}
	return ""
}

// nativeRoots are the picker shortcuts of a native install.
func nativeRoots() []string { return []string{"/", "/home", "/mnt", "/media", "/srv"} }

// browseRoot is where the picker opens without a path and a media root: the whole file system.
func browseRoot() string { return "/" }

// canGrantRead: on Unix the admin runs the setfacl commands; the server has no right to change ACLs.
const canGrantRead = false

func grantRead(string, []PermProblem) error {
	return errors.New("a jogok automatikus megadása csak Windowson működik")
}
