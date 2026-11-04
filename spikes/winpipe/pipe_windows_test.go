package winpipe

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

const (
	genericAll       = windows.GENERIC_ALL
	genericReadWrite = windows.GENERIC_READ | windows.GENERIC_WRITE
	localSystemSID   = "S-1-5-18"
	authUsersSID     = "S-1-5-11"
	dialTimeout      = 5 * time.Second
)

func uniquePipePath() string {
	return fmt.Sprintf(`\\.\pipe\edgesync-winpipe-test-%d-%d`, os.Getpid(), time.Now().UnixNano())
}

func currentSID(t *testing.T) string {
	t.Helper()
	sid, err := CurrentUserSID()
	if err != nil {
		t.Fatalf("CurrentUserSID: %v", err)
	}
	return sid
}

func listenUser(t *testing.T, path, sid string) net.Listener {
	t.Helper()
	sddl, err := UserPipeSDDL(sid)
	if err != nil {
		t.Fatalf("UserPipeSDDL: %v", err)
	}
	ln, err := winio.ListenPipe(path, &winio.PipeConfig{SecurityDescriptor: sddl})
	if err != nil {
		t.Fatalf("ListenPipe: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	return ln
}

// pipePair connects a client to a listener on path and returns both ends; the server end is the one accepted.
func pipePair(t *testing.T, path string, ln net.Listener) (client, server net.Conn) {
	t.Helper()
	type accepted struct {
		conn net.Conn
		err  error
	}
	acceptCh := make(chan accepted, 1)
	go func() {
		conn, err := ln.Accept()
		acceptCh <- accepted{conn, err}
	}()
	timeout := dialTimeout
	client, err := winio.DialPipe(path, &timeout)
	if err != nil {
		t.Fatalf("DialPipe: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	res := <-acceptCh
	if res.err != nil {
		t.Fatalf("Accept: %v", res.err)
	}
	t.Cleanup(func() { res.conn.Close() })
	return client, res.conn
}

func TestPipe_SDDLDeniesOtherSID(t *testing.T) {
	user := currentSID(t)
	sddl, err := UserPipeSDDL(user)
	if err != nil {
		t.Fatalf("UserPipeSDDL: %v", err)
	}
	dacl, err := ParseDACL(sddl)
	if err != nil {
		t.Fatalf("ParseDACL: %v", err)
	}
	want := []ACE{
		{SID: localSystemSID, Mask: genericAll},
		{SID: user, Mask: genericAll},
	}
	if !dacl.Protected || !slices.Equal(dacl.Entries, want) {
		t.Fatalf("DACL protected=%t entries %+v, want protected entries %+v", dacl.Protected, dacl.Entries, want)
	}
}

func TestPipe_ServiceSDDLGrantsAuthenticatedUsersReadWrite(t *testing.T) {
	dacl, err := ParseDACL(ServiceSDDL)
	if err != nil {
		t.Fatalf("ParseDACL: %v", err)
	}
	want := []ACE{
		{SID: localSystemSID, Mask: genericAll},
		{SID: authUsersSID, Mask: genericReadWrite},
	}
	if !dacl.Protected || !slices.Equal(dacl.Entries, want) {
		t.Fatalf("DACL protected=%t entries %+v, want protected entries %+v", dacl.Protected, dacl.Entries, want)
	}
}

func TestPipe_FirstInstanceRejectsSecondServer(t *testing.T) {
	path := uniquePipePath()
	user := currentSID(t)
	listenUser(t, path, user)
	sddl, err := UserPipeSDDL(user)
	if err != nil {
		t.Fatalf("UserPipeSDDL: %v", err)
	}
	second, err := winio.ListenPipe(path, &winio.PipeConfig{SecurityDescriptor: sddl})
	if err == nil {
		second.Close()
		t.Fatal("second server created the same pipe name")
	}
}

func TestPipe_ClientVerifiesServerSID(t *testing.T) {
	path := uniquePipePath()
	user := currentSID(t)
	client, _ := pipePair(t, path, listenUser(t, path, user))

	if err := VerifyServer(client, user); err != nil {
		t.Fatalf("VerifyServer for the server's own account: %v", err)
	}
	if err := VerifyServer(client, localSystemSID); !errors.Is(err, ErrServerIdentity) {
		t.Fatalf("VerifyServer against another account: got %v, want ErrServerIdentity", err)
	}
}

func TestPipe_ServerVerifiesClientPeer(t *testing.T) {
	path := uniquePipePath()
	user := currentSID(t)
	_, server := pipePair(t, path, listenUser(t, path, user))

	peer, err := ClientPeer(server)
	if err != nil {
		t.Fatalf("ClientPeer: %v", err)
	}
	session, err := CurrentSession()
	if err != nil {
		t.Fatalf("CurrentSession: %v", err)
	}
	image, err := CurrentImage()
	if err != nil {
		t.Fatalf("CurrentImage: %v", err)
	}
	if peer.PID != windows.GetCurrentProcessId() || peer.SID != user || peer.Session != session || !strings.EqualFold(peer.Image, image) {
		t.Fatalf("client peer %+v, want pid %d sid %s session %d image %s",
			peer, windows.GetCurrentProcessId(), user, session, image)
	}
	if err := Authorize(peer, Expectation{SID: user, Session: session, Image: image}); err != nil {
		t.Fatalf("Authorize of the test process: %v", err)
	}
}

func TestPipe_TLS13HandshakeThroughNamedPipe(t *testing.T) {
	path := uniquePipePath()
	user := currentSID(t)
	session, err := CurrentSession()
	if err != nil {
		t.Fatalf("CurrentSession: %v", err)
	}
	image, err := CurrentImage()
	if err != nil {
		t.Fatalf("CurrentImage: %v", err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	responder, err := NewResponder(key)
	if err != nil {
		t.Fatalf("NewResponder: %v", err)
	}

	ln := listenUser(t, path, user)
	client, server := pipePair(t, path, ln)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	served := make(chan error, 1)
	go func() {
		served <- responder.Serve(ctx, server, ClientPeer, Expectation{SID: user, Session: session, Image: image})
	}()

	if err := VerifyServer(client, user); err != nil {
		t.Fatalf("VerifyServer: %v", err)
	}
	proxy, err := NewProxySigner(client)
	if err != nil {
		t.Fatalf("NewProxySigner: %v", err)
	}
	cert, roots, err := SelfSignedLoopbackCert(proxy)
	if err != nil {
		t.Fatalf("SelfSignedLoopbackCert: %v", err)
	}
	res, err := LoopbackHandshake(cert, roots)
	if err != nil {
		t.Fatalf("LoopbackHandshake: %v", err)
	}
	if res.Version != tls.VersionTLS13 {
		t.Fatalf("negotiated version %#x, want TLS 1.3", res.Version)
	}
	t.Logf("TLS 1.3 handshake with the key in the server process, over a named pipe: %v", res.Elapsed)

	cancel()
	client.Close()
	if err := <-served; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Serve: %v", err)
	}
}
