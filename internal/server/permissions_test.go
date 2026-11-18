package server

import (
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"
)

type fakeInfo struct{ name string }

func (f fakeInfo) Name() string       { return f.name }
func (f fakeInfo) Size() int64        { return 0 }
func (f fakeInfo) Mode() fs.FileMode  { return fs.ModeDir | 0o755 }
func (f fakeInfo) ModTime() time.Time { return time.Time{} }
func (f fakeInfo) IsDir() bool        { return true }
func (f fakeInfo) Sys() any           { return nil }

// fakeFS: "noenter" folders cannot be traversed (no x), "noread" cannot be listed.
func fakeFS(noenter, noread map[string]bool) permProbe {
	parentOf := func(p string) string {
		i := strings.LastIndex(p, "/")
		if i <= 0 {
			return "/"
		}
		return p[:i]
	}
	return permProbe{
		lstat: func(p string) (fs.FileInfo, error) {
			if noenter[parentOf(p)] {
				return nil, &fs.PathError{Op: "lstat", Path: p, Err: fs.ErrPermission}
			}
			return fakeInfo{p}, nil
		},
		readDir: func(p string) ([]fs.DirEntry, error) {
			if noread[p] || noenter[p] {
				return nil, &fs.PathError{Op: "open", Path: p, Err: fs.ErrPermission}
			}
			return nil, nil
		},
		open: func(p string) (*os.File, error) { return nil, fs.ErrNotExist },
	}
}

func TestPermProblemsParentWithoutExecute(t *testing.T) {
	probs, err := findPermProblems(fakeFS(map[string]bool{"/home/mate": true}, nil), "/home/mate/Filmek")
	if err != nil {
		t.Fatal(err)
	}
	if len(probs) != 2 || probs[0] != (PermProblem{"/home/mate", "x"}) || probs[1] != (PermProblem{"/home/mate/Filmek", "rX"}) {
		t.Fatalf("problems %+v", probs)
	}
	rep := buildPermReport("/home/mate/Filmek", "systemd", "kinora", "998", probs, func(string) string { return "mate" })
	if rep.OK || rep.Message != "A(z) kinora felhasználónak nincs olvasási joga ehhez: /home/mate/Filmek" {
		t.Fatalf("message %q", rep.Message)
	}
	want := []string{
		"sudo setfacl -m u:kinora:x /home/mate",
		"sudo setfacl -R -m u:kinora:rX /home/mate/Filmek",
		"sudo setfacl -R -d -m u:kinora:rX /home/mate/Filmek",
	}
	if strings.Join(rep.Commands, "\n") != strings.Join(want, "\n") {
		t.Fatalf("commands:\n%s", strings.Join(rep.Commands, "\n"))
	}
	if len(rep.Group) == 0 || rep.Group[0] != "sudo usermod -aG mate kinora" {
		t.Fatalf("group alternative %v", rep.Group)
	}
}

func TestPermProblemsUnreadableFolder(t *testing.T) {
	probs, _ := findPermProblems(fakeFS(nil, map[string]bool{"/srv/media/Sorozatok": true}), "/srv/media/Sorozatok")
	if len(probs) != 1 || probs[0].Need != "rX" {
		t.Fatalf("problems %+v", probs)
	}
	ok, _ := findPermProblems(fakeFS(nil, nil), "/srv/media/Filmek")
	if rep := buildPermReport("/srv/media/Filmek", "systemd", "kinora", "998", ok, nil); !rep.OK || len(rep.Commands) != 0 {
		t.Fatalf("readable folder: %+v", rep)
	}
}

func TestPermReportDockerUsesHostPathAndUID(t *testing.T) {
	probs := []PermProblem{{"/host/home/mate", "x"}, {"/host/home/mate/Filmek és sorozatok", "rX"}}
	rep := buildPermReport("/host/home/mate/Filmek és sorozatok", "docker", "kinora", "1000", probs, nil)
	if rep.Commands[0] != "sudo setfacl -m u:1000:x /home/mate" ||
		rep.Commands[1] != "sudo setfacl -R -m u:1000:rX '/home/mate/Filmek és sorozatok'" {
		t.Fatalf("docker commands %v", rep.Commands)
	}
	if !strings.Contains(rep.Message, "/home/mate/Filmek és sorozatok") || len(rep.Group) != 0 {
		t.Fatalf("docker report %+v", rep)
	}
}

func TestBrowseShowsShortcutsAndPermcheck(t *testing.T) {
	c, _ := newTestServer(t)
	var b struct {
		Path      string   `json:"path"`
		Shortcuts []string `json:"shortcuts"`
		Mode      string   `json:"mode"`
	}
	if code := c.do("GET", "/api/admin/browse?path=/", nil, &b); code != 200 || len(b.Shortcuts) == 0 || b.Mode == "" {
		t.Fatalf("browse %d %+v", code, b)
	}
	dir := t.TempDir()
	var rep PermReport
	if code := c.do("GET", "/api/admin/permcheck?path="+dir, nil, &rep); code != 200 || !rep.OK {
		t.Fatalf("permcheck %d %+v", code, rep)
	}
	if code := c.do("GET", "/api/admin/permcheck?path=/nincs/ilyen/mappa", nil, nil); code != 400 {
		t.Fatalf("missing folder %d", code)
	}
}

func TestPermReportWindowsUsesIcacls(t *testing.T) {
	probs := []PermProblem{{`f:\`, "x"}, {`f:\filmek`, "rX"}, {`f:\filmek`, "rX"}}
	rep := buildPermReport(`f:\filmek`, "windows", `NT AUTHORITY\SYSTEM`, "S-1-5-18", probs, nil)
	want := []string{`icacls "f:\" /grant "*S-1-5-18:RX"`, `icacls "f:\filmek" /grant "*S-1-5-18:(OI)(CI)RX" /T`}
	if strings.Join(rep.Commands, "\n") != strings.Join(want, "\n") {
		t.Fatalf("windows commands %q", rep.Commands)
	}
	if rep.OK || !strings.Contains(rep.Message, `f:\filmek`) || len(rep.Group) != 0 || !strings.Contains(rep.Hint, "UNC") {
		t.Fatalf("windows report %+v", rep)
	}
}

func TestPermReportWindowsShares(t *testing.T) {
	hostName = func() (string, error) { return "WIN-IN3GO8AFV53", nil }
	defer func() { hostName = os.Hostname }()
	probs := []PermProblem{{`\\WIN-IN3GO8AFV53\filmek`, "rX"}}
	// own share as LocalSystem (seen as the machine account): add the local folder instead
	rep := buildPermReport(`\\WIN-IN3GO8AFV53\filmek`, "windows", `WORKGROUP\WIN-IN3GO8AFV53$`, "", probs, nil)
	if len(rep.Commands) != 0 || rep.User != `NT AUTHORITY\SYSTEM` || !strings.Contains(rep.Hint, "helyi mappájával") {
		t.Fatalf("own share %+v", rep)
	}
	// another machine's share as LocalSystem: run the service as a user
	rep = buildPermReport(`\\nas\filmek`, "windows", `NT AUTHORITY\SYSTEM`, "", []PermProblem{{`\\nas\filmek`, "rX"}}, nil)
	if len(rep.Commands) != 0 || !strings.Contains(rep.Hint, "services.msc") {
		t.Fatalf("other share %+v", rep)
	}
	// local folder as the machine account: icacls for SYSTEM
	rep = buildPermReport(`f:\filmek`, "windows", `WORKGROUP\WIN-IN3GO8AFV53$`, "S-1-5-18", []PermProblem{{`f:\filmek`, "rX"}}, nil)
	if len(rep.Commands) != 1 || rep.Commands[0] != `icacls "f:\filmek" /grant "*S-1-5-18:(OI)(CI)RX" /T` {
		t.Fatalf("local %q", rep.Commands)
	}
}

// NetworkService also reads as WORKGROUP\HOST$: the SID decides, not the name.
func TestPermReportWindowsGrantsBySID(t *testing.T) {
	rep := buildPermReport(`f:\filmek`, "windows", `WORKGROUP\WIN-IN3GO8AFV53$`, "S-1-5-20", []PermProblem{{`f:\filmek`, "rX"}}, nil)
	if rep.User != `NT AUTHORITY\NETWORK SERVICE` || rep.Commands[0] != `icacls "f:\filmek" /grant "*S-1-5-20:(OI)(CI)RX" /T` {
		t.Fatalf("network service %q %q", rep.User, rep.Commands)
	}
	rep = buildPermReport(`f:\filmek`, "windows", `SRV\kinora`, "S-1-5-21-1-2-3-1001", []PermProblem{{`f:\filmek`, "rX"}}, nil)
	if rep.User != `SRV\kinora` || rep.Commands[0] != `icacls "f:\filmek" /grant "*S-1-5-21-1-2-3-1001:(OI)(CI)RX" /T` {
		t.Fatalf("user %q %q", rep.User, rep.Commands)
	}
}

// "Javítás most" is offered only where the server can grant the rights itself (Windows, local folder).
func TestPermReportWindowsCanFix(t *testing.T) {
	rep := buildPermReport(`f:\filmek`, "windows", `NT AUTHORITY\SYSTEM`, "S-1-5-18", []PermProblem{{`f:\filmek`, "rX"}}, nil)
	if rep.CanFix != canGrantRead {
		t.Fatalf("local can_fix %v", rep.CanFix)
	}
	rep = buildPermReport(`\\nas\filmek`, "windows", `NT AUTHORITY\SYSTEM`, "S-1-5-18", []PermProblem{{`\\nas\filmek`, "rX"}}, nil)
	if rep.CanFix {
		t.Fatal("a share cannot be fixed from here")
	}
}
