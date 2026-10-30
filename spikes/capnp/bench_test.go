package capnp_test

import "testing"

func BenchmarkEncodeHello(b *testing.B) {
	h := sampleHello()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := encodeHello(h); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeClipAnnounce(b *testing.B) {
	data, err := encodeClipAnnounce(sampleClipAnnounce())
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := decodeClipAnnounce(data); err != nil {
			b.Fatal(err)
		}
	}
}
