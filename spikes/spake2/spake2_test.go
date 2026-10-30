package spake2

import (
	"bytes"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"
	"testing"

	"filippo.io/nistec"
)

const (
	testPeerIDA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testPeerIDB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	testCode    = "483920"
)

const orderHex = "ffffffff00000000ffffffffffffffffbce6faada7179e84f3b9cac2fc632551"

// RFC 9382 Appendix B, P256-SHA256-HKDF-HMAC section, transcribed mechanically from
// https://www.rfc-editor.org/rfc/rfc9382.txt (all four vectors).
type rfcVector struct {
	idA, idB                   string
	w, x, pA, y, pB, k         string
	tt, hash, ke, ka, kcA, kcB string
	confA, confB               string
}

var rfcVectorsP256SHA256 = []rfcVector{
	{
		idA: "server", idB: "client",
		w:     "2ee57912099d31560b3a44b1184b9b4866e904c49d12ac5042c97dca461b1a5f",
		x:     "43dd0fd7215bdcb482879fca3220c6a968e66d70b1356cac18bb26c84a78d729",
		pA:    "04a56fa807caaa53a4d28dbb9853b9815c61a411118a6fe516a8798434751470f9010153ac33d0d5f2047ffdb1a3e42c9b4e6be662766e1eeb4116988ede5f912c",
		y:     "dcb60106f276b02606d8ef0a328c02e4b629f84f89786af5befb0bc75b6e66be",
		pB:    "0406557e482bd03097ad0cbaa5df82115460d951e3451962f1eaf4367a420676d09857ccbc522686c83d1852abfa8ed6e4a1155cf8f1543ceca528afb591a1e0b7",
		k:     "0412af7e89717850671913e6b469ace67bd90a4df8ce45c2af19010175e37eed69f75897996d539356e2fa6a406d528501f907e04d97515fbe83db277b715d3325",
		tt:    "06000000000000007365727665720600000000000000636c69656e74410000000000000004a56fa807caaa53a4d28dbb9853b9815c61a411118a6fe516a8798434751470f9010153ac33d0d5f2047ffdb1a3e42c9b4e6be662766e1eeb4116988ede5f912c41000000000000000406557e482bd03097ad0cbaa5df82115460d951e3451962f1eaf4367a420676d09857ccbc522686c83d1852abfa8ed6e4a1155cf8f1543ceca528afb591a1e0b741000000000000000412af7e89717850671913e6b469ace67bd90a4df8ce45c2af19010175e37eed69f75897996d539356e2fa6a406d528501f907e04d97515fbe83db277b715d332520000000000000002ee57912099d31560b3a44b1184b9b4866e904c49d12ac5042c97dca461b1a5f",
		hash:  "0e0672dc86f8e45565d338b0540abe6915bdf72e2b35b5c9e5663168e960a91b",
		ke:    "0e0672dc86f8e45565d338b0540abe69",
		ka:    "15bdf72e2b35b5c9e5663168e960a91b",
		kcA:   "00c12546835755c86d8c0db7851ae86f",
		kcB:   "a9fa3406c3b781b93d804485430ca27a",
		confA: "58ad4aa88e0b60d5061eb6b5dd93e80d9c4f00d127c65b3b35b1b5281fee38f0",
		confB: "d3e2e547f1ae04f2dbdbf0fc4b79f8ecff2dff314b5d32fe9fcef2fb26dc459b",
	},
	{
		idA: "", idB: "client",
		w:     "0548d8729f730589e579b0475a582c1608138ddf7054b73b5381c7e883e2efae",
		x:     "403abbe3b1b4b9ba17e3032849759d723939a27a27b9d921c500edde18ed654b",
		pA:    "04a897b769e681c62ac1c2357319a3d363f610839c4477720d24cbe32f5fd85f44fb92ba966578c1b712be6962498834078262caa5b441ecfa9d4a9485720e918a",
		y:     "903023b6598908936ea7c929bd761af6039577a9c3f9581064187c3049d87065",
		pB:    "04e0f816fd1c35e22065d5556215c097e799390d16661c386e0ecc84593974a61b881a8c82327687d0501862970c64565560cb5671f696048050ca66ca5f8cc7fc",
		k:     "048f83ec9f6e4f87cc6f9dc740bdc2769725f923364f01c84148c049a39a735ebda82eac03e00112fd6a5710682767cff5361f7e819e53d8d3c3a2922e0d837aa6",
		tt:    "00000000000000000600000000000000636c69656e74410000000000000004a897b769e681c62ac1c2357319a3d363f610839c4477720d24cbe32f5fd85f44fb92ba966578c1b712be6962498834078262caa5b441ecfa9d4a9485720e918a410000000000000004e0f816fd1c35e22065d5556215c097e799390d16661c386e0ecc84593974a61b881a8c82327687d0501862970c64565560cb5671f696048050ca66ca5f8cc7fc4100000000000000048f83ec9f6e4f87cc6f9dc740bdc2769725f923364f01c84148c049a39a735ebda82eac03e00112fd6a5710682767cff5361f7e819e53d8d3c3a2922e0d837aa620000000000000000548d8729f730589e579b0475a582c1608138ddf7054b73b5381c7e883e2efae",
		hash:  "642f05c473c2cd79909f9a841e2f30a70bf89b18180af97353ba198789c2b963",
		ke:    "642f05c473c2cd79909f9a841e2f30a7",
		ka:    "0bf89b18180af97353ba198789c2b963",
		kcA:   "c6be376fc7cd1301fd0a13adf3e7bffd",
		kcB:   "b7243f4ae60440a49b3f8cab3c1fba07",
		confA: "47d29e6666af1b7dd450d571233085d7a9866e4d49d2645e2df975489521232b",
		confB: "3313c5cefc361d27fb16847a91c2a73b766ffa90a4839122a9b70a2f6bd1d6df",
	},
	{
		idA: "server", idB: "",
		w:     "626e0cdc7b14c9db3e52a0b1b3a768c98e37852d5db30febe0497b14eae8c254",
		x:     "07adb3db6bc623d3399726bfdbfd3d15a58ea776ab8a308b00392621291f9633",
		pA:    "04f88fb71c99bfffaea370966b7eb99cd4be0ff1a7d335caac4211c4afd855e2e15a873b298503ad8ba1d9cbb9a392d2ba309b48bfd7879aefd0f2cea6009763b0",
		y:     "b6a4fc8dbb629d4ba51d6f91ed1532cf87adec98f25dd153a75accafafedec16",
		pB:    "040c269d6be017dccb15182ac6bfcd9e2a14de019dd587eaf4bdfd353f031101e7cca177f8eb362a6e83e7d5e729c0732e1b528879c086f39ba0f31a9661bd34db",
		k:     "0445ee233b8ecb51ebd6e7da3f307e88a1616bae2166121221fdc0dadb986afaf3ec8a988dc9c626fa3b99f58a7ca7c9b844bb3e8dd9554aafc5b53813504c1cbe",
		tt:    "06000000000000007365727665720000000000000000410000000000000004f88fb71c99bfffaea370966b7eb99cd4be0ff1a7d335caac4211c4afd855e2e15a873b298503ad8ba1d9cbb9a392d2ba309b48bfd7879aefd0f2cea6009763b04100000000000000040c269d6be017dccb15182ac6bfcd9e2a14de019dd587eaf4bdfd353f031101e7cca177f8eb362a6e83e7d5e729c0732e1b528879c086f39ba0f31a9661bd34db41000000000000000445ee233b8ecb51ebd6e7da3f307e88a1616bae2166121221fdc0dadb986afaf3ec8a988dc9c626fa3b99f58a7ca7c9b844bb3e8dd9554aafc5b53813504c1cbe2000000000000000626e0cdc7b14c9db3e52a0b1b3a768c98e37852d5db30febe0497b14eae8c254",
		hash:  "005184ff460da2ce59062c87733c299c3521297d736598fc0a1127600efa1afb",
		ke:    "005184ff460da2ce59062c87733c299c",
		ka:    "3521297d736598fc0a1127600efa1afb",
		kcA:   "f3da53604f0aeecea5a33be7bddf6edf",
		kcB:   "9e3f86848736f159bd92b6e107ec6799",
		confA: "bc9f9bbe99f26d0b2260e6456e05a86196a3307ec6663a18bf6ac825736533b2",
		confB: "c2370e1bf813b086dff0d834e74425a06e6390f48f5411900276dcccc5a297ec",
	},
	{
		idA: "", idB: "",
		w:     "7bf46c454b4c1b25799527d896508afd5fc62ef4ec59db1efb49113063d70cca",
		x:     "8cef65df64bb2d0f83540c53632de911b5b24b3eab6cc74a97609fd659e95473",
		pA:    "04a65b367a3f613cf9f0654b1b28a1e3a8a40387956c8ba6063e8658563890f46ca1ef6a676598889fc28de2950ab8120b79a5ef1ea4c9f44bc98f585634b46d66",
		y:     "d7a66f64074a84652d8d623a92e20c9675c61cb5b4f6a0063e4648a2fdc02d53",
		pB:    "04589f13218822710d98d8b2123a079041052d9941b9cf88c6617ddb2fcc0494662eea8ba6b64692dc318250030c6af045cb738bc81ba35b043c3dcb46adf6f58d",
		k:     "041a3c03d51b452537ca2a1fea6110353c6d5ed483c4f0f86f4492ca3f378d40a994b4477f93c64d928edbbcd3e85a7c709b7ea73ee97986ce3d1438e135543772",
		tt:    "00000000000000000000000000000000410000000000000004a65b367a3f613cf9f0654b1b28a1e3a8a40387956c8ba6063e8658563890f46ca1ef6a676598889fc28de2950ab8120b79a5ef1ea4c9f44bc98f585634b46d66410000000000000004589f13218822710d98d8b2123a079041052d9941b9cf88c6617ddb2fcc0494662eea8ba6b64692dc318250030c6af045cb738bc81ba35b043c3dcb46adf6f58d4100000000000000041a3c03d51b452537ca2a1fea6110353c6d5ed483c4f0f86f4492ca3f378d40a994b4477f93c64d928edbbcd3e85a7c709b7ea73ee97986ce3d1438e13554377220000000000000007bf46c454b4c1b25799527d896508afd5fc62ef4ec59db1efb49113063d70cca",
		hash:  "fc6374762ba5cf11f4b2caa08b2cd1b9907ae0e26e8d6234318d91583cd74c86",
		ke:    "fc6374762ba5cf11f4b2caa08b2cd1b9",
		ka:    "907ae0e26e8d6234318d91583cd74c86",
		kcA:   "5dbd2f477166b7fb6d61febbd77a5563",
		kcB:   "7689b4654407a5faeffdc8f18359d8a3",
		confA: "dfb4db8d48ae5a675963ea5e6c19d98d4ea028d8e898dad96ea19a80ade95dca",
		confB: "d0f0609d1613138d354f7e95f19fb556bf52d751947241e8c7118df5ef0ae175",
	},
}

func mustHex(t testing.TB, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("decode hex %q: %v", s, err)
	}
	return b
}

func mustScalar(t testing.TB, s string) [scalarLen]byte {
	t.Helper()
	b := mustHex(t, s)
	if len(b) != scalarLen {
		t.Fatalf("scalar %q has %d bytes", s, len(b))
	}
	return [scalarLen]byte(b)
}

func assertHex(t testing.TB, name string, got []byte, want string) {
	t.Helper()
	if !bytes.Equal(got, mustHex(t, want)) {
		t.Errorf("%s mismatch\n got  %x\n want %s", name, got, want)
	}
}

func orderBig(t testing.TB) *big.Int {
	t.Helper()
	n, ok := new(big.Int).SetString(orderHex, 16)
	if !ok {
		t.Fatal("parse group order")
	}
	return n
}

func TestRFC9382_VectorsP256SHA256(t *testing.T) {
	if len(rfcVectorsP256SHA256) != 4 {
		t.Fatalf("expected the 4 vectors of RFC 9382 Appendix B, have %d", len(rfcVectorsP256SHA256))
	}
	for _, v := range rfcVectorsP256SHA256 {
		t.Run(fmt.Sprintf("A=%q,B=%q", v.idA, v.idB), func(t *testing.T) {
			w, x, y := mustScalar(t, v.w), mustScalar(t, v.x), mustScalar(t, v.y)
			newPair := func() (a, b *Exchange) {
				t.Helper()
				a, err := newExchange(RoleA, w, x, []byte(v.idA), []byte(v.idB), nil)
				if err != nil {
					t.Fatalf("role A: %v", err)
				}
				b, err = newExchange(RoleB, w, y, []byte(v.idA), []byte(v.idB), nil)
				if err != nil {
					t.Fatalf("role B: %v", err)
				}
				return a, b
			}

			a, b := newPair()
			assertHex(t, "pA", a.Share(), v.pA)
			assertHex(t, "pB", b.Share(), v.pB)

			dA, err := a.derive(mustHex(t, v.pB))
			if err != nil {
				t.Fatalf("A derive: %v", err)
			}
			dB, err := b.derive(mustHex(t, v.pA))
			if err != nil {
				t.Fatalf("B derive: %v", err)
			}
			for side, d := range map[string]*derivation{"A": dA, "B": dB} {
				assertHex(t, side+" K", d.k, v.k)
				assertHex(t, side+" TT", d.tt, v.tt)
				assertHex(t, side+" Hash(TT)", d.hash[:], v.hash)
				assertHex(t, side+" Ke", d.ke, v.ke)
				assertHex(t, side+" Ka", d.ka, v.ka)
				assertHex(t, side+" KcA", d.kcA, v.kcA)
				assertHex(t, side+" KcB", d.kcB, v.kcB)
			}

			a, b = newPair()
			confA, err := a.Finish(mustHex(t, v.pB))
			if err != nil {
				t.Fatalf("A finish: %v", err)
			}
			confB, err := b.Finish(mustHex(t, v.pA))
			if err != nil {
				t.Fatalf("B finish: %v", err)
			}
			assertHex(t, "A conf", confA, v.confA)
			assertHex(t, "B conf", confB, v.confB)

			keyB, err := b.Verify(confA)
			if err != nil {
				t.Fatalf("B verify cA: %v", err)
			}
			keyA, err := a.Verify(confB)
			if err != nil {
				t.Fatalf("A verify cB: %v", err)
			}
			assertHex(t, "A output Ke", keyA, v.ke)
			assertHex(t, "B output Ke", keyB, v.ke)
		})
	}
}

func TestConstants_MatchRFCAndAppendixA(t *testing.T) {
	for _, c := range []struct {
		name, compressed, seed string
	}{
		{"M", mCompressed, "1.2.840.10045.3.1.7 point generation seed (M)"},
		{"N", nCompressed, "1.2.840.10045.3.1.7 point generation seed (N)"},
	} {
		t.Run(c.name, func(t *testing.T) {
			p, err := decodeCompressed(c.compressed)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if p.IsInfinity() == 1 {
				t.Fatal("constant is the identity")
			}
			if got := hex.EncodeToString(p.BytesCompressed()); got != c.compressed {
				t.Fatalf("re-encoding differs: %s", got)
			}
			if got := appendixAGeneratePoint(t, c.seed); got != c.compressed {
				t.Fatalf("RFC 9382 Appendix A generation yields %s, constant is %s", got, c.compressed)
			}
		})
	}
}

func appendixAGeneratePoint(t *testing.T, seed string) string {
	t.Helper()
	const encodedLen = 33
	iteratedHash := func(n int) []byte {
		h := []byte(seed)
		for range n {
			sum := sha256.Sum256(h)
			h = sum[:]
		}
		return h
	}
	for i := 1; i < 1000; i++ {
		var stream []byte
		for j := i; j < i+(encodedLen+31)/32; j++ {
			stream = append(stream, iteratedHash(j)...)
		}
		candidate := stream[:encodedLen]
		candidate[0] = candidate[0]&1 | 2
		p, err := new(nistec.P256Point).SetBytes(candidate)
		if err == nil && p.IsInfinity() == 0 {
			return hex.EncodeToString(candidate)
		}
	}
	t.Fatal("Appendix A generation found no point")
	return ""
}

func TestReduceWide_MatchesBigInt(t *testing.T) {
	n := orderBig(t)
	one := big.NewInt(1)
	var inputs [][]byte
	pad := func(v *big.Int, size int) []byte { return v.FillBytes(make([]byte, size)) }
	for _, v := range []*big.Int{
		big.NewInt(0), one, new(big.Int).Sub(n, one), n, new(big.Int).Add(n, one),
		new(big.Int).Lsh(n, 1), new(big.Int).Sub(new(big.Int).Lsh(one, 256), one),
		new(big.Int).Mul(n, new(big.Int).Lsh(one, 100)),
		new(big.Int).Sub(new(big.Int).Lsh(one, 384), one),
	} {
		inputs = append(inputs, pad(v, passwordWideLen))
	}
	for range 2000 {
		raw := make([]byte, passwordWideLen)
		if _, err := rand.Read(raw); err != nil {
			t.Fatal(err)
		}
		inputs = append(inputs, raw)
	}
	for _, in := range inputs {
		want := pad(new(big.Int).Mod(new(big.Int).SetBytes(in), n), scalarLen)
		got := reduceWide(in)
		if !bytes.Equal(got[:], want) {
			t.Fatalf("reduce %x\n got  %x\n want %x", in, got, want)
		}
	}
}

func TestScalarBounds(t *testing.T) {
	n := orderBig(t)
	one := big.NewInt(1)
	at := func(v *big.Int) [scalarLen]byte { return [scalarLen]byte(v.FillBytes(make([]byte, scalarLen))) }
	if !lessThanOrder(at(new(big.Int).Sub(n, one))) {
		t.Error("n-1 must be below the order")
	}
	if lessThanOrder(at(n)) {
		t.Error("n must not be below the order")
	}
	if lessThanOrder(at(new(big.Int).Add(n, one))) {
		t.Error("n+1 must not be below the order")
	}
	if lessThanOrder([scalarLen]byte(bytes.Repeat([]byte{0xff}, scalarLen))) {
		t.Error("2^256-1 must not be below the order")
	}
	if !isZero([scalarLen]byte{}) || isZero(at(one)) {
		t.Error("isZero misclassifies")
	}
}

func TestPasswordScalar_MatchesBigIntOracle(t *testing.T) {
	n := orderBig(t)
	for _, c := range []struct{ password, idA, idB string }{
		{testCode, testPeerIDA, testPeerIDB},
		{"000000", testPeerIDA, testPeerIDB},
		{"999999", "", ""},
		{testCode, "server", "client"},
	} {
		got, err := passwordScalar([]byte(c.password), []byte(c.idA), []byte(c.idB))
		if err != nil {
			t.Fatal(err)
		}
		wide, err := hkdf.Key(sha256.New, []byte(c.password), []byte("EdgeSync SPAKE2 w v1"), c.idA+"\x00"+c.idB, 48)
		if err != nil {
			t.Fatal(err)
		}
		want := new(big.Int).Mod(new(big.Int).SetBytes(wide), n).FillBytes(make([]byte, scalarLen))
		if !bytes.Equal(got[:], want) {
			t.Errorf("%+v: got %x want %x", c, got, want)
		}
	}
}

func TestPasswordScalar_DependsOnPasswordAndIdentities(t *testing.T) {
	base, err := passwordScalar([]byte(testCode), []byte(testPeerIDA), []byte(testPeerIDB))
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct{ password, idA, idB string }{
		"password":  {"483921", testPeerIDA, testPeerIDB},
		"idA":       {testCode, testPeerIDB, testPeerIDB},
		"idB":       {testCode, testPeerIDA, testPeerIDA},
		"id swap":   {testCode, testPeerIDB, testPeerIDA},
		"id merged": {testCode, testPeerIDA + testPeerIDB, ""},
	} {
		got, err := passwordScalar([]byte(c.password), []byte(c.idA), []byte(c.idB))
		if err != nil {
			t.Fatal(err)
		}
		if got == base {
			t.Errorf("%s did not change w", name)
		}
	}
}

func runExchange(t *testing.T, passwordA, passwordB, idA, idB string) (confA, confB []byte, a, b *Exchange) {
	t.Helper()
	a, err := New(RoleA, []byte(passwordA), []byte(idA), []byte(idB), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err = New(RoleB, []byte(passwordB), []byte(idA), []byte(idB), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if confA, err = a.Finish(b.Share()); err != nil {
		t.Fatalf("A finish: %v", err)
	}
	if confB, err = b.Finish(a.Share()); err != nil {
		t.Fatalf("B finish: %v", err)
	}
	return confA, confB, a, b
}

func TestSPAKE2_SameCodeAgrees(t *testing.T) {
	confA, confB, a, b := runExchange(t, testCode, testCode, testPeerIDA, testPeerIDB)
	if len(a.Share()) != shareLen || len(b.Share()) != shareLen {
		t.Fatalf("share lengths %d and %d", len(a.Share()), len(b.Share()))
	}
	if len(confA) != confirmLen || len(confB) != confirmLen {
		t.Fatalf("confirmation lengths %d and %d", len(confA), len(confB))
	}
	keyB, err := b.Verify(confA)
	if err != nil {
		t.Fatalf("B verify: %v", err)
	}
	keyA, err := a.Verify(confB)
	if err != nil {
		t.Fatalf("A verify: %v", err)
	}
	if !bytes.Equal(keyA, keyB) || len(keyA) != keyHalfLen {
		t.Fatalf("keys differ or wrong length: %x vs %x", keyA, keyB)
	}
}

func TestSPAKE2_WrongPasswordFailsConfirmation(t *testing.T) {
	confA, confB, a, b := runExchange(t, testCode, "483921", testPeerIDA, testPeerIDB)
	if key, err := b.Verify(confA); !errors.Is(err, ErrConfirmation) || key != nil {
		t.Errorf("B verify: key %x err %v", key, err)
	}
	if key, err := a.Verify(confB); !errors.Is(err, ErrConfirmation) || key != nil {
		t.Errorf("A verify: key %x err %v", key, err)
	}
}

func TestSPAKE2_DifferentIdentitiesFailConfirmation(t *testing.T) {
	a, err := New(RoleA, []byte(testCode), []byte(testPeerIDA), []byte(testPeerIDB), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := New(RoleB, []byte(testCode), []byte(testPeerIDA), []byte("cccccccccccccccccccccccccccccccccccccccccccccccccccc"), nil, nil)
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
	if _, err := b.Verify(confA); !errors.Is(err, ErrConfirmation) {
		t.Fatalf("verify: %v", err)
	}
}

func TestSPAKE2_ConfirmationIsNotReflectable(t *testing.T) {
	confA, confB, a, b := runExchange(t, testCode, testCode, testPeerIDA, testPeerIDB)
	if _, err := a.Verify(confA); !errors.Is(err, ErrConfirmation) {
		t.Errorf("A accepted its own confirmation: %v", err)
	}
	if _, err := b.Verify(confB); !errors.Is(err, ErrConfirmation) {
		t.Errorf("B accepted its own confirmation: %v", err)
	}
}

func TestSPAKE2_FreshEphemeralsPerRun(t *testing.T) {
	first, err := New(RoleA, []byte(testCode), []byte(testPeerIDA), []byte(testPeerIDB), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := New(RoleA, []byte(testCode), []byte(testPeerIDA), []byte(testPeerIDB), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first.Share(), second.Share()) {
		t.Fatal("two runs produced the same share")
	}
}

func rfcVectorExchange(t *testing.T, role Role) (*Exchange, rfcVector) {
	t.Helper()
	v := rfcVectorsP256SHA256[0]
	scalar := v.x
	if role == RoleB {
		scalar = v.y
	}
	e, err := newExchange(role, mustScalar(t, v.w), mustScalar(t, scalar), []byte(v.idA), []byte(v.idB), nil)
	if err != nil {
		t.Fatal(err)
	}
	return e, v
}

func TestSPAKE2_RejectsIdentityAndOffCurvePoints(t *testing.T) {
	v := rfcVectorsP256SHA256[0]
	validB := mustHex(t, v.pB)

	offCurve := bytes.Clone(validB)
	offCurve[len(offCurve)-1] ^= 1

	hybrid := bytes.Clone(validB)
	hybrid[0] = 6

	compressed := mustHex(t, mCompressed)

	fieldPrime := mustHex(t, "ffffffff00000001000000000000000000000000ffffffffffffffffffffffff")
	nonCanonical := append(append([]byte{4}, fieldPrime...), validB[33:]...)

	const wrongEncoding = "uncompressed SEC1"
	for _, c := range []struct {
		name  string
		share []byte
		cause string
	}{
		{"nil", nil, wrongEncoding},
		{"empty", []byte{}, wrongEncoding},
		{"identity one byte", []byte{0}, wrongEncoding},
		{"compressed encoding", compressed, wrongEncoding},
		{"hybrid prefix", hybrid, wrongEncoding},
		{"length 64", validB[:64], wrongEncoding},
		{"length 66", append(bytes.Clone(validB), 0), wrongEncoding},
		{"all 0xff", bytes.Repeat([]byte{0xff}, shareLen), wrongEncoding},
		{"all zero uncompressed", append([]byte{4}, make([]byte, 64)...), "not on curve"},
		{"off curve", offCurve, "not on curve"},
		{"non-canonical x", nonCanonical, "encoding"},
		{"reflected own share", mustHex(t, v.pA), "reflected"},
	} {
		t.Run(c.name, func(t *testing.T) {
			a, _ := rfcVectorExchange(t, RoleA)
			_, err := a.Finish(c.share)
			if !errors.Is(err, ErrInvalidShare) || !strings.Contains(err.Error(), c.cause) {
				t.Fatalf("Finish returned %v, want ErrInvalidShare mentioning %q", err, c.cause)
			}
			if _, err := a.Finish(validB); !errors.Is(err, ErrState) {
				t.Fatalf("exchange stayed usable after a rejected share: %v", err)
			}
			if _, err := a.Verify(make([]byte, confirmLen)); !errors.Is(err, ErrState) {
				t.Fatalf("Verify after rejected share: %v", err)
			}
		})
	}
}

func TestSPAKE2_RejectsIdentitySharedPoint(t *testing.T) {
	a, v := rfcVectorExchange(t, RoleA)
	_, n, err := maskPoints()
	if err != nil {
		t.Fatal(err)
	}
	guessed, err := new(nistec.P256Point).ScalarMult(n, mustHex(t, v.w))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Finish(guessed.Bytes()); !errors.Is(err, ErrInvalidShare) {
		t.Fatalf("Finish with pB = w*N returned %v", err)
	}
}

func TestExchange_StateMachine(t *testing.T) {
	v := rfcVectorsP256SHA256[0]

	t.Run("verify before finish", func(t *testing.T) {
		a, _ := rfcVectorExchange(t, RoleA)
		if _, err := a.Verify(mustHex(t, v.confB)); !errors.Is(err, ErrState) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("finish twice", func(t *testing.T) {
		a, _ := rfcVectorExchange(t, RoleA)
		if _, err := a.Finish(mustHex(t, v.pB)); err != nil {
			t.Fatal(err)
		}
		if _, err := a.Finish(mustHex(t, v.pB)); !errors.Is(err, ErrState) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("verify twice", func(t *testing.T) {
		a, _ := rfcVectorExchange(t, RoleA)
		if _, err := a.Finish(mustHex(t, v.pB)); err != nil {
			t.Fatal(err)
		}
		if _, err := a.Verify(mustHex(t, v.confB)); err != nil {
			t.Fatal(err)
		}
		if _, err := a.Verify(mustHex(t, v.confB)); !errors.Is(err, ErrState) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("failed verify is terminal", func(t *testing.T) {
		a, _ := rfcVectorExchange(t, RoleA)
		if _, err := a.Finish(mustHex(t, v.pB)); err != nil {
			t.Fatal(err)
		}
		if _, err := a.Verify(make([]byte, confirmLen)); !errors.Is(err, ErrConfirmation) {
			t.Fatalf("got %v", err)
		}
		if _, err := a.Verify(mustHex(t, v.confB)); !errors.Is(err, ErrState) {
			t.Fatalf("retry after failure: %v", err)
		}
	})
	for name, conf := range map[string][]byte{
		"nil confirmation":   nil,
		"short confirmation": mustHex(t, v.confB)[:confirmLen-1],
		"long confirmation":  append(mustHex(t, v.confB), 0),
		"flipped bit":        func() []byte { c := mustHex(t, v.confB); c[0] ^= 1; return c }(),
	} {
		t.Run(name, func(t *testing.T) {
			a, _ := rfcVectorExchange(t, RoleA)
			if _, err := a.Finish(mustHex(t, v.pB)); err != nil {
				t.Fatal(err)
			}
			if _, err := a.Verify(conf); !errors.Is(err, ErrConfirmation) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

type sequenceReader struct {
	chunks [][]byte
}

func (r *sequenceReader) Read(p []byte) (int, error) {
	if len(r.chunks) == 0 {
		return 0, io.ErrUnexpectedEOF
	}
	n := copy(p, r.chunks[0])
	r.chunks = r.chunks[1:]
	return n, nil
}

type repeatReader byte

func (r repeatReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(r)
	}
	return len(p), nil
}

func TestRandomScalar_RejectionSampling(t *testing.T) {
	order := mustHex(t, orderHex)
	valid := bytes.Repeat([]byte{0x11}, scalarLen)
	got, err := randomScalar(&sequenceReader{chunks: [][]byte{
		make([]byte, scalarLen), order, bytes.Repeat([]byte{0xff}, scalarLen), valid,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got[:], valid) {
		t.Fatalf("got %x", got)
	}
	if _, err := randomScalar(repeatReader(0)); !errors.Is(err, ErrRandomness) {
		t.Fatalf("all-zero source: %v", err)
	}
	if _, err := randomScalar(repeatReader(0xff)); !errors.Is(err, ErrRandomness) {
		t.Fatalf("all-ones source: %v", err)
	}
	if _, err := randomScalar(&sequenceReader{}); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("exhausted source: %v", err)
	}
}

func TestNew_RejectsInvalidParameters(t *testing.T) {
	idA, idB := []byte(testPeerIDA), []byte(testPeerIDB)
	if _, err := New(RoleA, nil, idA, idB, nil, nil); !errors.Is(err, ErrParameter) {
		t.Errorf("empty password: %v", err)
	}
	if _, err := New(Role(0), []byte(testCode), idA, idB, nil, nil); !errors.Is(err, ErrParameter) {
		t.Errorf("role 0: %v", err)
	}
	if _, err := New(RoleA, []byte(testCode), idA, idB, make([]byte, maxAADLen+1), nil); !errors.Is(err, ErrParameter) {
		t.Errorf("oversized aad: %v", err)
	}
	if _, err := New(RoleA, []byte(testCode), idA, idB, make([]byte, maxAADLen), nil); err != nil {
		t.Errorf("aad at the limit: %v", err)
	}
	if _, err := New(RoleA, []byte(testCode), idA, idB, nil, repeatReader(0)); !errors.Is(err, ErrRandomness) {
		t.Errorf("zero randomness: %v", err)
	}

	w := mustScalar(t, rfcVectorsP256SHA256[0].w)
	x := mustScalar(t, rfcVectorsP256SHA256[0].x)
	order := [scalarLen]byte(mustHex(t, orderHex))
	for name, c := range map[string]struct{ w, x [scalarLen]byte }{
		"zero w":     {[scalarLen]byte{}, x},
		"w equals n": {order, x},
		"zero x":     {w, [scalarLen]byte{}},
		"x equals n": {w, order},
	} {
		if _, err := newExchange(RoleA, c.w, c.x, idA, idB, nil); !errors.Is(err, ErrParameter) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
