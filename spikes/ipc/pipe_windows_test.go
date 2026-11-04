//go:build windows

package ipc

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	pipeMessageSize = 1024
	pipeRoundTrips  = 5000
	pipeIOTimeout   = time.Minute
)

func TestPipe_OwnerOnlySDDL(t *testing.T) {
	sid, err := currentUserSID()
	if err != nil {
		t.Fatal(err)
	}
	wantSDDL := "D:P(A;;GA;;;SY)(A;;GA;;;" + sid + ")"
	if got := ownerOnlySDDL(sid); got != wantSDDL {
		t.Fatalf("ownerOnlySDDL() = %q, want %q", got, wantSDDL)
	}

	path := uniquePipePath(t)
	ln, err := listenOwnerOnly(path, sid)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	if dup, err := listenOwnerOnly(path, sid); err == nil {
		dup.Close()
		t.Fatal("second listener on the same pipe name was created; first-instance semantics are not enforced")
	}

	type accepted struct {
		conn net.Conn
		err  error
	}
	acceptedCh := make(chan accepted, 1)
	go func() {
		conn, err := ln.Accept()
		acceptedCh <- accepted{conn, err}
	}()
	client, err := dialVerified(path, sid, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	server := <-acceptedCh
	if server.err != nil {
		t.Fatal(server.err)
	}
	t.Cleanup(func() { server.conn.Close() })

	serverHandle, err := connHandle(server.conn)
	if err != nil {
		t.Fatal(err)
	}
	actualSD, err := windows.GetSecurityInfo(serverHandle, windows.SE_KERNEL_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatalf("read pipe DACL: %v", err)
	}
	actual, err := summarizeDACL(actualSD)
	if err != nil {
		t.Fatalf("parse pipe DACL: %v", err)
	}
	// The kernel stores the GA of the requested SDDL as FILE_ALL_ACCESS (FA) on a pipe.
	want, err := summarizeSDDL("D:P(A;;FA;;;S-1-5-18)(A;;FA;;;" + sid + ")")
	if err != nil {
		t.Fatal(err)
	}
	if !sameDACL(actual, want) {
		t.Errorf("pipe DACL = %v, want %v", actual, want)
	}

	if err := verifyServerUser(client, "S-1-0-0"); !errors.Is(err, ErrServerIdentity) {
		t.Errorf("verifyServerUser with a foreign SID = %v, want ErrServerIdentity", err)
	}
	if err := verifyServerUser(client, sid); err != nil {
		t.Errorf("verifyServerUser with the current SID: %v", err)
	}
}

func TestPipe_RoundTripLatency1KiB(t *testing.T) {
	client := startEchoPipe(t)
	payload := make([]byte, pipeMessageSize)
	reply := make([]byte, pipeMessageSize)

	start := time.Now()
	for i := range pipeRoundTrips {
		binary.LittleEndian.PutUint32(payload, uint32(i))
		if _, err := client.Write(payload); err != nil {
			t.Fatalf("round trip %d write: %v", i, err)
		}
		if _, err := io.ReadFull(client, reply); err != nil {
			t.Fatalf("round trip %d read: %v", i, err)
		}
		if !bytes.Equal(reply, payload) {
			t.Fatalf("round trip %d: echoed payload differs", i)
		}
	}
	elapsed := time.Since(start)
	t.Logf("%d round trips of %d bytes over a local named pipe: mean %s per round trip",
		pipeRoundTrips, pipeMessageSize, elapsed/pipeRoundTrips)
}

func BenchmarkPipeRoundTrip1KiB(b *testing.B) {
	client := startEchoPipe(b)
	payload := make([]byte, pipeMessageSize)
	reply := make([]byte, pipeMessageSize)
	b.ReportAllocs()
	b.SetBytes(pipeMessageSize)
	for b.Loop() {
		if _, err := client.Write(payload); err != nil {
			b.Fatal(err)
		}
		if _, err := io.ReadFull(client, reply); err != nil {
			b.Fatal(err)
		}
	}
}

func startEchoPipe(tb testing.TB) net.Conn {
	tb.Helper()
	sid, err := currentUserSID()
	if err != nil {
		tb.Fatal(err)
	}
	path := uniquePipePath(tb)
	ln, err := listenOwnerOnly(path, sid)
	if err != nil {
		tb.Fatal(err)
	}
	served := make(chan error, 1)
	go func() { served <- echoOne(ln) }()

	client, err := dialVerified(path, sid, 5*time.Second)
	if err != nil {
		ln.Close()
		<-served
		tb.Fatal(err)
	}
	if err := client.SetDeadline(time.Now().Add(pipeIOTimeout)); err != nil {
		client.Close()
		ln.Close()
		<-served
		tb.Fatal(err)
	}
	tb.Cleanup(func() {
		client.Close()
		ln.Close()
		if err := <-served; err != nil && !errors.Is(err, net.ErrClosed) {
			tb.Errorf("echo server: %v", err)
		}
	})
	return client
}

func echoOne(ln net.Listener) error {
	conn, err := ln.Accept()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = io.Copy(conn, conn)
	return err
}

func uniquePipePath(tb testing.TB) string {
	tb.Helper()
	var suffix [16]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		tb.Fatal(err)
	}
	return `\\.\pipe\edgesync-spike-ipc-` + hex.EncodeToString(suffix[:])
}

type daclEntry struct {
	mask uint32
	sid  *windows.SID
}

type daclSummary struct {
	protected bool
	allow     []daclEntry
}

func (s daclSummary) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "protected=%t", s.protected)
	for _, e := range s.allow {
		fmt.Fprintf(&b, " allow(0x%X, %s)", e.mask, e.sid.String())
	}
	return b.String()
}

// summarizeDACL compares SIDs as binary values: Windows renders well-known accounts such as the local
// Administrator as SDDL aliases (LA) in SDDL text, so text comparison is not reliable.
func summarizeDACL(sd *windows.SECURITY_DESCRIPTOR) (daclSummary, error) {
	control, _, err := sd.Control()
	if err != nil {
		return daclSummary{}, fmt.Errorf("read security descriptor control: %w", err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return daclSummary{}, fmt.Errorf("read DACL: %w", err)
	}
	if dacl == nil {
		return daclSummary{}, errors.New("security descriptor has a NULL DACL")
	}
	s := daclSummary{protected: control&windows.SE_DACL_PROTECTED != 0}
	for i := range uint32(dacl.AceCount) {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			return daclSummary{}, fmt.Errorf("read ACE %d: %w", i, err)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return daclSummary{}, fmt.Errorf("ACE %d has type %d, want an allow ACE", i, ace.Header.AceType)
		}
		s.allow = append(s.allow, daclEntry{
			mask: uint32(ace.Mask),
			sid:  (*windows.SID)(unsafe.Pointer(&ace.SidStart)),
		})
	}
	return s, nil
}

func summarizeSDDL(sddl string) (daclSummary, error) {
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return daclSummary{}, fmt.Errorf("parse %q: %w", sddl, err)
	}
	return summarizeDACL(sd)
}

// sameDACL ignores ACE order. Equal lengths and a one-to-one match make it a set equality.
func sameDACL(a, b daclSummary) bool {
	if a.protected != b.protected || len(a.allow) != len(b.allow) {
		return false
	}
	matched := make([]bool, len(b.allow))
	for _, ea := range a.allow {
		found := false
		for j, eb := range b.allow {
			if !matched[j] && ea.mask == eb.mask && ea.sid.Equals(eb.sid) {
				matched[j] = true
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// SY (S-1-5-18) and BA (S-1-5-32-544) have fixed SIDs. LA is domain-relative RID 500 of the local
// account domain, so it cannot be compared against a synthetic SID on another machine.
func TestDACLSameDACL(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{"alias and SID render equal", "D:P(A;;FA;;;SY)(A;;FA;;;BA)", "D:P(A;;FA;;;S-1-5-18)(A;;FA;;;S-1-5-32-544)", true},
		{"ACE order ignored", "D:P(A;;FA;;;SY)(A;;FA;;;BA)", "D:P(A;;FA;;;BA)(A;;FA;;;SY)", true},
		{"different SID", "D:P(A;;FA;;;SY)(A;;FA;;;BA)", "D:P(A;;FA;;;SY)(A;;FA;;;S-1-5-32-545)", false},
		{"different mask", "D:P(A;;FA;;;SY)(A;;FA;;;BA)", "D:P(A;;GA;;;SY)(A;;FA;;;BA)", false},
		{"not protected", "D:P(A;;FA;;;SY)(A;;FA;;;BA)", "D:(A;;FA;;;SY)(A;;FA;;;BA)", false},
		{"extra ACE", "D:P(A;;FA;;;SY)(A;;FA;;;BA)", "D:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FA;;;BU)", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, err := summarizeSDDL(tc.a)
			if err != nil {
				t.Fatal(err)
			}
			b, err := summarizeSDDL(tc.b)
			if err != nil {
				t.Fatal(err)
			}
			if got := sameDACL(a, b); got != tc.want {
				t.Errorf("sameDACL(%v, %v) = %t, want %t", a, b, got, tc.want)
			}
		})
	}
}
