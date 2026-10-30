package fw

import (
	"errors"
	"fmt"
	"unsafe"

	"github.com/ebitengine/purego/objc"
)

var (
	ErrClassNotFound = errors.New("Objective-C class not found")
	ErrNilString     = errors.New("Objective-C string is nil")
)

func Class(name string) (objc.Class, error) {
	c := objc.GetClass(name)
	if c == 0 {
		return 0, fmt.Errorf("class %s: %w", name, ErrClassNotFound)
	}
	return c, nil
}

func Sel(name string) objc.SEL {
	return objc.RegisterName(name)
}

// NSString returns an autoreleased NSString; the caller must hold an autorelease pool.
func NSString(s string) (objc.ID, error) {
	class, err := Class("NSString")
	if err != nil {
		return 0, err
	}
	return objc.ID(class).Send(Sel("stringWithUTF8String:"), s), nil
}

func GoString(s objc.ID) (string, error) {
	if s == 0 {
		return "", fmt.Errorf("NSString receiver: %w", ErrNilString)
	}
	cstr := objc.Send[uintptr](s, Sel("UTF8String"))
	if cstr == 0 {
		return "", fmt.Errorf("UTF8String: %w", ErrNilString)
	}
	return cString(cstr), nil
}

func cString(p uintptr) string {
	ptr := *(*unsafe.Pointer)(unsafe.Pointer(&p))
	n := 0
	for *(*byte)(unsafe.Add(ptr, n)) != 0 {
		n++
	}
	return string(unsafe.Slice((*byte)(ptr), n))
}
