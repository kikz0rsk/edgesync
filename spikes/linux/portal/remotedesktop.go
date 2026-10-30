package portal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/godbus/dbus/v5"
)

const (
	interfaceRemoteDesktop = "org.freedesktop.portal.RemoteDesktop"

	methodRDCreateSession = interfaceRemoteDesktop + ".CreateSession"
	methodRDSelectDevices = interfaceRemoteDesktop + ".SelectDevices"
	methodRDStart         = interfaceRemoteDesktop + ".Start"
	methodRDConnectToEIS  = interfaceRemoteDesktop + ".ConnectToEIS"

	propertyAvailableDeviceTypes = "AvailableDeviceTypes"

	remoteDesktopVersionWithEIS = 2
)

var (
	ErrNoDevices      = errors.New("portal: the user granted no input device")
	ErrEISUnsupported = errors.New("portal: RemoteDesktop.ConnectToEIS needs interface version 2")
)

type DeviceType uint32

const (
	DeviceKeyboard    DeviceType = 1
	DevicePointer     DeviceType = 2
	DeviceTouchscreen DeviceType = 4
)

type RemoteParams struct {
	Types        DeviceType
	PersistMode  PersistMode
	RestoreToken string
}

type RemoteDesktop struct {
	caller *Caller
}

func NewRemoteDesktop(caller *Caller) *RemoteDesktop {
	return &RemoteDesktop{caller: caller}
}

func (rd *RemoteDesktop) Version(ctx context.Context) (uint32, error) {
	return interfaceVersion(ctx, rd.caller.Bus(), interfaceRemoteDesktop)
}

func (rd *RemoteDesktop) AvailableDeviceTypes(ctx context.Context) (DeviceType, error) {
	v, err := rd.caller.Bus().Property(ctx, DesktopPath, interfaceRemoteDesktop, propertyAvailableDeviceTypes)
	if err != nil {
		return 0, err
	}
	types, ok := v.Value().(uint32)
	if !ok {
		return 0, fmt.Errorf("%w: AvailableDeviceTypes is %T, want uint32", ErrResultType, v.Value())
	}
	return DeviceType(types), nil
}

type RemoteSession struct {
	*Session
	caller *Caller

	Version          uint32
	Devices          DeviceType
	ClipboardEnabled bool
	RestoreToken     string

	eisMu        sync.Mutex
	eisConnected bool
}

// Open runs CreateSession, SelectDevices and Start. Start shows the consent dialog unless a restore token
// from an earlier session is still valid.
func (rd *RemoteDesktop) Open(ctx context.Context, p RemoteParams) (*RemoteSession, error) {
	if p.Types == 0 {
		return nil, ErrNoDevices
	}
	version, err := rd.Version(ctx)
	if err != nil {
		return nil, err
	}
	if version < remoteDesktopVersionWithEIS && (p.PersistMode != PersistNone || p.RestoreToken != "") {
		return nil, ErrPersistenceUnsupported
	}

	sessionToken, err := rd.caller.NewToken()
	if err != nil {
		return nil, err
	}
	created, err := rd.caller.Request(ctx, methodRDCreateSession,
		Options{optionSessionToken: dbus.MakeVariant(sessionToken)},
		func(o Options) []any { return []any{o} })
	if err != nil {
		return nil, err
	}
	path, err := created.Results.PathOrString("session_handle")
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	rs := &RemoteSession{Session: newSession(rd.caller.Bus(), path), caller: rd.caller, Version: version}

	if err := rs.selectDevices(ctx, p, version); err != nil {
		return nil, errors.Join(err, rs.Close(ctx))
	}
	if err := rs.start(ctx); err != nil {
		return nil, errors.Join(err, rs.Close(ctx))
	}
	if rs.Devices == 0 {
		return nil, errors.Join(ErrNoDevices, rs.Close(ctx))
	}
	return rs, nil
}

func (rs *RemoteSession) selectDevices(ctx context.Context, p RemoteParams, version uint32) error {
	opts := Options{"types": dbus.MakeVariant(uint32(p.Types))}
	if version >= remoteDesktopVersionWithEIS {
		opts["persist_mode"] = dbus.MakeVariant(uint32(p.PersistMode))
		if p.RestoreToken != "" {
			opts["restore_token"] = dbus.MakeVariant(p.RestoreToken)
		}
	}
	_, err := rs.caller.Request(ctx, methodRDSelectDevices, opts, func(o Options) []any {
		return []any{rs.path, o}
	})
	return err
}

func (rs *RemoteSession) start(ctx context.Context) error {
	resp, err := rs.caller.Request(ctx, methodRDStart, Options{}, func(o Options) []any {
		return []any{rs.path, "", o}
	})
	if err != nil {
		return err
	}
	devices, err := resp.Results.Uint32("devices")
	if err != nil {
		return fmt.Errorf("start: %w", err)
	}
	rs.Devices = DeviceType(devices)
	if resp.Results.Has("clipboard_enabled") {
		if rs.ClipboardEnabled, err = resp.Results.Bool("clipboard_enabled"); err != nil {
			return fmt.Errorf("start: %w", err)
		}
	}
	if resp.Results.Has("restore_token") {
		if rs.RestoreToken, err = resp.Results.String("restore_token"); err != nil {
			return fmt.Errorf("start: %w", err)
		}
	}
	return nil
}

func (rs *RemoteSession) ConnectToEIS(ctx context.Context) (*os.File, error) {
	if err := rs.ensureOpen(); err != nil {
		return nil, err
	}
	if rs.Version < remoteDesktopVersionWithEIS {
		return nil, ErrEISUnsupported
	}
	rs.eisMu.Lock()
	defer rs.eisMu.Unlock()
	if rs.eisConnected {
		return nil, ErrEISAlreadyConnected
	}
	f, err := rs.caller.Bus().CallFD(ctx, DesktopPath, methodRDConnectToEIS, rs.path, Options{})
	if err != nil {
		return nil, err
	}
	rs.eisConnected = true
	return f, nil
}

func (rs *RemoteSession) Events(ctx context.Context) (*EventStream, error) {
	if err := rs.ensureOpen(); err != nil {
		return nil, err
	}
	return newEventStream(ctx, rs.caller.Bus(), closedSource(rs.path))
}
