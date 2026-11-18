//go:build windows

package server

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// The service (LocalSystem) holds SeBackupPrivilege and SeRestorePrivilege: with them enabled it can
// read and rewrite the permissions of any folder, so the admin does not have to run icacls by hand.

// withBackupPrivileges runs fn with SeBackupPrivilege and SeRestorePrivilege enabled, then disables
// them again: while enabled, every open would succeed and the permission check could not see problems.
func withBackupPrivileges(fn func() error) error {
	var tok windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &tok); err != nil {
		return err
	}
	defer tok.Close()
	set := func(name string, attr uint32) error {
		var luid windows.LUID
		if err := windows.LookupPrivilegeValue(nil, windows.StringToUTF16Ptr(name), &luid); err != nil {
			return err
		}
		tp := windows.Tokenprivileges{PrivilegeCount: 1}
		tp.Privileges[0] = windows.LUIDAndAttributes{Luid: luid, Attributes: attr}
		return windows.AdjustTokenPrivileges(tok, false, &tp, 0, nil, nil)
	}
	for _, p := range []string{"SeBackupPrivilege", "SeRestorePrivilege"} {
		if err := set(p, windows.SE_PRIVILEGE_ENABLED); err != nil {
			// not held (not an administrator / LocalSystem): try anyway, the owner may change the DACL
			if !errors.Is(err, windows.ERROR_NOT_ALL_ASSIGNED) {
				return err
			}
		}
		defer set(p, 0)
	}
	return fn()
}

func readAccess(sid *windows.SID, inherit bool) []windows.EXPLICIT_ACCESS {
	inh := uint32(windows.NO_INHERITANCE)
	if inherit {
		inh = windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT
	}
	return []windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.GENERIC_READ | windows.GENERIC_EXECUTE,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       inh,
		Trustee: windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: windows.TRUSTEE_IS_UNKNOWN,
			TrusteeValue: windows.TrusteeValueFromSID(sid)},
	}}
}

// grantNamed adds the ACE by name: Windows also pushes an inheritable ACE down to the children.
func grantNamed(path string, sid *windows.SID, inherit bool) error {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	acl, err := windows.ACLFromEntries(readAccess(sid, inherit), dacl)
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
}

// grantHandle adds the ACE through a backup-semantics handle: works on objects the DACL locks out
// (SeRestorePrivilege), but does not propagate to the children.
// It also reports whether the object had its inheritance switched off (it would miss an inherited ACE).
func grantHandle(path string, sid *windows.SID, inherit, onlyProtected bool) (bool, error) {
	h, err := windows.CreateFile(windows.StringToUTF16Ptr(path), windows.READ_CONTROL|windows.WRITE_DAC,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return false, err
	}
	defer windows.CloseHandle(h)
	sd, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return false, err
	}
	ctrl, _, err := sd.Control()
	if err != nil {
		return false, err
	}
	if onlyProtected && ctrl&windows.SE_DACL_PROTECTED == 0 {
		return false, nil
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return false, err
	}
	acl, err := windows.ACLFromEntries(readAccess(sid, inherit), dacl)
	if err != nil {
		return false, err
	}
	return true, windows.SetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
}

// grantRead gives sid read access like icacls /grant sid:(OI)(CI)RX /T: "rX" problems get an inherited
// ACE on the folder and an explicit one on everything below that does not inherit (inheritance off,
// or the change could not be propagated); "x" problems (a parent) get read on that folder only.
func grantRead(sidStr string, probs []PermProblem) error {
	sid, err := windows.StringToSid(sidStr)
	if err != nil {
		return err
	}
	return withBackupPrivileges(func() error {
		var errs []error
		for _, pr := range probs {
			if pr.Need == "x" {
				if err := grantNamed(pr.Path, sid, false); err != nil {
					if _, err = grantHandle(pr.Path, sid, false, false); err != nil {
						errs = append(errs, err)
					}
				}
				continue
			}
			propagated := grantNamed(pr.Path, sid, true) == nil
			if !propagated {
				if _, err := grantHandle(pr.Path, sid, true, false); err != nil {
					errs = append(errs, err)
					continue
				}
			}
			failed := 0
			_ = filepath.WalkDir(pr.Path, func(p string, d fs.DirEntry, err error) error {
				if p == pr.Path {
					return nil
				}
				if err != nil {
					failed++
					return nil
				}
				if d.IsDir() && strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir // the scanner skips these too
				}
				if _, err := grantHandle(p, sid, d.IsDir(), propagated); err != nil {
					failed++
				}
				return nil
			})
			if failed > 0 {
				errs = append(errs, fmt.Errorf("%s: %d elem jogát nem sikerült megadni", pr.Path, failed))
			}
		}
		return errors.Join(errs...)
	})
}

// canGrantRead: the server can fix permissions itself.
const canGrantRead = true
