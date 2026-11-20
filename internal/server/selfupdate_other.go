//go:build !windows

package server

import "errors"

// startDetached: only the Windows install updates itself from the dashboard.
func startDetached(string) error { return errors.New("csak Windowson") }
