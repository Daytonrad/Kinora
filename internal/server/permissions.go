package server

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Native Linux installs can add any folder the service user can read, at any time.
// When it cannot, the admin gets the exact reason and the command that fixes it
// instead of an empty library.

// installMode is how this server was installed: docker | systemd | windows | manual.
func installMode() string {
	if m := os.Getenv("KINORA_INSTALL"); m != "" {
		return m
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return "docker"
	}
	if runtime.GOOS == "windows" {
		return "windows"
	}
	return "manual"
}

// PermProblem is one missing permission on the way to a folder.
type PermProblem struct {
	Path string `json:"path"`
	Need string `json:"need"` // "x": may not enter (a parent folder), "rX": may not read (recursive)
}

// PermReport is the answer of the permission check.
type PermReport struct {
	OK       bool          `json:"ok"`
	Path     string        `json:"path"`
	User     string        `json:"user"`
	Mode     string        `json:"mode"`
	Message  string        `json:"message"`
	Problems []PermProblem `json:"problems"`
	Commands []string      `json:"commands"`
	Group    []string      `json:"group_commands,omitempty"` // alternative: add the user to the owning group
	Hint     string        `json:"hint,omitempty"`
}

// permProbe abstracts the file system for tests.
type permProbe struct {
	lstat   func(string) (fs.FileInfo, error)
	readDir func(string) ([]fs.DirEntry, error)
	open    func(string) (*os.File, error)
}

var osProbe = permProbe{lstat: os.Lstat, readDir: os.ReadDir, open: os.Open}

func isPerm(err error) bool { return errors.Is(err, fs.ErrPermission) }

// findPermProblems walks from / to path: a parent that cannot be entered needs x,
// the folder itself (and a sample of its content) needs r and x.
func findPermProblems(p permProbe, path string) ([]PermProblem, error) {
	path = filepath.Clean(path)
	var probs []PermProblem
	// Windows: start at the volume root (f:\), not at \ – filepath.Join(`\`, "f:") is `\f:`.
	vol := filepath.VolumeName(path)
	parts := strings.Split(strings.TrimPrefix(path[len(vol):], string(filepath.Separator)), string(filepath.Separator))
	cur := vol + string(filepath.Separator)
	for _, part := range parts {
		if part == "" {
			continue
		}
		next := filepath.Join(cur, part)
		if _, err := p.lstat(next); err != nil {
			if isPerm(err) {
				// deeper levels cannot be checked from here; the target will need rX too
				probs = append(probs, PermProblem{Path: cur, Need: "x"}, PermProblem{Path: path, Need: "rX"})
				return probs, nil
			}
			return nil, err // does not exist
		}
		cur = next
	}
	entries, err := p.readDir(path)
	if err != nil {
		if isPerm(err) {
			return append(probs, PermProblem{Path: path, Need: "rX"}), nil
		}
		return nil, err
	}
	// sample the content: an unreadable sub-folder or file breaks the scan as well
	checked := 0
	var walk func(dir string, ents []fs.DirEntry, depth int) bool
	walk = func(dir string, ents []fs.DirEntry, depth int) bool {
		for _, e := range ents {
			if checked >= 300 {
				return true
			}
			checked++
			full := filepath.Join(dir, e.Name())
			if strings.HasPrefix(e.Name(), ".") {
				continue
			}
			if e.IsDir() {
				sub, err := p.readDir(full)
				if err != nil {
					if isPerm(err) {
						probs = append(probs, PermProblem{Path: path, Need: "rX"})
						return false
					}
					continue
				}
				if depth < 3 && !walk(full, sub, depth+1) {
					return false
				}
				continue
			}
			if f, err := p.open(full); err != nil {
				if isPerm(err) {
					probs = append(probs, PermProblem{Path: path, Need: "rX"})
					return false
				}
			} else {
				f.Close()
			}
		}
		return true
	}
	walk(path, entries, 0)
	return probs, nil
}

func serviceUser() (name string, uid string) {
	if u, err := user.Current(); err == nil {
		return u.Username, u.Uid
	}
	return strconv.Itoa(os.Getuid()), strconv.Itoa(os.Getuid())
}

// hostPath maps a container path under /host back to the host (docker-compose.host.yml).
func hostPath(mode, p string) string {
	if mode == "docker" && (p == "/host" || strings.HasPrefix(p, "/host/")) {
		if h := strings.TrimPrefix(p, "/host"); h != "" {
			return h
		}
		return "/"
	}
	return p
}

// winQuote quotes a path or account for cmd.exe.
func winQuote(p string) string { return `"` + p + `"` }

// windowsPermCommands are the icacls fixes: RX on the folder, inherited by sub-folders and files,
// also the existing ones (/T); a parent that cannot be listed gets RX on itself only.
func windowsPermCommands(userName string, probs []PermProblem) []string {
	var cmds []string
	seen := map[string]bool{}
	for _, pr := range probs {
		c := fmt.Sprintf("icacls %s /grant %s /T", winQuote(pr.Path), winQuote(userName+":(OI)(CI)RX"))
		if pr.Need == "x" {
			c = fmt.Sprintf("icacls %s /grant %s", winQuote(pr.Path), winQuote(userName+":RX"))
		}
		if !seen[c] {
			seen[c] = true
			cmds = append(cmds, c)
		}
	}
	return cmds
}

func shellQuote(p string) string {
	if strings.ContainsAny(p, " '\"$`\\!&;()|<>*?[]{}#~") {
		return "'" + strings.ReplaceAll(p, "'", `'\''`) + "'"
	}
	return p
}

// buildPermReport turns problems into a Hungarian message and fix commands.
func buildPermReport(path, mode, userName, uid string, probs []PermProblem, groupOf func(string) string) PermReport {
	rep := PermReport{OK: len(probs) == 0, Path: path, User: userName, Mode: mode, Problems: probs, Commands: []string{}}
	if rep.OK {
		rep.Message = fmt.Sprintf("Rendben: a(z) %s felhasználó olvashatja ezt a mappát.", userName)
		return rep
	}
	if mode == "windows" {
		rep.Message = fmt.Sprintf("A(z) %s felhasználónak nincs olvasási joga ehhez: %s", userName, path)
		rep.Commands = windowsPermCommands(userName, probs)
		rep.Hint = "A parancsokat rendszergazdaként futtatott parancssorban futtasd, utána szkenneld újra a könyvtárat. A szolgáltatás nem látja a hozzárendelt hálózati meghajtókat (pl. Z:) – hálózati megosztásnál az UNC útvonalat add meg (\\\\szerver\\megosztás)."
		return rep
	}
	who := userName
	if mode == "docker" {
		who = uid // the container user exists on the host only by its id (PUID)
	}
	target := hostPath(mode, path)
	rep.Message = fmt.Sprintf("A(z) %s felhasználónak nincs olvasási joga ehhez: %s", userName, target)
	seen := map[string]bool{}
	for _, pr := range probs {
		hp := shellQuote(hostPath(mode, pr.Path))
		var cmds []string
		if pr.Need == "x" {
			cmds = []string{fmt.Sprintf("sudo setfacl -m u:%s:x %s", who, hp)}
		} else {
			cmds = []string{
				fmt.Sprintf("sudo setfacl -R -m u:%s:rX %s", who, hp),
				fmt.Sprintf("sudo setfacl -R -d -m u:%s:rX %s", who, hp), // new files later on, too
			}
		}
		for _, c := range cmds {
			if !seen[c] {
				seen[c] = true
				rep.Commands = append(rep.Commands, c)
			}
		}
	}
	if groupOf != nil {
		if g := groupOf(path); g != "" && g != "root" && mode != "docker" {
			restart := "sudo systemctl restart kinora"
			rep.Group = []string{fmt.Sprintf("sudo usermod -aG %s %s", g, userName), fmt.Sprintf("sudo chmod -R g+rX %s", shellQuote(target)), restart}
		}
	}
	switch mode {
	case "docker":
		rep.Hint = fmt.Sprintf("A konténer a %s azonosítójú felhasználóval fut (PUID). A parancsokat a hoston futtasd; a setfacl az „acl” csomagban van.", uid)
	default:
		rep.Hint = "A setfacl az „acl” csomagban van (Debian/Ubuntu: sudo apt install acl). Hálózati megosztásnál (SMB/NFS) a jogokat a csatolási beállítások adják (uid=, gid=, file_mode=)."
	}
	return rep
}

func (s *Server) permReport(path string) (PermReport, error) {
	if !filepath.IsAbs(path) {
		return PermReport{}, fmt.Errorf("adj meg egy abszolút mappaútvonalat")
	}
	path = filepath.Clean(path)
	probs, err := findPermProblems(osProbe, path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return PermReport{}, fmt.Errorf("a mappa nem létezik: %s", path)
		}
		return PermReport{}, err
	}
	name, uid := serviceUser()
	return buildPermReport(path, installMode(), name, uid, probs, ownerGroup), nil
}

// GET /api/admin/permcheck?path=… – "Jogosultság ellenőrzése".
func (s *Server) handlePermCheck(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	path := r.URL.Query().Get("path")
	if id := queryInt(r, "library", 0); id > 0 {
		if lib, err := s.st.Library(int64(id)); err == nil {
			path = lib.Path
		}
	}
	rep, err := s.permReport(path)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// browseShortcuts are the quick-start folders of the admin folder picker.
func browseShortcuts(mode, mediaRoot string) []string {
	var cands []string
	if mode == "docker" {
		cands = []string{"/media", "/host", "/host/home", "/host/mnt", "/host/media", "/host/srv"}
	} else {
		cands = nativeRoots()
		if h, err := os.UserHomeDir(); err == nil {
			cands = append(cands, h)
		}
	}
	if mediaRoot != "" {
		cands = append([]string{mediaRoot}, cands...)
	}
	var out []string
	seen := map[string]bool{}
	for _, c := range cands {
		if !seen[c] && isDir(c) {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}
