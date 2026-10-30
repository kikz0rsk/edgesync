// Package pb drives NSPasteboard, a lazy NSPasteboardItemDataProvider and NSFilePromiseProvider through purego for
// the macOS clipboard spike (D-10, D-11, D-86). Everything runs without cgo.
package pb

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/kikz0r/edgesync/spikes/macos/fw"
)

// Pasteboard type identifiers. The public.* values are listed in Apple's "System-Declared Uniform Type
// Identifiers" (archived UTI reference); the NSPasteboardType* constants carrying them do not state the string in
// Apple's current docs, so ResolveAppKitTypes reads them from AppKit at run time to cross-check these literals.
// TypeConcealed is the nspasteboard.org community convention, not an Apple API (second-source).
const (
	TypePlainText = "public.utf8-plain-text"
	TypeHTML      = "public.html"
	TypeRTF       = "public.rtf"
	TypePNG       = "public.png"
	TypeFileURL   = "public.file-url"
	TypeConcealed = "org.nspasteboard.ConcealedType"
)

// PollInterval is the changeCount polling period fixed by D-86.
const PollInterval = 500 * time.Millisecond

var (
	ErrTypeUnavailable = errors.New("pasteboard has no data for the type")
	ErrWriteRejected   = errors.New("pasteboard rejected the write")
	ErrSymbolEmpty     = errors.New("AppKit constant is NULL")
)

type selectors struct {
	alloc, init, new, autorelease, release, drain                      objc.SEL
	count, objectAtIndex, arrayWithObjectsCount, array                 objc.SEL
	data, dataWithBytesLength, length, bytes                           objc.SEL
	generalPasteboard, changeCount, clearContents, types, dataForType  objc.SEL
	stringForType, setDataForType, writeObjects, readObjectsForClasses objc.SEL
	setDataProvider                                                    objc.SEL
	fileURLWithPath, path                                              objc.SEL
	initWithFileType, fileNames, dictionary                            objc.SEL
	dictionaryWithObjectForKey, errorWithDomainCodeUserInfo            objc.SEL
	localizedDescription, receivePromisedFiles                         objc.SEL
	provideData, finishedWithProvider, fileNameForType, writePromise   objc.SEL
}

func newSelectors() selectors {
	return selectors{
		alloc:                       fw.Sel("alloc"),
		init:                        fw.Sel("init"),
		new:                         fw.Sel("new"),
		autorelease:                 fw.Sel("autorelease"),
		release:                     fw.Sel("release"),
		drain:                       fw.Sel("drain"),
		count:                       fw.Sel("count"),
		objectAtIndex:               fw.Sel("objectAtIndex:"),
		arrayWithObjectsCount:       fw.Sel("arrayWithObjects:count:"),
		array:                       fw.Sel("array"),
		data:                        fw.Sel("data"),
		dataWithBytesLength:         fw.Sel("dataWithBytes:length:"),
		length:                      fw.Sel("length"),
		bytes:                       fw.Sel("bytes"),
		generalPasteboard:           fw.Sel("generalPasteboard"),
		changeCount:                 fw.Sel("changeCount"),
		clearContents:               fw.Sel("clearContents"),
		types:                       fw.Sel("types"),
		dataForType:                 fw.Sel("dataForType:"),
		stringForType:               fw.Sel("stringForType:"),
		setDataForType:              fw.Sel("setData:forType:"),
		writeObjects:                fw.Sel("writeObjects:"),
		readObjectsForClasses:       fw.Sel("readObjectsForClasses:options:"),
		setDataProvider:             fw.Sel("setDataProvider:forTypes:"),
		fileURLWithPath:             fw.Sel("fileURLWithPath:"),
		path:                        fw.Sel("path"),
		initWithFileType:            fw.Sel("initWithFileType:delegate:"),
		fileNames:                   fw.Sel("fileNames"),
		dictionary:                  fw.Sel("dictionary"),
		dictionaryWithObjectForKey:  fw.Sel("dictionaryWithObject:forKey:"),
		errorWithDomainCodeUserInfo: fw.Sel("errorWithDomain:code:userInfo:"),
		localizedDescription:        fw.Sel("localizedDescription"),
		receivePromisedFiles:        fw.Sel("receivePromisedFilesAtDestination:options:operationQueue:reader:"),
		provideData:                 fw.Sel("pasteboard:item:provideDataForType:"),
		finishedWithProvider:        fw.Sel("pasteboardFinishedWithDataProvider:"),
		fileNameForType:             fw.Sel("filePromiseProvider:fileNameForType:"),
		writePromise:                fw.Sel("filePromiseProvider:writePromiseToURL:completionHandler:"),
	}
}

type kit struct {
	sel                 selectors
	pool                objc.Class
	nsData              objc.Class
	nsArray             objc.Class
	nsDictionary        objc.Class
	nsURL               objc.Class
	nsError             objc.Class
	nsOperationQueue    objc.Class
	pasteboard          objc.Class
	pasteboardItem      objc.Class
	filePromiseProvider objc.Class
	filePromiseReceiver objc.Class
	descriptionKey      objc.ID
}

func newKit(lib *fw.Lib) (*kit, error) {
	k := &kit{sel: newSelectors()}
	classes := []struct {
		name string
		dst  *objc.Class
	}{
		{"NSAutoreleasePool", &k.pool},
		{"NSData", &k.nsData},
		{"NSArray", &k.nsArray},
		{"NSDictionary", &k.nsDictionary},
		{"NSURL", &k.nsURL},
		{"NSError", &k.nsError},
		{"NSOperationQueue", &k.nsOperationQueue},
		{"NSPasteboard", &k.pasteboard},
		{"NSPasteboardItem", &k.pasteboardItem},
		{"NSFilePromiseProvider", &k.filePromiseProvider},
		{"NSFilePromiseReceiver", &k.filePromiseReceiver},
	}
	for _, c := range classes {
		class, err := fw.Class(c.name)
		if err != nil {
			return nil, err
		}
		*c.dst = class
	}
	key, err := stringConstant(lib.Handles.Foundation, "NSLocalizedDescriptionKey")
	if err != nil {
		return nil, err
	}
	k.descriptionKey = key
	return k, nil
}

func stringConstant(handle uintptr, symbol string) (objc.ID, error) {
	addr, err := purego.Dlsym(handle, symbol)
	if err != nil {
		return 0, fmt.Errorf("resolve %s: %w", symbol, err)
	}
	value := *(*objc.ID)(pointerFrom(addr))
	if value == 0 {
		return 0, fmt.Errorf("%s: %w", symbol, ErrSymbolEmpty)
	}
	return value, nil
}

func pointerFrom(address uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&address))
}

// inPool runs fn on a locked OS thread inside an autorelease pool: a pool must be drained on the thread that
// created it, and Go threads carry no pool of their own.
func (k *kit) inPool(fn func() error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := objc.ID(k.pool).Send(k.sel.alloc).Send(k.sel.init)
	defer pool.Send(k.sel.drain)
	return fn()
}

func (k *kit) nsDataFrom(b []byte) objc.ID {
	if len(b) == 0 {
		return objc.ID(k.nsData).Send(k.sel.data)
	}
	return objc.ID(k.nsData).Send(k.sel.dataWithBytesLength, unsafe.Pointer(unsafe.SliceData(b)), uint(len(b)))
}

func (k *kit) goBytes(data objc.ID) []byte {
	n := objc.Send[uint](data, k.sel.length)
	if n == 0 {
		return []byte{}
	}
	ptr := pointerFrom(objc.Send[uintptr](data, k.sel.bytes))
	return bytes.Clone(unsafe.Slice((*byte)(ptr), n))
}

func (k *kit) nsArrayFrom(objects []objc.ID) objc.ID {
	if len(objects) == 0 {
		return objc.ID(k.nsArray).Send(k.sel.array)
	}
	return objc.ID(k.nsArray).Send(k.sel.arrayWithObjectsCount, unsafe.Pointer(unsafe.SliceData(objects)), uint(len(objects)))
}

func (k *kit) nsStringArray(values []string) (objc.ID, error) {
	objects := make([]objc.ID, len(values))
	for i, v := range values {
		s, err := fw.NSString(v)
		if err != nil {
			return 0, err
		}
		objects[i] = s
	}
	return k.nsArrayFrom(objects), nil
}

func (k *kit) goStrings(array objc.ID) ([]string, error) {
	n := objc.Send[uint](array, k.sel.count)
	out := make([]string, 0, n)
	for i := range n {
		s, err := fw.GoString(array.Send(k.sel.objectAtIndex, i))
		if err != nil {
			return nil, fmt.Errorf("element %d: %w", i, err)
		}
		out = append(out, s)
	}
	return out, nil
}

// AppKitTypes holds the values of the documented NSPasteboardType constants as AppKit reports them.
type AppKitTypes struct {
	String, HTML, RTF, PNG, FileURL string
}

func ResolveAppKitTypes(lib *fw.Lib) (AppKitTypes, error) {
	var types AppKitTypes
	for _, s := range []struct {
		name string
		dst  *string
	}{
		{"NSPasteboardTypeString", &types.String},
		{"NSPasteboardTypeHTML", &types.HTML},
		{"NSPasteboardTypeRTF", &types.RTF},
		{"NSPasteboardTypePNG", &types.PNG},
		{"NSPasteboardTypeFileURL", &types.FileURL},
	} {
		id, err := stringConstant(lib.Handles.AppKit, s.name)
		if err != nil {
			return AppKitTypes{}, err
		}
		value, err := fw.GoString(id)
		if err != nil {
			return AppKitTypes{}, fmt.Errorf("%s: %w", s.name, err)
		}
		*s.dst = value
	}
	return types, nil
}

type Pasteboard struct {
	kit     *kit
	general objc.ID
}

func New(lib *fw.Lib) (*Pasteboard, error) {
	k, err := newKit(lib)
	if err != nil {
		return nil, err
	}
	general := objc.ID(k.pasteboard).Send(k.sel.generalPasteboard)
	if general == 0 {
		return nil, fmt.Errorf("generalPasteboard: %w", fw.ErrClassNotFound)
	}
	return &Pasteboard{kit: k, general: general}, nil
}

// ChangeCount reads only the counter, never pasteboard contents.
func (p *Pasteboard) ChangeCount() int {
	return objc.Send[int](p.general, p.kit.sel.changeCount)
}

// Clear returns the new change count.
func (p *Pasteboard) Clear() int {
	return objc.Send[int](p.general, p.kit.sel.clearContents)
}

// Types returns the type identifiers currently on the pasteboard without reading data; an empty pasteboard
// yields a nil slice and a nil error.
func (p *Pasteboard) Types() ([]string, error) {
	var out []string
	err := p.kit.inPool(func() error {
		array := p.general.Send(p.kit.sel.types)
		if array == 0 {
			return nil
		}
		var err error
		out, err = p.kit.goStrings(array)
		return err
	})
	return out, err
}

func (p *Pasteboard) Data(pbType string) ([]byte, error) {
	var out []byte
	err := p.kit.inPool(func() error {
		name, err := fw.NSString(pbType)
		if err != nil {
			return err
		}
		data := p.general.Send(p.kit.sel.dataForType, name)
		if data == 0 {
			return fmt.Errorf("dataForType %s: %w", pbType, ErrTypeUnavailable)
		}
		out = p.kit.goBytes(data)
		return nil
	})
	return out, err
}

func (p *Pasteboard) String(pbType string) (string, error) {
	var out string
	err := p.kit.inPool(func() error {
		name, err := fw.NSString(pbType)
		if err != nil {
			return err
		}
		value := p.general.Send(p.kit.sel.stringForType, name)
		if value == 0 {
			return fmt.Errorf("stringForType %s: %w", pbType, ErrTypeUnavailable)
		}
		out, err = fw.GoString(value)
		return err
	})
	return out, err
}

// SetData adds eager data for one type; call Clear first to take ownership of the pasteboard.
func (p *Pasteboard) SetData(pbType string, data []byte) error {
	return p.kit.inPool(func() error {
		name, err := fw.NSString(pbType)
		if err != nil {
			return err
		}
		if !objc.Send[bool](p.general, p.kit.sel.setDataForType, p.kit.nsDataFrom(data), name) {
			return fmt.Errorf("setData:forType: %s: %w", pbType, ErrWriteRejected)
		}
		return nil
	})
}

func (p *Pasteboard) WriteObjects(objects ...objc.ID) error {
	return p.kit.inPool(func() error {
		if !objc.Send[bool](p.general, p.kit.sel.writeObjects, p.kit.nsArrayFrom(objects)) {
			return fmt.Errorf("writeObjects: %w", ErrWriteRejected)
		}
		return nil
	})
}

func (p *Pasteboard) IsConcealed() (bool, error) {
	types, err := p.Types()
	if err != nil {
		return false, err
	}
	return slices.Contains(types, TypeConcealed), nil
}

// Watch polls the change counter every interval, reports every value that differs from the last one seen
// (starting with since) and returns ctx.Err() once ctx ends.
func (p *Pasteboard) Watch(ctx context.Context, interval time.Duration, since int, onChange func(changeCount int)) error {
	last := since
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			current := p.ChangeCount()
			if current != last {
				last = current
				onChange(current)
			}
		}
	}
}
