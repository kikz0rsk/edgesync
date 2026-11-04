package tap

import (
	"github.com/kikz0r/edgesync/spikes/macos/fw"
)

func bindLoopAPI(lib *fw.Lib) (*loopAPI, error) {
	s := symbols{handle: lib.Handles.CoreFoundation}
	api := &loopAPI{release: lib.CoreFoundation.Release}
	s.bind(&api.getCurrent, "CFRunLoopGetCurrent")
	s.bind(&api.run, "CFRunLoopRun")
	s.bind(&api.stop, "CFRunLoopStop")
	s.bind(&api.wakeUp, "CFRunLoopWakeUp")
	s.bind(&api.addSource, "CFRunLoopAddSource")
	s.bind(&api.removeSource, "CFRunLoopRemoveSource")
	s.bind(&api.createRunLoopSource, "CFMachPortCreateRunLoopSource")
	s.bind(&api.machPortInvalidate, "CFMachPortInvalidate")
	s.bind(&api.retain, "CFRetain")
	api.commonModes = s.pointerVariable("kCFRunLoopCommonModes")
	if s.err != nil {
		return nil, s.err
	}
	return api, nil
}
