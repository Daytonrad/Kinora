//go:build !windows

package server

import (
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
