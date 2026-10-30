package dnd

import (
	"errors"
	"fmt"
)

const ProtocolVersion = 5

var (
	ErrVersion    = errors.New("dnd: XDND version above 5")
	ErrNoTypes    = errors.New("dnd: the drag offers no data type")
	ErrCoordinate = errors.New("dnd: coordinate outside 0..65535")
)

const (
	maxInlineTypes = 3
	maxCoordinate  = 0xFFFF

	enterMoreTypesFlag     = 1 << 0
	statusAcceptFlag       = 1 << 0
	statusWantPositionFlag = 1 << 1
	finishedAcceptedFlag   = 1 << 0
	versionShift           = 24
	positionStateMask      = 0xFF
)

// Atoms holds the interned atom ids the protocol uses. A zero atom is None.
type Atoms struct {
	Aware         uint32
	Proxy         uint32
	Selection     uint32
	TypeList      uint32
	Enter         uint32
	Position      uint32
	Status        uint32
	Leave         uint32
	Drop          uint32
	Finished      uint32
	ActionCopy    uint32
	ActionMove    uint32
	ActionLink    uint32
	ActionAsk     uint32
	ActionPrivate uint32
	URIList       uint32
	Targets       uint32
}

func (a *Atoms) fields() []*uint32 {
	return []*uint32{
		&a.Aware, &a.Proxy, &a.Selection, &a.TypeList, &a.Enter, &a.Position, &a.Status, &a.Leave, &a.Drop,
		&a.Finished, &a.ActionCopy, &a.ActionMove, &a.ActionLink, &a.ActionAsk, &a.ActionPrivate, &a.URIList, &a.Targets,
	}
}

var atomNames = []string{
	"XdndAware", "XdndProxy", "XdndSelection", "XdndTypeList", "XdndEnter", "XdndPosition", "XdndStatus", "XdndLeave",
	"XdndDrop", "XdndFinished", "XdndActionCopy", "XdndActionMove", "XdndActionLink", "XdndActionAsk",
	"XdndActionPrivate", "text/uri-list", "TARGETS",
}

// NewAtoms interns every atom name the protocol needs through intern.
func NewAtoms(intern func(name string) (uint32, error)) (Atoms, error) {
	var a Atoms
	for i, field := range a.fields() {
		id, err := intern(atomNames[i])
		if err != nil {
			return Atoms{}, fmt.Errorf("dnd: intern %s: %w", atomNames[i], err)
		}
		*field = id
	}
	return a, nil
}

type Kind uint8

const (
	KindEnter Kind = iota + 1
	KindPosition
	KindStatus
	KindLeave
	KindDrop
	KindFinished
)

func (k Kind) String() string {
	switch k {
	case KindEnter:
		return "XdndEnter"
	case KindPosition:
		return "XdndPosition"
	case KindStatus:
		return "XdndStatus"
	case KindLeave:
		return "XdndLeave"
	case KindDrop:
		return "XdndDrop"
	case KindFinished:
		return "XdndFinished"
	}
	return fmt.Sprintf("Kind(%d)", uint8(k))
}

// Kind classifies the message_type atom of a ClientMessage.
func (a Atoms) Kind(messageType uint32) (Kind, bool) {
	switch messageType {
	case a.Enter:
		return KindEnter, true
	case a.Position:
		return KindPosition, true
	case a.Status:
		return KindStatus, true
	case a.Leave:
		return KindLeave, true
	case a.Drop:
		return KindDrop, true
	case a.Finished:
		return KindFinished, true
	}
	return 0, false
}

// MessageType is the message_type atom of a kind.
func (a Atoms) MessageType(k Kind) uint32 {
	switch k {
	case KindEnter:
		return a.Enter
	case KindPosition:
		return a.Position
	case KindStatus:
		return a.Status
	case KindLeave:
		return a.Leave
	case KindDrop:
		return a.Drop
	case KindFinished:
		return a.Finished
	}
	return 0
}

// Data is the five 32-bit words of an XDND ClientMessage (format 32).
type Data [5]uint32

// NegotiateVersion is the protocol version a drag uses: the lower of the source's and the target's highest version.
func NegotiateVersion(source, target uint8) uint8 {
	return min(source, target, ProtocolVersion)
}

// EnterMsg: data.l[0] source window, data.l[1] bit 0 "more than three types" and the version in the top byte,
// data.l[2..4] the first three types. Types beyond three travel in the XdndTypeList property of the source window.
type EnterMsg struct {
	Source           uint32
	Version          uint8
	Types            []uint32
	TypeListRequired bool
}

func (e EnterMsg) Encode() (Data, error) {
	if len(e.Types) == 0 {
		return Data{}, ErrNoTypes
	}
	if e.Version > ProtocolVersion {
		return Data{}, fmt.Errorf("%w: %d", ErrVersion, e.Version)
	}
	d := Data{e.Source, uint32(e.Version) << versionShift}
	if len(e.Types) > maxInlineTypes {
		d[1] |= enterMoreTypesFlag
	}
	for i := range min(len(e.Types), maxInlineTypes) {
		d[2+i] = e.Types[i]
	}
	return d, nil
}

func DecodeEnter(d Data) (EnterMsg, error) {
	version := d[1] >> versionShift
	if version > ProtocolVersion {
		return EnterMsg{}, fmt.Errorf("%w: %d", ErrVersion, version)
	}
	e := EnterMsg{Source: d[0], Version: uint8(version), TypeListRequired: d[1]&enterMoreTypesFlag != 0}
	for _, atom := range d[2:] {
		if atom != 0 {
			e.Types = append(e.Types, atom)
		}
	}
	return e, nil
}

// PositionMsg: data.l[1] key state in bits 0-7, data.l[2] root x in the high and y in the low 16 bits, data.l[3] the
// timestamp (version 1 and above), data.l[4] the requested action (version 2 and above).
type PositionMsg struct {
	Source uint32
	State  uint16
	X, Y   int
	Time   uint32
	Action uint32
}

func (p PositionMsg) Encode() (Data, error) {
	packed, err := packPair(p.X, p.Y)
	if err != nil {
		return Data{}, err
	}
	return Data{p.Source, uint32(p.State) & positionStateMask, packed, p.Time, p.Action}, nil
}

func DecodePosition(d Data) PositionMsg {
	x, y := unpackPair(d[2])
	return PositionMsg{Source: d[0], State: uint16(d[1] & positionStateMask), X: x, Y: y, Time: d[3], Action: d[4]}
}

type Rect struct {
	X, Y, W, H int
}

func (r Rect) Empty() bool {
	return r.W == 0 || r.H == 0
}

func (r Rect) Contains(x, y int) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

// StatusMsg: data.l[1] bit 0 accept and bit 1 "keep sending positions inside the rectangle", data.l[2] and
// data.l[3] the rectangle in root coordinates packed like a position and as width and height, data.l[4] the action
// the target will perform (None means no drop).
type StatusMsg struct {
	Target        uint32
	Accept        bool
	WantPositions bool
	Rect          Rect
	Action        uint32
}

func (s StatusMsg) Encode() (Data, error) {
	origin, err := packPair(s.Rect.X, s.Rect.Y)
	if err != nil {
		return Data{}, err
	}
	size, err := packPair(s.Rect.W, s.Rect.H)
	if err != nil {
		return Data{}, err
	}
	var flags uint32
	if s.Accept {
		flags |= statusAcceptFlag
	}
	if s.WantPositions {
		flags |= statusWantPositionFlag
	}
	return Data{s.Target, flags, origin, size, s.Action}, nil
}

func DecodeStatus(d Data) StatusMsg {
	x, y := unpackPair(d[2])
	w, h := unpackPair(d[3])
	return StatusMsg{
		Target:        d[0],
		Accept:        d[1]&statusAcceptFlag != 0,
		WantPositions: d[1]&statusWantPositionFlag != 0,
		Rect:          Rect{X: x, Y: y, W: w, H: h},
		Action:        d[4],
	}
}

type LeaveMsg struct {
	Source uint32
}

func (l LeaveMsg) Encode() Data {
	return Data{l.Source}
}

func DecodeLeave(d Data) LeaveMsg {
	return LeaveMsg{Source: d[0]}
}

// DropMsg: data.l[2] is the timestamp used to retrieve the data (version 1 and above).
type DropMsg struct {
	Source uint32
	Time   uint32
}

func (m DropMsg) Encode() Data {
	return Data{m.Source, 0, m.Time}
}

func DecodeDrop(d Data) DropMsg {
	return DropMsg{Source: d[0], Time: d[2]}
}

// FinishedMsg: data.l[1] bit 0 "drop accepted and action performed" and data.l[2] the performed action (both
// version 5 and above; a source below version 5 must treat the drop as accepted).
type FinishedMsg struct {
	Target   uint32
	Accepted bool
	Action   uint32
}

func (f FinishedMsg) Encode() Data {
	d := Data{f.Target, 0, f.Action}
	if f.Accepted {
		d[1] |= finishedAcceptedFlag
	}
	return d
}

func DecodeFinished(d Data) FinishedMsg {
	return FinishedMsg{Target: d[0], Accepted: d[1]&finishedAcceptedFlag != 0, Action: d[2]}
}

func packPair(hi, lo int) (uint32, error) {
	if hi < 0 || hi > maxCoordinate || lo < 0 || lo > maxCoordinate {
		return 0, fmt.Errorf("%w: %d,%d", ErrCoordinate, hi, lo)
	}
	return uint32(hi)<<16 | uint32(lo), nil
}

func unpackPair(v uint32) (hi, lo int) {
	return int(v >> 16), int(v & 0xFFFF)
}
