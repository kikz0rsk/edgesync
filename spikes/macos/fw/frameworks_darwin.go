// Package fw binds the macOS frameworks used by the EdgeSync spikes through purego, without cgo.
package fw

import (
	"fmt"

	"github.com/ebitengine/purego"
)

const (
	coreFoundationPath      = "/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation"
	coreGraphicsPath        = "/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics"
	applicationServicesPath = "/System/Library/Frameworks/ApplicationServices.framework/ApplicationServices"
	appKitPath              = "/System/Library/Frameworks/AppKit.framework/AppKit"
	foundationPath          = "/System/Library/Frameworks/Foundation.framework/Foundation"
)

type Handles struct {
	CoreFoundation      uintptr
	CoreGraphics        uintptr
	ApplicationServices uintptr
	AppKit              uintptr
	Foundation          uintptr
}

type Lib struct {
	Handles        Handles
	CoreFoundation *CoreFoundation
	CoreGraphics   *CoreGraphics
}

func Load() (*Lib, error) {
	var h Handles
	frameworks := []struct {
		path   string
		handle *uintptr
	}{
		{coreFoundationPath, &h.CoreFoundation},
		{coreGraphicsPath, &h.CoreGraphics},
		{applicationServicesPath, &h.ApplicationServices},
		{appKitPath, &h.AppKit},
		{foundationPath, &h.Foundation},
	}
	for _, f := range frameworks {
		handle, err := purego.Dlopen(f.path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			return nil, fmt.Errorf("open framework %s: %w", f.path, err)
		}
		*f.handle = handle
	}

	cf, err := bindCoreFoundation(h.CoreFoundation)
	if err != nil {
		return nil, err
	}
	cg, err := bindCoreGraphics(h.CoreGraphics)
	if err != nil {
		return nil, err
	}
	return &Lib{Handles: h, CoreFoundation: cf, CoreGraphics: cg}, nil
}

type binder struct {
	handle uintptr
	err    error
}

func (b *binder) bind(fptr any, name string) {
	if b.err != nil {
		return
	}
	sym, err := purego.Dlsym(b.handle, name)
	if err != nil {
		b.err = fmt.Errorf("resolve %s: %w", name, err)
		return
	}
	purego.RegisterFunc(fptr, sym)
}
