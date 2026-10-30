// Package spake2 implements SPAKE2 (RFC 9382) for the ciphersuite SPAKE2-P256-SHA256-HKDF-HMAC on
// filippo.io/nistec. It is a Phase 0 feasibility spike, not an audited library.
package spake2

import (
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/bits"

	"filippo.io/nistec"
)

type Role uint8

const (
	RoleA Role = 1
	RoleB Role = 2
)

var (
	ErrInvalidShare = errors.New("spake2: invalid peer share")
	ErrConfirmation = errors.New("spake2: key confirmation failed")
	ErrState        = errors.New("spake2: operation not allowed in the current state")
	ErrParameter    = errors.New("spake2: invalid parameter")
	ErrRandomness   = errors.New("spake2: random source did not yield a valid scalar")
)

const (
	scalarLen       = 32
	shareLen        = 1 + 2*scalarLen
	keyHalfLen      = sha256.Size / 2
	confirmLen      = sha256.Size
	passwordWideLen = scalarLen + 16
	maxAADLen       = (1<<16 - 128) / 8
	maxScalarTries  = 64

	// RFC 9382 section 6, P-256 entries, compressed SEC1.
	mCompressed = "02886e2f97ace46e55ba9dd7242579f2993b64e16ef3dcab95afd497333d8fa12f"
	nCompressed = "03d8bbd6c639c62937b04d997f38c3770719c629d7014d49a24b4f98baa1292b49"

	passwordSalt  = "EdgeSync SPAKE2 w v1"
	confirmPrefix = "ConfirmationKeys"
)

type limbs [4]uint64

// Order of the P-256 group, little-endian 64-bit limbs.
var orderLimbs = limbs{0xf3b9cac2fc632551, 0xbce6faada7179e84, 0xffffffffffffffff, 0xffffffff00000000}

type exchangeState uint8

const (
	stateReady exchangeState = iota
	stateFinished
	stateDone
)

type Exchange struct {
	role        Role
	state       exchangeState
	idA, idB    []byte
	aad         []byte
	w           [scalarLen]byte
	x           [scalarLen]byte
	own         *nistec.P256Point
	peerMask    *nistec.P256Point
	share       []byte
	ke          [keyHalfLen]byte
	peerConfirm [confirmLen]byte
}

type derivation struct {
	k    []byte
	tt   []byte
	hash [sha256.Size]byte
	ke   []byte
	ka   []byte
	kcA  []byte
	kcB  []byte
}

func New(role Role, password, idA, idB, aad []byte, rnd io.Reader) (*Exchange, error) {
	if len(password) == 0 {
		return nil, fmt.Errorf("%w: empty password", ErrParameter)
	}
	if rnd == nil {
		rnd = rand.Reader
	}
	w, err := passwordScalar(password, idA, idB)
	if err != nil {
		return nil, err
	}
	x, err := randomScalar(rnd)
	if err != nil {
		return nil, err
	}
	return newExchange(role, w, x, idA, idB, aad)
}

func newExchange(role Role, w, x [scalarLen]byte, idA, idB, aad []byte) (*Exchange, error) {
	if role != RoleA && role != RoleB {
		return nil, fmt.Errorf("%w: unknown role %d", ErrParameter, role)
	}
	if len(aad) > maxAADLen {
		return nil, fmt.Errorf("%w: aad longer than %d bytes", ErrParameter, maxAADLen)
	}
	if !lessThanOrder(w) || isZero(w) {
		return nil, fmt.Errorf("%w: w outside [1, n-1]", ErrParameter)
	}
	if !lessThanOrder(x) || isZero(x) {
		return nil, fmt.Errorf("%w: ephemeral scalar outside [1, n-1]", ErrParameter)
	}
	m, n, err := maskPoints()
	if err != nil {
		return nil, err
	}
	own, peerMask := m, n
	if role == RoleB {
		own, peerMask = n, m
	}

	base, err := new(nistec.P256Point).ScalarBaseMult(x[:])
	if err != nil {
		return nil, fmt.Errorf("spake2: x*P: %w", err)
	}
	mask, err := new(nistec.P256Point).ScalarMult(own, w[:])
	if err != nil {
		return nil, fmt.Errorf("spake2: w*mask: %w", err)
	}
	share := base.Add(base, mask).Bytes()

	return &Exchange{
		role:     role,
		idA:      append([]byte(nil), idA...),
		idB:      append([]byte(nil), idB...),
		aad:      append([]byte(nil), aad...),
		w:        w,
		x:        x,
		own:      own,
		peerMask: peerMask,
		share:    share,
	}, nil
}

func (e *Exchange) Share() []byte {
	return append([]byte(nil), e.share...)
}

func (e *Exchange) Finish(peerShare []byte) ([]byte, error) {
	if e.state != stateReady {
		return nil, ErrState
	}
	e.state = stateDone
	d, err := e.derive(peerShare)
	if err != nil {
		return nil, err
	}
	kcOwn, kcPeer := d.kcA, d.kcB
	if e.role == RoleB {
		kcOwn, kcPeer = d.kcB, d.kcA
	}
	copy(e.peerConfirm[:], confirmationMAC(kcPeer, d.tt))
	copy(e.ke[:], d.ke)
	clear(e.x[:])
	clear(e.w[:])
	e.state = stateFinished
	return confirmationMAC(kcOwn, d.tt), nil
}

func (e *Exchange) Verify(peerConfirm []byte) ([]byte, error) {
	if e.state != stateFinished {
		return nil, ErrState
	}
	e.state = stateDone
	if !hmac.Equal(e.peerConfirm[:], peerConfirm) {
		return nil, ErrConfirmation
	}
	return append([]byte(nil), e.ke[:]...), nil
}

func (e *Exchange) derive(peerShare []byte) (*derivation, error) {
	peer, err := e.parsePeerShare(peerShare)
	if err != nil {
		return nil, err
	}

	// P-256 has cofactor h = 1, so the RFC 9382 section 3.3 multiplication by h is the identity
	// map and a point that passes the on-curve check is already in the prime-order group.
	wp, err := new(nistec.P256Point).ScalarMult(e.peerMask, e.w[:])
	if err != nil {
		return nil, fmt.Errorf("spake2: w*peerMask: %w", err)
	}
	unmasked := new(nistec.P256Point).Negate(wp)
	unmasked.Add(unmasked, peer)
	k, err := new(nistec.P256Point).ScalarMult(unmasked, e.x[:])
	if err != nil {
		return nil, fmt.Errorf("spake2: x*(peer-w*mask): %w", err)
	}
	// K is the identity only if the peer share equals w*mask, which a peer can arrange only by
	// guessing w; the identity also has a one-byte encoding that would corrupt the TT layout.
	if k.IsInfinity() == 1 {
		return nil, fmt.Errorf("%w: shared point is the identity", ErrInvalidShare)
	}

	pA, pB := e.share, peerShare
	if e.role == RoleB {
		pA, pB = peerShare, e.share
	}
	d := &derivation{k: k.Bytes()}
	d.tt = transcript(e.idA, e.idB, pA, pB, d.k, e.w[:])
	d.hash = sha256.Sum256(d.tt)
	d.ke, d.ka = d.hash[:keyHalfLen], d.hash[keyHalfLen:]

	confirmKeys, err := hkdf.Key(sha256.New, d.ka, nil, confirmPrefix+string(e.aad), 2*keyHalfLen)
	if err != nil {
		return nil, fmt.Errorf("spake2: derive confirmation keys: %w", err)
	}
	d.kcA, d.kcB = confirmKeys[:keyHalfLen], confirmKeys[keyHalfLen:]
	return d, nil
}

func (e *Exchange) parsePeerShare(b []byte) (*nistec.P256Point, error) {
	// SetBytes would also accept the one-byte identity and the compressed encodings; the exact length
	// and prefix check rejects them first, as RFC 9382 section 6 mandates uncompressed SEC1 for P-256.
	if len(b) != shareLen || b[0] != 4 {
		return nil, fmt.Errorf("%w: not a %d-byte uncompressed SEC1 point", ErrInvalidShare, shareLen)
	}
	p, err := new(nistec.P256Point).SetBytes(b)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidShare, err)
	}
	if subtle.ConstantTimeCompare(b, e.share) == 1 {
		return nil, fmt.Errorf("%w: reflected own share", ErrInvalidShare)
	}
	return p, nil
}

func transcript(idA, idB, pA, pB, k, w []byte) []byte {
	var tt []byte
	for _, field := range [][]byte{idA, idB, pA, pB, k, w} {
		tt = binary.LittleEndian.AppendUint64(tt, uint64(len(field)))
		tt = append(tt, field...)
	}
	return tt
}

func confirmationMAC(key, tt []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(tt)
	return mac.Sum(nil)
}

func maskPoints() (m, n *nistec.P256Point, err error) {
	if m, err = decodeCompressed(mCompressed); err != nil {
		return nil, nil, fmt.Errorf("spake2: constant M: %w", err)
	}
	if n, err = decodeCompressed(nCompressed); err != nil {
		return nil, nil, fmt.Errorf("spake2: constant N: %w", err)
	}
	return m, n, nil
}

func decodeCompressed(h string) (*nistec.P256Point, error) {
	raw, err := hex.DecodeString(h)
	if err != nil {
		return nil, err
	}
	return new(nistec.P256Point).SetBytes(raw)
}

// passwordScalar maps the password to w as D-110 specifies: HKDF-SHA256 with salt "EdgeSync SPAKE2 w v1"
// and info idA || 0x00 || idB, 48 bytes, reduced mod n. RFC 9382 section 3.2 leaves this to the application.
func passwordScalar(password, idA, idB []byte) ([scalarLen]byte, error) {
	info := string(idA) + "\x00" + string(idB)
	wide, err := hkdf.Key(sha256.New, password, []byte(passwordSalt), info, passwordWideLen)
	if err != nil {
		return [scalarLen]byte{}, fmt.Errorf("spake2: derive password scalar: %w", err)
	}
	return reduceWide(wide), nil
}

// reduceWide computes the big-endian integer in wide mod n bit by bit with fixed-width limb arithmetic.
// math/big would make the running time depend on the value of the password-derived scalar.
func reduceWide(wide []byte) [scalarLen]byte {
	var r limbs
	for _, b := range wide {
		for i := 7; i >= 0; i-- {
			carry := r[3] >> 63
			r[3] = r[3]<<1 | r[2]>>63
			r[2] = r[2]<<1 | r[1]>>63
			r[1] = r[1]<<1 | r[0]>>63
			r[0] = r[0]<<1 | uint64(b>>i)&1
			diff, borrow := subOrder(r, carry)
			keep := borrow - 1
			for j := range r {
				r[j] = r[j]&^keep | diff[j]&keep
			}
		}
	}
	return limbsToBytes(r)
}

// subOrder returns a - n and the final borrow, where a carries one extra most significant bit.
func subOrder(a limbs, carry uint64) (diff limbs, borrow uint64) {
	for i := range a {
		diff[i], borrow = bits.Sub64(a[i], orderLimbs[i], borrow)
	}
	_, borrow = bits.Sub64(carry, 0, borrow)
	return diff, borrow
}

func lessThanOrder(s [scalarLen]byte) bool {
	_, borrow := subOrder(bytesToLimbs(s), 0)
	return borrow == 1
}

func isZero(s [scalarLen]byte) bool {
	var zero [scalarLen]byte
	return subtle.ConstantTimeCompare(s[:], zero[:]) == 1
}

func bytesToLimbs(s [scalarLen]byte) limbs {
	var l limbs
	for i := range l {
		l[i] = binary.BigEndian.Uint64(s[scalarLen-8*(i+1) : scalarLen-8*i])
	}
	return l
}

func limbsToBytes(l limbs) [scalarLen]byte {
	var s [scalarLen]byte
	for i := range l {
		binary.BigEndian.PutUint64(s[scalarLen-8*(i+1):scalarLen-8*i], l[i])
	}
	return s
}

// randomScalar rejection-samples a uniform scalar in [1, n-1] as RFC 9382 section 7 recommends. The rejected
// candidates are discarded, so the branch only reveals values that are never used.
func randomScalar(rnd io.Reader) ([scalarLen]byte, error) {
	var s [scalarLen]byte
	for range maxScalarTries {
		if _, err := io.ReadFull(rnd, s[:]); err != nil {
			return [scalarLen]byte{}, fmt.Errorf("spake2: read randomness: %w", err)
		}
		if lessThanOrder(s) && !isZero(s) {
			return s, nil
		}
	}
	return [scalarLen]byte{}, ErrRandomness
}
