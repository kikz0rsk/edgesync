package tap

import (
	"fmt"

	"github.com/kikz0r/edgesync/spikes/macos/fw"
)

type State struct {
	AccessibilityTrusted bool
	ListenAccess         bool
	PostAccess           bool
}

func (s State) String() string {
	return fmt.Sprintf("AXIsProcessTrusted=%t CGPreflightListenEventAccess=%t CGPreflightPostEventAccess=%t",
		s.AccessibilityTrusted, s.ListenAccess, s.PostAccess)
}

type Permissions struct {
	axIsProcessTrusted            func() bool
	axIsProcessTrustedWithOptions func(options uintptr) bool
	preflightListenEventAccess    func() bool
	requestListenEventAccess      func() bool
	preflightPostEventAccess      func() bool
	requestPostEventAccess        func() bool
	dictionaryCreate              func(allocator uintptr, keys, values *uintptr, count int, keyCallbacks, valueCallbacks uintptr) uintptr
	release                       func(ref uintptr)

	promptKey      uintptr
	trueValue      uintptr
	keyCallbacks   uintptr
	valueCallbacks uintptr
}

func NewPermissions(lib *fw.Lib) (*Permissions, error) {
	p := &Permissions{release: lib.CoreFoundation.Release}

	services := symbols{handle: lib.Handles.ApplicationServices}
	services.bind(&p.axIsProcessTrusted, "AXIsProcessTrusted")
	services.bind(&p.axIsProcessTrustedWithOptions, "AXIsProcessTrustedWithOptions")
	p.promptKey = services.pointerVariable("kAXTrustedCheckOptionPrompt")
	if services.err != nil {
		return nil, services.err
	}

	graphics := symbols{handle: lib.Handles.CoreGraphics}
	graphics.bind(&p.preflightListenEventAccess, "CGPreflightListenEventAccess")
	graphics.bind(&p.requestListenEventAccess, "CGRequestListenEventAccess")
	graphics.bind(&p.preflightPostEventAccess, "CGPreflightPostEventAccess")
	graphics.bind(&p.requestPostEventAccess, "CGRequestPostEventAccess")
	if graphics.err != nil {
		return nil, graphics.err
	}

	foundation := symbols{handle: lib.Handles.CoreFoundation}
	foundation.bind(&p.dictionaryCreate, "CFDictionaryCreate")
	p.trueValue = foundation.pointerVariable("kCFBooleanTrue")
	p.keyCallbacks = foundation.address("kCFTypeDictionaryKeyCallBacks")
	p.valueCallbacks = foundation.address("kCFTypeDictionaryValueCallBacks")
	if foundation.err != nil {
		return nil, foundation.err
	}
	return p, nil
}

func (p *Permissions) Preflight() State {
	return State{
		AccessibilityTrusted: p.AccessibilityTrusted(),
		ListenAccess:         p.ListenAccess(),
		PostAccess:           p.PostAccess(),
	}
}

func (p *Permissions) AccessibilityTrustedLegacy() bool {
	return p.axIsProcessTrusted()
}

// AccessibilityTrusted calls AXIsProcessTrustedWithOptions without options, so it never prompts.
func (p *Permissions) AccessibilityTrusted() bool {
	return p.axIsProcessTrustedWithOptions(0)
}

// AccessibilityTrustedPrompting calls AXIsProcessTrustedWithOptions with kAXTrustedCheckOptionPrompt set to true.
func (p *Permissions) AccessibilityTrustedPrompting() (bool, error) {
	keys := [1]uintptr{p.promptKey}
	values := [1]uintptr{p.trueValue}
	options := p.dictionaryCreate(0, &keys[0], &values[0], 1, p.keyCallbacks, p.valueCallbacks)
	if options == 0 {
		return false, ErrDictionary
	}
	defer p.release(options)
	return p.axIsProcessTrustedWithOptions(options), nil
}

func (p *Permissions) ListenAccess() bool {
	return p.preflightListenEventAccess()
}

func (p *Permissions) RequestListenAccess() bool {
	return p.requestListenEventAccess()
}

func (p *Permissions) PostAccess() bool {
	return p.preflightPostEventAccess()
}

func (p *Permissions) RequestPostAccess() bool {
	return p.requestPostEventAccess()
}
