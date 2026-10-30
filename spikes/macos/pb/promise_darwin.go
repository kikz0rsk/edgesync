package pb

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/kikz0r/edgesync/spikes/macos/fw"
)

// PromiseFileType is the base UTI every file promise type must conform to (Apple: public.data, kUTTypeData).
const PromiseFileType = "public.data"

// blockInvokeOffset is the offset of the invoke pointer in a block literal (isa 8 bytes, flags 4, reserved 4;
// Clang Block ABI "Block_literal_1", identical to purego's own blockLayout). AppKit hands the completion handler
// over as a block it created itself, which purego's objc.Block.Invoke cannot call because that only knows blocks
// made by objc.NewBlock.
const blockInvokeOffset = 16

const (
	errorDomain = "io.github.kikz0r.edgesync.spike"
	errorCode   = 1
)

var (
	ErrNoPromise           = errors.New("pasteboard holds no file promise")
	ErrCompletionNotCalled = errors.New("completion handler was not called")
	ErrPromiseFailed       = errors.New("file promise failed")
)

type PromiseConfig struct {
	// ClassName must be unique in the process, see ProviderConfig.
	ClassName string
	FileType  string
	FileName  string
	Contents  []byte
}

// Promise owns an NSFilePromiseProvider and the Objective-C delegate object that implements
// NSFilePromiseProviderDelegate. The provider holds its delegate weakly, so Promise keeps the delegate alive.
type Promise struct {
	kit      *kit
	cfg      PromiseConfig
	delegate objc.ID
	provider objc.ID

	mu           sync.Mutex
	nameRequests int
	written      []string
	errs         []error
}

func NewPromise(lib *fw.Lib, cfg PromiseConfig) (*Promise, error) {
	if cfg.ClassName == "" || cfg.FileType == "" || cfg.FileName == "" {
		return nil, fmt.Errorf("promise needs a class name, file type and file name: %w", ErrInvalidConfig)
	}
	k, err := newKit(lib)
	if err != nil {
		return nil, err
	}
	protocol := objc.GetProtocol("NSFilePromiseProviderDelegate")
	if protocol == nil {
		return nil, fmt.Errorf("NSFilePromiseProviderDelegate: %w", ErrProtocolNotFound)
	}
	pr := &Promise{kit: k, cfg: cfg}
	class, err := objc.RegisterClass(cfg.ClassName, objc.GetClass("NSObject"), []*objc.Protocol{protocol}, nil, []objc.MethodDef{
		{Cmd: k.sel.fileNameForType, Fn: pr.fileNameForType},
		{Cmd: k.sel.writePromise, Fn: pr.writePromise},
	})
	if err != nil {
		return nil, fmt.Errorf("register class %s: %w", cfg.ClassName, err)
	}
	pr.delegate = objc.ID(class).Send(k.sel.alloc).Send(k.sel.init)
	if pr.delegate == 0 {
		return nil, fmt.Errorf("instantiate %s: %w", cfg.ClassName, fw.ErrClassNotFound)
	}
	err = k.inPool(func() error {
		fileType, err := fw.NSString(cfg.FileType)
		if err != nil {
			return err
		}
		pr.provider = objc.ID(k.filePromiseProvider).Send(k.sel.alloc).Send(k.sel.initWithFileType, fileType, pr.delegate)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if pr.provider == 0 {
		return nil, fmt.Errorf("initWithFileType:delegate: %s: %w", cfg.FileType, ErrProviderRejected)
	}
	return pr, nil
}

// Close releases the provider and the delegate; clear the pasteboard first.
func (p *Promise) Close() {
	p.provider.Send(p.kit.sel.release)
	p.delegate.Send(p.kit.sel.release)
	p.provider, p.delegate = 0, 0
}

// Write replaces the pasteboard contents with the promise.
func (p *Promise) Write(board *Pasteboard) error {
	board.Clear()
	return board.WriteObjects(p.provider)
}

func (p *Promise) NameRequests() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.nameRequests
}

func (p *Promise) Written() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.written...)
}

// Err joins every failure recorded inside the Objective-C callbacks.
func (p *Promise) Err() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return errors.Join(p.errs...)
}

func (p *Promise) fail(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.errs = append(p.errs, err)
}

func (p *Promise) fileNameForType(_ objc.ID, _ objc.SEL, _ objc.ID, _ objc.ID) objc.ID {
	p.mu.Lock()
	p.nameRequests++
	p.mu.Unlock()
	name, err := fw.NSString(p.cfg.FileName)
	if err != nil {
		p.fail(fmt.Errorf("file name string: %w", err))
		return 0
	}
	return name
}

func (p *Promise) writePromise(_ objc.ID, _ objc.SEL, _ objc.ID, url objc.ID, completion objc.ID) {
	failure := p.writeFile(url)
	if failure != nil {
		p.fail(failure)
	}
	if err := p.complete(completion, failure); err != nil {
		p.fail(err)
	}
}

func (p *Promise) writeFile(url objc.ID) error {
	var path string
	err := p.kit.inPool(func() error {
		var err error
		path, err = fw.GoString(url.Send(p.kit.sel.path))
		return err
	})
	if err != nil {
		return fmt.Errorf("destination path: %w", err)
	}
	if err := os.WriteFile(path, p.cfg.Contents, 0o600); err != nil {
		return fmt.Errorf("write promised file: %w", err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.written = append(p.written, path)
	return nil
}

func (p *Promise) complete(completion objc.ID, failure error) error {
	if completion == 0 {
		return fmt.Errorf("completion handler: %w", ErrCompletionNotCalled)
	}
	return p.kit.inPool(func() error {
		var errID objc.ID
		if failure != nil {
			var err error
			if errID, err = p.nsError(failure); err != nil {
				return err
			}
		}
		invokeBlock(completion, errID)
		return nil
	})
}

func (p *Promise) nsError(failure error) (objc.ID, error) {
	domain, err := fw.NSString(errorDomain)
	if err != nil {
		return 0, err
	}
	message, err := fw.NSString(failure.Error())
	if err != nil {
		return 0, err
	}
	userInfo := objc.ID(p.kit.nsDictionary).Send(p.kit.sel.dictionaryWithObjectForKey, message, p.kit.descriptionKey)
	return objc.ID(p.kit.nsError).Send(p.kit.sel.errorWithDomainCodeUserInfo, domain, errorCode, userInfo), nil
}

func invokeBlock(block objc.ID, arg objc.ID) {
	invoke := *(*uintptr)(unsafe.Add(pointerFrom(uintptr(block)), blockInvokeOffset))
	purego.SyscallN(invoke, uintptr(block), uintptr(arg))
}

// FulfillDirect sends the delegate's two protocol messages itself, as AppKit would when a promise is fulfilled:
// it asks for the file name, then for the write into dir with a completion block. It proves class registration,
// selector dispatch and the raw block invocation, not AppKit's own fulfilment path (see ReceivePromised).
func (p *Promise) FulfillDirect(dir string) (string, error) {
	var path string
	err := p.kit.inPool(func() error {
		fileType, err := fw.NSString(p.cfg.FileType)
		if err != nil {
			return err
		}
		nameID := p.delegate.Send(p.kit.sel.fileNameForType, p.provider, fileType)
		name, err := fw.GoString(nameID)
		if err != nil {
			return fmt.Errorf("delegate file name: %w", err)
		}
		path = filepath.Join(dir, name)
		pathString, err := fw.NSString(path)
		if err != nil {
			return err
		}
		url := objc.ID(p.kit.nsURL).Send(p.kit.sel.fileURLWithPath, pathString)

		completed := make(chan error, 1)
		block := objc.NewBlock(func(_ objc.Block, failure objc.ID) {
			completed <- p.describeError(failure)
		})
		defer block.Release()
		p.delegate.Send(p.kit.sel.writePromise, p.provider, url, objc.ID(block))
		select {
		case err := <-completed:
			return err
		default:
			return fmt.Errorf("write promise to %s: %w", path, ErrCompletionNotCalled)
		}
	})
	return path, err
}

func (p *Promise) describeError(failure objc.ID) error {
	if failure == 0 {
		return nil
	}
	var text string
	err := p.kit.inPool(func() error {
		var err error
		text, err = fw.GoString(failure.Send(p.kit.sel.localizedDescription))
		return err
	})
	if err != nil {
		return fmt.Errorf("describe NSError: %w", err)
	}
	return fmt.Errorf("%s: %w", text, ErrPromiseFailed)
}

type promisedFile struct {
	path string
	err  error
}

// ReceivePromised reads NSFilePromiseReceiver objects from the pasteboard and asks AppKit to fulfil every promise
// into dir, which is the path a drag destination uses. It returns ErrNoPromise when the pasteboard has none and
// the context error when AppKit never calls the reader back; on that timeout the block and queue are leaked on
// purpose because AppKit may still use them.
func ReceivePromised(ctx context.Context, board *Pasteboard, dir string) ([]string, error) {
	k := board.kit
	var received []string
	err := k.inPool(func() error {
		classes := k.nsArrayFrom([]objc.ID{objc.ID(k.filePromiseReceiver)})
		receivers := board.general.Send(k.sel.readObjectsForClasses, classes, objc.ID(0))
		if receivers == 0 {
			return fmt.Errorf("readObjectsForClasses: %w", ErrNoPromise)
		}
		count := objc.Send[uint](receivers, k.sel.count)
		expected := uint(0)
		for i := range count {
			names := receivers.Send(k.sel.objectAtIndex, i).Send(k.sel.fileNames)
			expected += objc.Send[uint](names, k.sel.count)
		}
		if expected == 0 {
			return fmt.Errorf("receivers report no file names: %w", ErrNoPromise)
		}

		dirString, err := fw.NSString(dir)
		if err != nil {
			return err
		}
		destination := objc.ID(k.nsURL).Send(k.sel.fileURLWithPath, dirString)
		options := objc.ID(k.nsDictionary).Send(k.sel.dictionary)
		queue := objc.ID(k.nsOperationQueue).Send(k.sel.new)

		results := make(chan promisedFile, expected)
		block := objc.NewBlock(func(_ objc.Block, fileURL objc.ID, failure objc.ID) {
			results <- k.promisedFileFrom(fileURL, failure)
		})
		for i := range count {
			receivers.Send(k.sel.objectAtIndex, i).Send(k.sel.receivePromisedFiles, destination, options, queue, objc.ID(block))
		}
		for range expected {
			select {
			case r := <-results:
				if r.err != nil {
					return r.err
				}
				received = append(received, r.path)
			case <-ctx.Done():
				return fmt.Errorf("wait for promised files: %w", ctx.Err())
			}
		}
		block.Release()
		queue.Send(k.sel.release)
		return nil
	})
	return received, err
}

func (k *kit) promisedFileFrom(fileURL objc.ID, failure objc.ID) promisedFile {
	var result promisedFile
	err := k.inPool(func() error {
		if failure != 0 {
			text, err := fw.GoString(failure.Send(k.sel.localizedDescription))
			if err != nil {
				return fmt.Errorf("describe NSError: %w", err)
			}
			result.err = fmt.Errorf("%s: %w", text, ErrPromiseFailed)
			return nil
		}
		path, err := fw.GoString(fileURL.Send(k.sel.path))
		if err != nil {
			return fmt.Errorf("promised file path: %w", err)
		}
		result.path = path
		return nil
	})
	if err != nil {
		result.err = err
	}
	return result
}
