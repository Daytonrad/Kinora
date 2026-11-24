//go:build windows

package media

import (
	"errors"
	"io"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// readShared reads a file ffmpeg keeps replacing (the HLS playlist is written to a .tmp and
// renamed over the old one). os.ReadFile opens without FILE_SHARE_DELETE, so a rename landing
// while the server reads fails on Windows, and ffmpeg stops mid-remux. This open lets it through;
// a sharing violation from ffmpeg's own brief exclusive window is retried.
func readShared(path string) ([]byte, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	var h windows.Handle
	for i := 0; ; i++ {
		h, err = windows.CreateFile(p, windows.GENERIC_READ,
			windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil,
			windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
		if err == nil {
			break
		}
		if i >= 5 || !(errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_ACCESS_DENIED)) {
			if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
				return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrNotExist}
			}
			return nil, &os.PathError{Op: "open", Path: path, Err: err}
		}
		time.Sleep(20 * time.Millisecond)
	}
	f := os.NewFile(uintptr(h), path)
	defer f.Close()
	return io.ReadAll(f)
}
