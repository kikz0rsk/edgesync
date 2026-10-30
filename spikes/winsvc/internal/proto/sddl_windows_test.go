package proto

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestPipeSDDL_SystemOnlyProtectedDACL(t *testing.T) {
	sd, err := windows.SecurityDescriptorFromString(PipeSDDL)
	if err != nil {
		t.Fatalf("parse SDDL: %v", err)
	}
	control, _, err := sd.Control()
	if err != nil {
		t.Fatalf("Control: %v", err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatalf("DACL is not protected (control %#x)", control)
	}

	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		t.Fatalf("CreateWellKnownSid: %v", err)
	}
	owner, _, err := sd.Owner()
	if err != nil {
		t.Fatalf("Owner: %v", err)
	}
	if !owner.Equals(system) {
		t.Fatalf("owner %s, want %s", owner, system)
	}

	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatalf("DACL: %v", err)
	}
	if dacl == nil {
		t.Fatal("DACL is absent (a NULL DACL grants everyone access)")
	}
	if dacl.AceCount != 1 {
		t.Fatalf("ACE count %d, want 1", dacl.AceCount)
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(dacl, 0, &ace); err != nil {
		t.Fatalf("GetAce: %v", err)
	}
	if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
		t.Fatalf("ACE type %d, want allow", ace.Header.AceType)
	}
	aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	if !aceSID.Equals(system) {
		t.Fatalf("ACE SID %s, want %s", aceSID, system)
	}
	if LocalSystemSID != system.String() {
		t.Fatalf("LocalSystemSID %q differs from the well-known SID %q", LocalSystemSID, system)
	}
}
