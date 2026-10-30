package fw

type CoreFoundation struct {
	release func(ref uintptr)
}

func bindCoreFoundation(handle uintptr) (*CoreFoundation, error) {
	b := binder{handle: handle}
	cf := &CoreFoundation{}
	b.bind(&cf.release, "CFRelease")
	if b.err != nil {
		return nil, b.err
	}
	return cf, nil
}

func (cf *CoreFoundation) Release(ref uintptr) {
	cf.release(ref)
}
