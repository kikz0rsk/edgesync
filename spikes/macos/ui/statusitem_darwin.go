package ui

import (
	"errors"
	"fmt"

	"github.com/ebitengine/purego/objc"

	"github.com/kikz0r/edgesync/spikes/macos/fw"
)

// NSVariableStatusItemLength is not stated on Apple's NSStatusBar pages, which name it only (second-source:
// servo/core-foundation-rs cocoa appkit.rs, MIT/Apache-2.0).
const variableStatusItemLength = -1.0

var (
	ErrInvalidConfig  = errors.New("invalid configuration")
	ErrClassExists    = errors.New("Objective-C class name already registered")
	ErrUnknownMenuTag = errors.New("menu action from an item without a registered tag")
)

type MenuItem struct {
	ID        string
	Label     string
	Separator bool
}

type StatusItemConfig struct {
	// ClassName must be unique in the process: an Objective-C class cannot be registered twice or unregistered.
	ClassName string
	Title     string
	Items     []MenuItem
	// OnAction runs on the main thread inside AppKit's event handling and must return quickly.
	OnAction func(id string)
}

type statusSelectors struct {
	systemStatusBar, statusItemWithLength, removeStatusItem objc.SEL
	button, setTitle, title, setMenu, isVisible             objc.SEL
	initWithTitle, setAutoenablesItems                      objc.SEL
	initWithTitleAction, separatorItem, addItem             objc.SEL
	setTarget, setTag, tag                                  objc.SEL
	menuAction                                              objc.SEL
}

// StatusItem is an NSStatusItem whose NSMenu items call back into Go through a registered Objective-C target class.
// The registered IMP is a purego callback that is never freed, so a StatusItem is created once per class name and
// lives for the rest of the process; Close only removes the item and releases the instances.
type StatusItem struct {
	app       *App
	sel       statusSelectors
	statusBar objc.ID
	item      objc.ID
	menu      objc.ID
	target    objc.ID
	ids       []string
	onAction  func(id string)
}

// menuActionIDs returns the action id for each tag; separators have no tag.
func menuActionIDs(items []MenuItem) ([]string, error) {
	ids := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for i, item := range items {
		if item.Separator {
			continue
		}
		if item.ID == "" || item.Label == "" {
			return nil, fmt.Errorf("menu item %d needs an id and a label: %w", i, ErrInvalidConfig)
		}
		if _, dup := seen[item.ID]; dup {
			return nil, fmt.Errorf("menu item id %q is used twice: %w", item.ID, ErrInvalidConfig)
		}
		seen[item.ID] = struct{}{}
		ids = append(ids, item.ID)
	}
	return ids, nil
}

// NewStatusItem must run on the main thread.
func NewStatusItem(app *App, cfg StatusItemConfig) (*StatusItem, error) {
	if cfg.ClassName == "" || cfg.OnAction == nil || len(cfg.Items) == 0 {
		return nil, fmt.Errorf("status item needs a class name, items and an action handler: %w", ErrInvalidConfig)
	}
	ids, err := menuActionIDs(cfg.Items)
	if err != nil {
		return nil, err
	}
	if err := app.requireMainThread(); err != nil {
		return nil, err
	}
	if objc.GetClass(cfg.ClassName) != 0 {
		return nil, fmt.Errorf("class %s: %w", cfg.ClassName, ErrClassExists)
	}
	s := &StatusItem{app: app, ids: ids, onAction: cfg.OnAction, sel: newStatusSelectors()}
	statusBarClass, menuClass, menuItemClass, err := statusClasses()
	if err != nil {
		return nil, err
	}

	err = app.inPool(func() error {
		target, err := s.newTarget(cfg.ClassName)
		if err != nil {
			return err
		}
		s.target = target

		menu, err := s.newMenu(menuClass, menuItemClass, cfg.Items)
		if err != nil {
			return err
		}
		s.menu = menu

		s.statusBar = objc.ID(statusBarClass).Send(s.sel.systemStatusBar)
		if s.statusBar == 0 {
			return fmt.Errorf("NSStatusBar systemStatusBar: %w", ErrNilObject)
		}
		// NSStatusBar keeps no reference to the item (statusItemWithLength: documentation), so ours keeps it alive.
		s.item = s.statusBar.Send(s.sel.statusItemWithLength, variableStatusItemLength).Send(app.base.retain)
		if s.item == 0 {
			return fmt.Errorf("NSStatusBar statusItemWithLength: %w", ErrNilObject)
		}
		button := s.item.Send(s.sel.button)
		if button == 0 {
			return fmt.Errorf("NSStatusItem button: %w", ErrNilObject)
		}
		title, err := fw.NSString(cfg.Title)
		if err != nil {
			return err
		}
		button.Send(s.sel.setTitle, title)
		s.item.Send(s.sel.setMenu, s.menu)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s, nil
}

func newStatusSelectors() statusSelectors {
	return statusSelectors{
		systemStatusBar:      fw.Sel("systemStatusBar"),
		statusItemWithLength: fw.Sel("statusItemWithLength:"),
		removeStatusItem:     fw.Sel("removeStatusItem:"),
		button:               fw.Sel("button"),
		setTitle:             fw.Sel("setTitle:"),
		title:                fw.Sel("title"),
		setMenu:              fw.Sel("setMenu:"),
		isVisible:            fw.Sel("isVisible"),
		initWithTitle:        fw.Sel("initWithTitle:"),
		setAutoenablesItems:  fw.Sel("setAutoenablesItems:"),
		initWithTitleAction:  fw.Sel("initWithTitle:action:keyEquivalent:"),
		separatorItem:        fw.Sel("separatorItem"),
		addItem:              fw.Sel("addItem:"),
		setTarget:            fw.Sel("setTarget:"),
		setTag:               fw.Sel("setTag:"),
		tag:                  fw.Sel("tag"),
		menuAction:           fw.Sel("edgesyncMenuAction:"),
	}
}

func statusClasses() (statusBar, menu, menuItem objc.Class, err error) {
	for _, c := range []struct {
		name string
		dst  *objc.Class
	}{
		{"NSStatusBar", &statusBar},
		{"NSMenu", &menu},
		{"NSMenuItem", &menuItem},
	} {
		class, classErr := fw.Class(c.name)
		if classErr != nil {
			return 0, 0, 0, classErr
		}
		*c.dst = class
	}
	return statusBar, menu, menuItem, nil
}

func (s *StatusItem) newTarget(className string) (objc.ID, error) {
	class, err := objc.RegisterClass(className, objc.GetClass("NSObject"), nil, nil, []objc.MethodDef{
		{Cmd: s.sel.menuAction, Fn: s.menuActionIMP},
	})
	if err != nil {
		return 0, fmt.Errorf("register class %s: %w", className, err)
	}
	target := objc.ID(class).Send(s.app.base.alloc).Send(s.app.base.init)
	if target == 0 {
		return 0, fmt.Errorf("instantiate %s: %w", className, ErrNilObject)
	}
	return target, nil
}

func (s *StatusItem) newMenu(menuClass, menuItemClass objc.Class, items []MenuItem) (objc.ID, error) {
	empty, err := fw.NSString("")
	if err != nil {
		return 0, err
	}
	menu := objc.ID(menuClass).Send(s.app.base.alloc).Send(s.sel.initWithTitle, empty)
	if menu == 0 {
		return 0, fmt.Errorf("NSMenu initWithTitle: %w", ErrNilObject)
	}
	menu.Send(s.sel.setAutoenablesItems, false)

	tag := 0
	for _, item := range items {
		if item.Separator {
			menu.Send(s.sel.addItem, objc.ID(menuItemClass).Send(s.sel.separatorItem))
			continue
		}
		label, err := fw.NSString(item.Label)
		if err != nil {
			return 0, err
		}
		menuItem := objc.ID(menuItemClass).Send(s.app.base.alloc).Send(s.sel.initWithTitleAction, label, s.sel.menuAction, empty)
		if menuItem == 0 {
			return 0, fmt.Errorf("NSMenuItem initWithTitle:action:keyEquivalent: %q: %w", item.Label, ErrNilObject)
		}
		menuItem.Send(s.sel.setTarget, s.target)
		menuItem.Send(s.sel.setTag, tag)
		menu.Send(s.sel.addItem, menuItem)
		menuItem.Send(s.app.base.release)
		tag++
	}
	return menu, nil
}

// menuActionIMP runs on the main thread. NSMenuItem holds its target weakly, which is why the StatusItem owns it.
func (s *StatusItem) menuActionIMP(_ objc.ID, _ objc.SEL, sender objc.ID) {
	tag := objc.Send[int](sender, s.sel.tag)
	if tag < 0 || tag >= len(s.ids) {
		s.app.Fail(fmt.Errorf("tag %d of %d items: %w", tag, len(s.ids), ErrUnknownMenuTag))
		return
	}
	s.onAction(s.ids[tag])
}

// ButtonTitle reads the title back from the status item button; it must run on the main thread.
func (s *StatusItem) ButtonTitle() (string, error) {
	if err := s.app.requireMainThread(); err != nil {
		return "", err
	}
	var title string
	err := s.app.inPool(func() error {
		value := s.item.Send(s.sel.button).Send(s.sel.title)
		goTitle, err := fw.GoString(value)
		title = goTitle
		return err
	})
	return title, err
}

// Visible reports NSStatusItem isVisible; it must run on the main thread.
func (s *StatusItem) Visible() (bool, error) {
	if err := s.app.requireMainThread(); err != nil {
		return false, err
	}
	return objc.Send[bool](s.item, s.sel.isVisible), nil
}

// Close removes the item from the menu bar and releases the instances; it must run on the main thread after Run
// has returned or before it starts.
func (s *StatusItem) Close() error {
	if err := s.app.requireMainThread(); err != nil {
		return err
	}
	s.statusBar.Send(s.sel.removeStatusItem, s.item)
	for _, object := range []objc.ID{s.item, s.menu, s.target} {
		object.Send(s.app.base.release)
	}
	s.item, s.menu, s.target = 0, 0, 0
	return nil
}
