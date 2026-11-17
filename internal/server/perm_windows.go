//go:build windows

package server

// ownerGroup has no Unix group on Windows.
func ownerGroup(string) string { return "" }
