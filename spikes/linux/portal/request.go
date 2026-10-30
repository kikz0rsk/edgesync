// Package portal drives the xdg-desktop-portal InputCapture and RemoteDesktop interfaces over D-Bus for the
// EdgeSync Wayland spike. All D-Bus traffic goes through the Bus interface, so the Request/Response and session
// state machines run against a fake bus on the host; SessionBus is the godbus implementation used on Linux.
// See README.md for the portal documentation version every method name, signature and option key was checked against.
package portal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"

	"github.com/godbus/dbus/v5"
)

const (
	Destination = "org.freedesktop.portal.Desktop"
	DesktopPath = dbus.ObjectPath("/org/freedesktop/portal/desktop")

	interfaceRequest = "org.freedesktop.portal.Request"
	interfaceSession = "org.freedesktop.portal.Session"

	methodRequestClose = interfaceRequest + ".Close"
	signalResponse     = "Response"
	optionHandleToken  = "handle_token"
	optionSessionToken = "session_handle_token"
)

var (
	ErrCancelled        = errors.New("portal: request cancelled by the user")
	ErrEnded            = errors.New("portal: request ended by the portal without success")
	ErrUnknownResponse  = errors.New("portal: unknown response code")
	ErrBadResponse      = errors.New("portal: malformed response")
	ErrHandleMismatch   = errors.New("portal: request handle differs from the expected path")
	ErrInvalidToken     = errors.New("portal: invalid token, only [A-Za-z0-9_] is allowed")
	ErrNoUniqueName     = errors.New("portal: bus connection has no unique name")
	ErrMissingResult    = errors.New("portal: result key missing")
	ErrResultType       = errors.New("portal: result has an unexpected type")
	ErrBusClosed        = errors.New("portal: bus connection closed")
	ErrNoUnixFDSupport  = errors.New("portal: bus connection cannot pass file descriptors")
	ErrUnexpectedReturn = errors.New("portal: unexpected method return")
)

type Signal struct {
	Path dbus.ObjectPath
	Name string
	Body []any
}

type SignalMatch struct {
	Path      dbus.ObjectPath
	Interface string
	Member    string
}

func (m SignalMatch) Matches(s Signal) bool {
	if m.Path != "" && m.Path != s.Path {
		return false
	}
	iface, member := splitSignalName(s.Name)
	if m.Interface != "" && m.Interface != iface {
		return false
	}
	return m.Member == "" || m.Member == member
}

func splitSignalName(name string) (iface, member string) {
	i := strings.LastIndexByte(name, '.')
	if i < 0 {
		return "", name
	}
	return name[:i], name[i+1:]
}

type Subscription interface {
	Signals() <-chan Signal
	Close() error
}

type Bus interface {
	UniqueName() string
	Call(ctx context.Context, path dbus.ObjectPath, method string, args ...any) ([]any, error)
	CallFD(ctx context.Context, path dbus.ObjectPath, method string, args ...any) (*os.File, error)
	Property(ctx context.Context, path dbus.ObjectPath, iface, name string) (dbus.Variant, error)
	Subscribe(ctx context.Context, match SignalMatch) (Subscription, error)
}

type Options = map[string]dbus.Variant

type TokenSource func() (string, error)

func RandomTokens(prefix string) TokenSource {
	var n atomic.Uint64
	return func() (string, error) {
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", fmt.Errorf("portal: token entropy: %w", err)
		}
		token := fmt.Sprintf("%s_%s_%d", prefix, hex.EncodeToString(b[:]), n.Add(1))
		if !validElement(token) {
			return "", fmt.Errorf("%w: %q", ErrInvalidToken, token)
		}
		return token, nil
	}
}

func validElement(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
		default:
			return false
		}
	}
	return true
}

func senderElement(uniqueName string) (string, error) {
	if uniqueName == "" {
		return "", ErrNoUniqueName
	}
	element := strings.ReplaceAll(strings.TrimPrefix(uniqueName, ":"), ".", "_")
	if !validElement(element) {
		return "", fmt.Errorf("%w: unique name %q", ErrInvalidToken, uniqueName)
	}
	return element, nil
}

func handlePath(kind, uniqueName, token string) (dbus.ObjectPath, error) {
	sender, err := senderElement(uniqueName)
	if err != nil {
		return "", err
	}
	if !validElement(token) {
		return "", fmt.Errorf("%w: %q", ErrInvalidToken, token)
	}
	return dbus.ObjectPath(string(DesktopPath) + "/" + kind + "/" + sender + "/" + token), nil
}

func RequestPath(uniqueName, token string) (dbus.ObjectPath, error) {
	return handlePath("request", uniqueName, token)
}

func SessionPath(uniqueName, token string) (dbus.ObjectPath, error) {
	return handlePath("session", uniqueName, token)
}

type Results map[string]dbus.Variant

type Response struct {
	Code    uint32
	Results Results
}

func DecodeResponse(body []any) (Response, error) {
	if len(body) != 2 {
		return Response{}, fmt.Errorf("%w: Response carries %d values, want 2", ErrBadResponse, len(body))
	}
	code, ok := body[0].(uint32)
	if !ok {
		return Response{}, fmt.Errorf("%w: response code is %T, want uint32", ErrBadResponse, body[0])
	}
	results, ok := body[1].(map[string]dbus.Variant)
	if !ok {
		return Response{}, fmt.Errorf("%w: results are %T, want a{sv}", ErrBadResponse, body[1])
	}
	return Response{Code: code, Results: results}, nil
}

type ResponseError struct {
	Method string
	Code   uint32
}

func (e *ResponseError) Error() string {
	return fmt.Sprintf("portal: %s answered with response code %d", e.Method, e.Code)
}

func (e *ResponseError) Unwrap() error {
	switch e.Code {
	case 1:
		return ErrCancelled
	case 2:
		return ErrEnded
	default:
		return ErrUnknownResponse
	}
}

func (r Response) Err(method string) error {
	if r.Code == 0 {
		return nil
	}
	return &ResponseError{Method: method, Code: r.Code}
}

func (r Results) Has(key string) bool {
	_, ok := r[key]
	return ok
}

func (r Results) value(key string) (any, error) {
	v, ok := r[key]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrMissingResult, key)
	}
	return v.Value(), nil
}

func typedResult[T any](r Results, key, want string) (T, error) {
	var zero T
	raw, err := r.value(key)
	if err != nil {
		return zero, err
	}
	v, ok := raw.(T)
	if !ok {
		return zero, fmt.Errorf("%w: %q is %T, want %s", ErrResultType, key, raw, want)
	}
	return v, nil
}

func (r Results) Uint32(key string) (uint32, error) {
	return typedResult[uint32](r, key, "uint32")
}

func (r Results) String(key string) (string, error) {
	return typedResult[string](r, key, "string")
}

func (r Results) Bool(key string) (bool, error) {
	return typedResult[bool](r, key, "bool")
}

func (r Results) ObjectPath(key string) (dbus.ObjectPath, error) {
	return typedResult[dbus.ObjectPath](r, key, "object path")
}

func (r Results) Uint32s(key string) ([]uint32, error) {
	return typedResult[[]uint32](r, key, "array of uint32")
}

func (r Results) PathOrString(key string) (dbus.ObjectPath, error) {
	raw, err := r.value(key)
	if err != nil {
		return "", err
	}
	switch v := raw.(type) {
	case dbus.ObjectPath:
		return v, nil
	case string:
		return dbus.ObjectPath(v), nil
	default:
		return "", fmt.Errorf("%w: %q is %T, want object path or string", ErrResultType, key, raw)
	}
}

type Caller struct {
	bus    Bus
	tokens TokenSource
}

func NewCaller(bus Bus, tokens TokenSource) *Caller {
	return &Caller{bus: bus, tokens: tokens}
}

func (c *Caller) Bus() Bus {
	return c.bus
}

func (c *Caller) NewToken() (string, error) {
	return c.tokens()
}

// Request runs one portal method that answers through a Request object: the Response subscription is
// installed before the call, so an immediate Response cannot be lost.
func (c *Caller) Request(ctx context.Context, method string, opts Options, args func(Options) []any) (resp Response, err error) {
	token, err := c.tokens()
	if err != nil {
		return Response{}, err
	}
	expected, err := RequestPath(c.bus.UniqueName(), token)
	if err != nil {
		return Response{}, err
	}
	sub, err := c.bus.Subscribe(ctx, SignalMatch{Path: expected, Interface: interfaceRequest, Member: signalResponse})
	if err != nil {
		return Response{}, fmt.Errorf("portal: subscribe to %s: %w", expected, err)
	}
	defer func() { err = errors.Join(err, sub.Close()) }()

	full := make(Options, len(opts)+1)
	for k, v := range opts {
		full[k] = v
	}
	full[optionHandleToken] = dbus.MakeVariant(token)

	ret, err := c.bus.Call(ctx, DesktopPath, method, args(full)...)
	if err != nil {
		return Response{}, err
	}
	if len(ret) != 1 {
		return Response{}, fmt.Errorf("%w: %s returned %d values, want the request handle", ErrUnexpectedReturn, method, len(ret))
	}
	handle, ok := ret[0].(dbus.ObjectPath)
	if !ok {
		return Response{}, fmt.Errorf("%w: %s returned %T, want an object path", ErrUnexpectedReturn, method, ret[0])
	}
	if handle != expected {
		return Response{}, fmt.Errorf("%w: %s returned %s, expected %s", ErrHandleMismatch, method, handle, expected)
	}

	select {
	case sig, open := <-sub.Signals():
		if !open {
			return Response{}, ErrBusClosed
		}
		resp, err = DecodeResponse(sig.Body)
		if err != nil {
			return Response{}, fmt.Errorf("%s: %w", method, err)
		}
		return resp, resp.Err(method)
	case <-ctx.Done():
		_, closeErr := c.bus.Call(context.WithoutCancel(ctx), expected, methodRequestClose)
		return Response{}, errors.Join(context.Cause(ctx), closeErr)
	}
}
