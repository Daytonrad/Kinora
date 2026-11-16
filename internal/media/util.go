package media

import (
	"os"
	"os/exec"
	"syscall"
)

func statSize(path string) (int64, error) {
	st, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return st.Size(), nil
}

// gracefulStop asks ffmpeg to quit (SIGTERM, Interrupt on Windows) instead of SIGKILL.
func gracefulStop(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		cmd.Process.Kill()
	}
}
