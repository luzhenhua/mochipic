package files

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/sys/windows"
)

func denyCreatingFiles(t *testing.T, path string) {
	t.Helper()
	const daclInfo = windows.DACL_SECURITY_INFORMATION
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, daclInfo)
	if err != nil {
		t.Fatal(err)
	}
	previous, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	var pin runtime.Pinner
	pin.Pin(user.User.Sid)
	defer pin.Unpin()
	restricted, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.FILE_WRITE_DATA, // FILE_ADD_FILE on directories.
		AccessMode:        windows.DENY_ACCESS,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_USER,
			TrusteeValue: windows.TrusteeValueFromSID(user.User.Sid),
		},
	}}, previous)
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, daclInfo, nil, nil, restricted, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, daclInfo, nil, nil, previous, nil); err != nil {
			t.Error(err)
		}
	})
	if err := os.WriteFile(filepath.Join(path, "access-probe"), nil, 0o600); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("expected the DACL to deny file creation, got %v", err)
	}
}
