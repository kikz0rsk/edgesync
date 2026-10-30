package spake2

import "testing"

func BenchmarkSPAKE2Full(b *testing.B) {
	password, idA, idB := []byte(testCode), []byte(testPeerIDA), []byte(testPeerIDB)
	binding := make([]byte, exporterLength)
	b.ReportAllocs()
	for b.Loop() {
		a, err := New(RoleA, password, idA, idB, binding, nil)
		if err != nil {
			b.Fatal(err)
		}
		r, err := New(RoleB, password, idA, idB, binding, nil)
		if err != nil {
			b.Fatal(err)
		}
		confA, err := a.Finish(r.Share())
		if err != nil {
			b.Fatal(err)
		}
		confB, err := r.Finish(a.Share())
		if err != nil {
			b.Fatal(err)
		}
		if _, err := r.Verify(confA); err != nil {
			b.Fatal(err)
		}
		if _, err := a.Verify(confB); err != nil {
			b.Fatal(err)
		}
	}
}
