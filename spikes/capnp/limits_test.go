package capnp_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"capnproto.org/go/capnp/v3"

	"github.com/kikz0r/edgesync/spikes/capnp/gen"
)

var errMessageTooLarge = errors.New("message exceeds control size cap")

const (
	aliasedBlobBytes  = 4 << 20
	aliasedRefs       = 32
	pointerChainLen   = 40
	compositeChainLen = 40
)

func buildAliasedHello(t *testing.T) []byte {
	t.Helper()
	msg, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		t.Fatalf("new message: %v", err)
	}
	root, err := gen.NewRootHello(seg)
	if err != nil {
		t.Fatalf("root hello: %v", err)
	}
	blob, err := capnp.NewData(seg, bytes.Repeat([]byte{0xa5}, aliasedBlobBytes))
	if err != nil {
		t.Fatalf("blob: %v", err)
	}
	refs, err := capnp.NewPointerList(seg, aliasedRefs)
	if err != nil {
		t.Fatalf("refs: %v", err)
	}
	for i := range aliasedRefs {
		if err := refs.Set(i, blob.ToPtr()); err != nil {
			t.Fatalf("ref %d: %v", i, err)
		}
	}
	if err := root.SetMacs(capnp.DataList(refs)); err != nil {
		t.Fatalf("set macs: %v", err)
	}
	out, err := msg.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return out
}

func TestLimits_TraversalBombRejected(t *testing.T) {
	data := buildAliasedHello(t)
	if len(data) > controlMaxMessageBytes {
		t.Fatalf("fixture is %d bytes, above the control cap; the bomb must stay under the size cap", len(data))
	}

	readAll := func(traverseWords uint64) (int, error) {
		msg, err := decodeLimited(data, traverseWords, controlDepthLimit)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		root, err := gen.ReadRootHello(msg)
		if err != nil {
			t.Fatalf("root hello: %v", err)
		}
		macs, err := root.Macs()
		if err != nil {
			return -1, err
		}
		for i := range macs.Len() {
			if _, err := macs.At(i); err != nil {
				return i, err
			}
		}
		return macs.Len(), nil
	}

	failedAt, err := readAll(controlTraverseLimitWords)
	if err == nil {
		t.Fatalf("all %d aliased reads succeeded under the control traversal limit", failedAt)
	}
	t.Logf("observed traversal error at element %d: %v", failedAt, err)
	if !strings.Contains(err.Error(), "read traversal limit reached") {
		t.Fatalf("unexpected error: %v", err)
	}
	// Root and list pointer reads spend part of the 64 MiB budget, so only 15 of the 4 MiB reads fit.
	const wantFailIndex = 15
	if failedAt != wantFailIndex {
		t.Fatalf("traversal limit hit at element %d, want %d", failedAt, wantFailIndex)
	}

	n, err := readAll(1 << 30)
	if err != nil {
		t.Fatalf("control read with a 1 GiB-word limit failed: %v", err)
	}
	if n != aliasedRefs {
		t.Fatalf("control read returned %d elements, want %d", n, aliasedRefs)
	}
}

func buildPointerChain(t *testing.T, levels int) []byte {
	t.Helper()
	msg, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		t.Fatalf("new message: %v", err)
	}
	root, err := capnp.NewRootStruct(seg, capnp.ObjectSize{PointerCount: 1})
	if err != nil {
		t.Fatalf("root struct: %v", err)
	}
	child := capnp.Ptr{}
	for range levels {
		pl, err := capnp.NewPointerList(seg, 1)
		if err != nil {
			t.Fatalf("pointer list: %v", err)
		}
		if err := pl.Set(0, child); err != nil {
			t.Fatalf("set chain: %v", err)
		}
		child = pl.ToPtr()
	}
	if err := root.SetPtr(0, child); err != nil {
		t.Fatalf("set root ptr: %v", err)
	}
	out, err := msg.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return out
}

func walkPointerChain(data []byte, depth uint, levels int) (int, error) {
	msg, err := decodeLimited(data, controlTraverseLimitWords, depth)
	if err != nil {
		return 0, err
	}
	root, err := msg.Root()
	if err != nil {
		return 0, err
	}
	p, err := root.Struct().Ptr(0)
	if err != nil {
		return 0, err
	}
	for i := range levels {
		p, err = capnp.PointerList(p.List()).At(0)
		if err != nil {
			return i, err
		}
	}
	return levels, nil
}

func TestLimits_DepthLimitRejected(t *testing.T) {
	t.Run("pointer chain", func(t *testing.T) {
		data := buildPointerChain(t, pointerChainLen)
		reached, err := walkPointerChain(data, controlDepthLimit, pointerChainLen)
		if err == nil {
			t.Fatalf("a %d-level chain was accepted under DepthLimit %d", pointerChainLen, controlDepthLimit)
		}
		t.Logf("observed pointer chain error after %d dereferences: %v", reached, err)
		if !strings.Contains(err.Error(), "depth limit reached") {
			t.Fatalf("unexpected error after %d levels: %v", reached, err)
		}
		reached, err = walkPointerChain(data, 64, pointerChainLen)
		if err != nil {
			t.Fatalf("control walk under DepthLimit 64 failed after %d levels: %v", reached, err)
		}
	})

	t.Run("schema monitor id", func(t *testing.T) {
		data, err := encodeHello(hello{monitors: []monitor{{id: "DISPLAY-1"}}})
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		msg, err := decodeLimited(data, controlTraverseLimitWords, 4)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		root, err := gen.ReadRootHello(msg)
		if err != nil {
			t.Fatalf("root hello: %v", err)
		}
		desktop, err := root.Desktop()
		if err != nil {
			t.Fatalf("desktop: %v", err)
		}
		monitors, err := desktop.Monitors()
		if err != nil {
			t.Fatalf("monitors: %v", err)
		}
		_, err = monitors.At(0).Id()
		t.Logf("observed schema monitor id error at DepthLimit 4: %v", err)
		if err == nil || !strings.Contains(err.Error(), "depth limit reached") {
			t.Fatalf("monitor id under DepthLimit 4: got %v, want depth limit error", err)
		}
	})
}

func TestLimits_CompositeListDepthBypass(t *testing.T) {
	t.Run("schema monitor id at DepthLimit 3", func(t *testing.T) {
		data, err := encodeHello(hello{monitors: []monitor{{id: "DISPLAY-1"}}})
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		msg, err := decodeLimited(data, controlTraverseLimitWords, 3)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		root, err := gen.ReadRootHello(msg)
		if err != nil {
			t.Fatalf("root hello: %v", err)
		}
		desktop, err := root.Desktop()
		if err != nil {
			t.Fatalf("desktop: %v", err)
		}
		monitors, err := desktop.Monitors()
		if err != nil {
			t.Fatalf("monitors: %v", err)
		}
		id, err := monitors.At(0).Id()
		if err != nil {
			t.Fatalf("expected the composite-list underflow to let the read through, got: %v", err)
		}
		if id != "DISPLAY-1" {
			t.Fatalf("id = %q", id)
		}
	})

	t.Run("generic composite chain at DepthLimit 32", func(t *testing.T) {
		data := buildCompositeChain(t, compositeChainLen)
		msg, err := decodeLimited(data, controlTraverseLimitWords, controlDepthLimit)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		root, err := msg.Root()
		if err != nil {
			t.Fatalf("root: %v", err)
		}
		st := root.Struct()
		for i := range compositeChainLen {
			p, err := st.Ptr(0)
			if err != nil {
				t.Fatalf("level %d ptr: %v", i, err)
			}
			st = capnp.List(p.List()).Struct(0)
		}
	})
}

func buildCompositeChain(t *testing.T, levels int) []byte {
	t.Helper()
	msg, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		t.Fatalf("new message: %v", err)
	}
	root, err := capnp.NewRootStruct(seg, capnp.ObjectSize{PointerCount: 1})
	if err != nil {
		t.Fatalf("root struct: %v", err)
	}
	child := capnp.Ptr{}
	for range levels {
		l, err := capnp.NewCompositeList(seg, capnp.ObjectSize{PointerCount: 1}, 1)
		if err != nil {
			t.Fatalf("composite list: %v", err)
		}
		if err := l.Struct(0).SetPtr(0, child); err != nil {
			t.Fatalf("set chain: %v", err)
		}
		child = l.ToPtr()
	}
	if err := root.SetPtr(0, child); err != nil {
		t.Fatalf("set root ptr: %v", err)
	}
	out, err := msg.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return out
}

func TestLimits_OversizedMessageRejected(t *testing.T) {
	data := make([]byte, controlMaxMessageBytes+1)
	if _, err := capnp.Unmarshal(data); err != nil {
		t.Fatalf("precondition: the zero-filled buffer must be structurally accepted by Unmarshal so the size check is what rejects it, got %v", err)
	}
	_, err := decodeLimited(data, controlTraverseLimitWords, controlDepthLimit)
	t.Logf("observed oversize error: %v", err)
	if !errors.Is(err, errMessageTooLarge) {
		t.Fatalf("oversized message: got %v, want errMessageTooLarge", err)
	}
}
