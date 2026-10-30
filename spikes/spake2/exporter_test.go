package spake2

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"math/big"
	"net"
	"testing"
	"time"
)

const (
	exporterLabel   = "EXPORTER-EdgeSync-pairing-v1"
	exporterLength  = 32
	connDeadline    = 20 * time.Second
	handshakeBudget = 10 * time.Second
)

type tlsPair struct {
	client, server *tls.Conn
}

type tlsLoopback struct {
	listener  net.Listener
	clientCfg *tls.Config
}

func newTLSLoopback(t *testing.T) *tlsLoopback {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "edgesync-spake2-spike"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)

	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return &tlsLoopback{
		listener:  listener,
		clientCfg: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool, ServerName: "127.0.0.1"},
	}
}

func (l *tlsLoopback) connect(t *testing.T) tlsPair {
	t.Helper()
	type accepted struct {
		conn *tls.Conn
		err  error
	}
	result := make(chan accepted, 1)
	go func() {
		raw, err := l.listener.Accept()
		if err != nil {
			result <- accepted{err: err}
			return
		}
		conn := raw.(*tls.Conn)
		if err := conn.SetDeadline(time.Now().Add(connDeadline)); err != nil {
			result <- accepted{err: err}
			return
		}
		result <- accepted{conn: conn, err: conn.Handshake()}
	}()

	dialer := &net.Dialer{Timeout: handshakeBudget}
	client, err := tls.DialWithDialer(dialer, "tcp", l.listener.Addr().String(), l.clientCfg)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.SetDeadline(time.Now().Add(connDeadline)); err != nil {
		t.Fatal(err)
	}
	server := <-result
	if server.err != nil {
		t.Fatalf("accept: %v", server.err)
	}
	t.Cleanup(func() { _ = server.conn.Close() })
	return tlsPair{client: client, server: server.conn}
}

func exportBinding(t *testing.T, conn *tls.Conn) []byte {
	t.Helper()
	state := conn.ConnectionState()
	if state.Version != tls.VersionTLS13 {
		t.Fatalf("negotiated TLS version %#x, want TLS 1.3", state.Version)
	}
	binding, err := state.ExportKeyingMaterial(exporterLabel, nil, exporterLength)
	if err != nil {
		t.Fatalf("ExportKeyingMaterial: %v", err)
	}
	return binding
}

type pairingResult struct {
	key []byte
	err error
}

func runClientSide(conn io.ReadWriter, password, idA, idB, binding []byte) pairingResult {
	e, err := New(RoleA, password, idA, idB, binding, nil)
	if err != nil {
		return pairingResult{err: err}
	}
	if _, err := conn.Write(e.Share()); err != nil {
		return pairingResult{err: err}
	}
	peerShare := make([]byte, shareLen)
	if _, err := io.ReadFull(conn, peerShare); err != nil {
		return pairingResult{err: err}
	}
	confirm, err := e.Finish(peerShare)
	if err != nil {
		return pairingResult{err: err}
	}
	if _, err := conn.Write(confirm); err != nil {
		return pairingResult{err: err}
	}
	peerConfirm := make([]byte, confirmLen)
	if _, err := io.ReadFull(conn, peerConfirm); err != nil {
		return pairingResult{err: err}
	}
	key, err := e.Verify(peerConfirm)
	return pairingResult{key: key, err: err}
}

func runServerSide(conn io.ReadWriter, password, idA, idB, binding []byte) pairingResult {
	e, err := New(RoleB, password, idA, idB, binding, nil)
	if err != nil {
		return pairingResult{err: err}
	}
	peerShare := make([]byte, shareLen)
	if _, err := io.ReadFull(conn, peerShare); err != nil {
		return pairingResult{err: err}
	}
	if _, err := conn.Write(e.Share()); err != nil {
		return pairingResult{err: err}
	}
	confirm, err := e.Finish(peerShare)
	if err != nil {
		return pairingResult{err: err}
	}
	peerConfirm := make([]byte, confirmLen)
	if _, err := io.ReadFull(conn, peerConfirm); err != nil {
		return pairingResult{err: err}
	}
	key, err := e.Verify(peerConfirm)
	if err != nil {
		return pairingResult{err: err}
	}
	if _, err := conn.Write(confirm); err != nil {
		return pairingResult{err: err}
	}
	return pairingResult{key: key}
}

func pairOverConnection(t *testing.T, p tlsPair) (client, server pairingResult) {
	t.Helper()
	idA, idB := []byte(testPeerIDA), []byte(testPeerIDB)
	serverBinding, clientBinding := exportBinding(t, p.server), exportBinding(t, p.client)
	serverDone := make(chan pairingResult, 1)
	go func() {
		serverDone <- runServerSide(p.server, []byte(testCode), idA, idB, serverBinding)
	}()
	client = runClientSide(p.client, []byte(testCode), idA, idB, clientBinding)
	return client, <-serverDone
}

func TestExporter_BindingDiffersPerConnection(t *testing.T) {
	loopback := newTLSLoopback(t)
	first := loopback.connect(t)
	second := loopback.connect(t)

	firstClient, firstServer := exportBinding(t, first.client), exportBinding(t, first.server)
	secondClient, secondServer := exportBinding(t, second.client), exportBinding(t, second.server)

	t.Run("both ends of one connection agree", func(t *testing.T) {
		if !bytes.Equal(firstClient, firstServer) {
			t.Errorf("first connection: client %x server %x", firstClient, firstServer)
		}
		if !bytes.Equal(secondClient, secondServer) {
			t.Errorf("second connection: client %x server %x", secondClient, secondServer)
		}
		if len(firstClient) != exporterLength {
			t.Errorf("binding length %d", len(firstClient))
		}
	})

	t.Run("different connections differ", func(t *testing.T) {
		if bytes.Equal(firstClient, secondClient) {
			t.Fatalf("two connections exported the same binding %x", firstClient)
		}
	})

	t.Run("label separates exporters", func(t *testing.T) {
		state := first.client.ConnectionState()
		other, err := state.ExportKeyingMaterial("EXPORTER-EdgeSync-other-v1", nil, exporterLength)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(other, firstClient) {
			t.Fatal("different labels exported the same value")
		}
	})

	t.Run("pairing succeeds on each connection with its own binding", func(t *testing.T) {
		for name, p := range map[string]tlsPair{"first": first, "second": second} {
			client, server := pairOverConnection(t, p)
			if client.err != nil || server.err != nil {
				t.Fatalf("%s: client %v server %v", name, client.err, server.err)
			}
			if !bytes.Equal(client.key, server.key) || len(client.key) != keyHalfLen {
				t.Fatalf("%s: keys differ: %x vs %x", name, client.key, server.key)
			}
		}
	})

	t.Run("run bound to connection A fails confirmation when relayed to connection B", func(t *testing.T) {
		idA, idB := []byte(testPeerIDA), []byte(testPeerIDB)
		a, err := New(RoleA, []byte(testCode), idA, idB, firstClient, nil)
		if err != nil {
			t.Fatal(err)
		}
		b, err := New(RoleB, []byte(testCode), idA, idB, secondServer, nil)
		if err != nil {
			t.Fatal(err)
		}
		confA, err := a.Finish(b.Share())
		if err != nil {
			t.Fatal(err)
		}
		confB, err := b.Finish(a.Share())
		if err != nil {
			t.Fatal(err)
		}
		if key, err := b.Verify(confA); !errors.Is(err, ErrConfirmation) || key != nil {
			t.Errorf("responder on B accepted a confirmation from A: key %x err %v", key, err)
		}
		if key, err := a.Verify(confB); !errors.Is(err, ErrConfirmation) || key != nil {
			t.Errorf("initiator on A accepted a confirmation from B: key %x err %v", key, err)
		}
	})

	t.Run("binding alone decides the outcome with identical scalars", func(t *testing.T) {
		v := rfcVectorsP256SHA256[0]
		w, x, y := mustScalar(t, v.w), mustScalar(t, v.x), mustScalar(t, v.y)
		run := func(aadA, aadB []byte) error {
			a, err := newExchange(RoleA, w, x, []byte(v.idA), []byte(v.idB), aadA)
			if err != nil {
				t.Fatal(err)
			}
			b, err := newExchange(RoleB, w, y, []byte(v.idA), []byte(v.idB), aadB)
			if err != nil {
				t.Fatal(err)
			}
			confA, err := a.Finish(b.Share())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := b.Finish(a.Share()); err != nil {
				t.Fatal(err)
			}
			_, err = b.Verify(confA)
			return err
		}
		if err := run(firstClient, firstServer); err != nil {
			t.Fatalf("same binding: %v", err)
		}
		if err := run(firstClient, secondServer); !errors.Is(err, ErrConfirmation) {
			t.Fatalf("different binding: %v", err)
		}
	})
}
