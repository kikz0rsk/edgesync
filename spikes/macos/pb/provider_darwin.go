package pb

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"sync"

	"github.com/ebitengine/purego/objc"

	"github.com/kikz0r/edgesync/spikes/macos/fw"
)

var (
	ErrProtocolNotFound = errors.New("Objective-C protocol not found")
	ErrProviderRejected = errors.New("NSPasteboardItem rejected the data provider")
	ErrItemRejectedData = errors.New("NSPasteboardItem rejected the provided data")
	ErrUnknownType      = errors.New("no content for the pasteboard type")
	ErrInvalidConfig    = errors.New("invalid configuration")
)

type Renderer func(pbType string) ([]byte, error)

type ProviderConfig struct {
	// ClassName must be unique in the process: an Objective-C class cannot be registered twice or unregistered.
	ClassName string
	Render    Renderer
}

// Provider is an Objective-C object whose class conforms to NSPasteboardItemDataProvider and forwards both protocol
// methods to Go. The registered IMPs are purego callbacks that are never freed, so a Provider is created once per
// class name and lives for the rest of the process; Close only releases the instance.
type Provider struct {
	kit      *kit
	instance objc.ID
	render   Renderer

	mu       sync.Mutex
	calls    map[string]int
	finished int
	errs     []error
}

func NewProvider(lib *fw.Lib, cfg ProviderConfig) (*Provider, error) {
	if cfg.ClassName == "" || cfg.Render == nil {
		return nil, fmt.Errorf("provider needs a class name and a renderer: %w", ErrInvalidConfig)
	}
	k, err := newKit(lib)
	if err != nil {
		return nil, err
	}
	protocol := objc.GetProtocol("NSPasteboardItemDataProvider")
	if protocol == nil {
		return nil, fmt.Errorf("NSPasteboardItemDataProvider: %w", ErrProtocolNotFound)
	}
	p := &Provider{kit: k, render: cfg.Render, calls: make(map[string]int)}
	class, err := objc.RegisterClass(cfg.ClassName, objc.GetClass("NSObject"), []*objc.Protocol{protocol}, nil, []objc.MethodDef{
		{Cmd: k.sel.provideData, Fn: p.provideData},
		{Cmd: k.sel.finishedWithProvider, Fn: p.finishedWithProvider},
	})
	if err != nil {
		return nil, fmt.Errorf("register class %s: %w", cfg.ClassName, err)
	}
	p.instance = objc.ID(class).Send(k.sel.alloc).Send(k.sel.init)
	if p.instance == 0 {
		return nil, fmt.Errorf("instantiate %s: %w", cfg.ClassName, fw.ErrClassNotFound)
	}
	return p, nil
}

// Close releases the instance; clear the pasteboard first, because the pasteboard may still call the provider.
func (p *Provider) Close() {
	p.instance.Send(p.kit.sel.release)
	p.instance = 0
}

// Write replaces the pasteboard contents with one item whose listed types are rendered lazily.
func (p *Provider) Write(board *Pasteboard, pbTypes ...string) error {
	return p.kit.inPool(func() error {
		item := objc.ID(p.kit.pasteboardItem).Send(p.kit.sel.new).Send(p.kit.sel.autorelease)
		types, err := p.kit.nsStringArray(pbTypes)
		if err != nil {
			return err
		}
		if !objc.Send[bool](item, p.kit.sel.setDataProvider, p.instance, types) {
			return fmt.Errorf("setDataProvider:forTypes: %v: %w", pbTypes, ErrProviderRejected)
		}
		board.Clear()
		return board.WriteObjects(item)
	})
}

func (p *Provider) Calls(pbType string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls[pbType]
}

func (p *Provider) Finished() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.finished
}

// Err joins every failure recorded inside the Objective-C callbacks, which cannot return errors to their caller.
func (p *Provider) Err() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return errors.Join(p.errs...)
}

func (p *Provider) fail(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.errs = append(p.errs, err)
}

func (p *Provider) provideData(_ objc.ID, _ objc.SEL, _ objc.ID, item objc.ID, pbType objc.ID) {
	name, err := fw.GoString(pbType)
	if err != nil {
		p.fail(fmt.Errorf("provided type name: %w", err))
		return
	}
	p.mu.Lock()
	p.calls[name]++
	p.mu.Unlock()

	data, err := p.render(name)
	if err != nil {
		p.fail(fmt.Errorf("render %s: %w", name, err))
		return
	}
	err = p.kit.inPool(func() error {
		if !objc.Send[bool](item, p.kit.sel.setDataForType, p.kit.nsDataFrom(data), pbType) {
			return fmt.Errorf("setData:forType: %s: %w", name, ErrItemRejectedData)
		}
		return nil
	})
	if err != nil {
		p.fail(err)
	}
}

func (p *Provider) finishedWithProvider(_ objc.ID, _ objc.SEL, _ objc.ID) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.finished++
}

// SampleContent returns deterministic content for the four provider types, including a real 2x2 PNG.
func SampleContent() (map[string][]byte, error) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	img.Set(1, 0, color.NRGBA{G: 255, A: 255})
	img.Set(0, 1, color.NRGBA{B: 255, A: 255})
	img.Set(1, 1, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		return nil, fmt.Errorf("encode sample png: %w", err)
	}
	return map[string][]byte{
		TypePlainText: []byte("EdgeSync lazy text"),
		TypeHTML:      []byte("<p>EdgeSync <b>lazy</b> html</p>"),
		TypeRTF:       []byte(`{\rtf1\ansi EdgeSync lazy rtf}`),
		TypePNG:       encoded.Bytes(),
	}, nil
}

func RendererFor(content map[string][]byte) Renderer {
	return func(pbType string) ([]byte, error) {
		data, ok := content[pbType]
		if !ok {
			return nil, fmt.Errorf("%s: %w", pbType, ErrUnknownType)
		}
		return data, nil
	}
}
