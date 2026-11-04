//go:build windows

package winpipe

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ServiceSDDL is the service pipe descriptor of D-47: SYSTEM full access, Authenticated Users read and write only.
const ServiceSDDL = "D:P(A;;GA;;;SY)(A;;GRGW;;;AU)"

// UserPipeSDDL is the owner-only descriptor of a per-user pipe (D-46): SYSTEM and one account, nothing else.
func UserPipeSDDL(userSID string) (string, error) {
	sid, err := windows.StringToSid(userSID)
	if err != nil {
		return "", fmt.Errorf("winpipe: parse user SID: %w", err)
	}
	if sid.String() != userSID {
		return "", fmt.Errorf("winpipe: user SID %q is not in canonical form", userSID)
	}
	return "D:P(A;;GA;;;SY)(A;;GA;;;" + userSID + ")", nil
}

type ACE struct {
	SID  string
	Mask uint32
}

type DACL struct {
	Protected bool
	Entries   []ACE
}

// ParseDACL reads an SDDL string back and lists its allow ACEs. A missing DACL is an error because a NULL DACL grants
// everyone access.
func ParseDACL(sddl string) (DACL, error) {
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return DACL{}, fmt.Errorf("winpipe: parse SDDL %q: %w", sddl, err)
	}
	control, _, err := sd.Control()
	if err != nil {
		return DACL{}, fmt.Errorf("winpipe: read control flags: %w", err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return DACL{}, fmt.Errorf("winpipe: read DACL: %w", err)
	}
	if dacl == nil {
		return DACL{}, fmt.Errorf("winpipe: SDDL %q has no DACL, which grants everyone access", sddl)
	}
	out := DACL{Protected: control&windows.SE_DACL_PROTECTED != 0}
	for i := range dacl.AceCount {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, uint32(i), &ace); err != nil {
			return DACL{}, fmt.Errorf("winpipe: read ACE %d: %w", i, err)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return DACL{}, fmt.Errorf("winpipe: ACE %d has type %d, want allow", i, ace.Header.AceType)
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		out.Entries = append(out.Entries, ACE{SID: sid.String(), Mask: uint32(ace.Mask)})
	}
	return out, nil
}
