//go:build !windows

package media

import "os"

// readShared reads a file ffmpeg keeps replacing; on Unix a rename over an open file just works.
func readShared(path string) ([]byte, error) { return os.ReadFile(path) }
