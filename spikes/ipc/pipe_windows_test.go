//go:build windows

package ipc

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"testing"
	"time"

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
	// The kernel stores the GA of the requested SDDL as FILE_ALL_ACCESS (FA) on a pipe.
	storedDACL := "D:P(A;;FA;;;SY)(A;;FA;;;" + sid + ")"
	if got := actualSD.String(); got != storedDACL {
		t.Errorf("pipe DACL = %q, want %q", got, storedDACL)
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
