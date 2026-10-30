package capnp_test

import (
	"errors"
	"testing"
)

func FuzzDecodeHello(f *testing.F) {
	valid, err := encodeHello(sampleHello())
	if err != nil {
		f.Fatalf("seed encode: %v", err)
	}
	f.Add(valid)
	f.Add([]byte{})
	f.Add(make([]byte, 8))

	f.Fuzz(func(t *testing.T, data []byte) {
		first, err := decodeHelloLimited(data)
		if err != nil {
			return
		}
		again, err := encodeHello(first)
		if errors.Is(err, errMessageTooLarge) {
			return
		}
		if err != nil {
			t.Fatalf("re-encode of accepted input failed: %v", err)
		}
		second, err := decodeHelloLimited(again)
		if err != nil {
			t.Fatalf("decode of re-encoded input failed: %v", err)
		}
		if !second.equal(first) {
			t.Fatalf("decode is not stable under re-encoding:\n first %+v\nsecond %+v", first, second)
		}
	})
}
