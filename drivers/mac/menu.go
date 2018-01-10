// +build darwin,amd64

package mac

import (
	"fmt"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/murlokswarm/app"
	"github.com/murlokswarm/app/bridge"
	"github.com/murlokswarm/app/html"
)

// Menu implements the app.Menu interface.
type Menu struct {
	id        uuid.UUID
	markup    app.Markup
	lastFocus time.Time
	component app.Component
}

func newMenu(config app.MenuConfig) (m *Menu, err error) {
	var markup app.Markup = html.NewMarkup(driver.factory)
	markup = app.NewConcurrentMarkup(markup)

	m = &Menu{
		id:        uuid.New(),
		markup:    markup,
		lastFocus: time.Now(),
	}

	if _, err = driver.macos.Request(
		fmt.Sprintf("/menu/new?id=%s", m.id),
		nil,
	); err != nil {
		return
	}

	if err = driver.elements.Add(m); err != nil {
		return
	}

	m.Load(config.DefaultURL)
	return
}

// ID satisfies the app.Element interface.
func (m *Menu) ID() uuid.UUID {
	return m.id
}

// Load satisfies the app.Menu interface.
func (m *Menu) Load(rawurl string, v ...interface{}) error {
	var compoName string
	var compo app.Component
	var root app.Tag

	rawurl = fmt.Sprintf(rawurl, v...)
	u, err := url.Parse(rawurl)
	if err != nil {
		return err
	}

	compoName = app.ComponentNameFromURL(u)
	if compo, err = driver.factory.NewComponent(compoName); err != nil {
		return err
	}

	if m.component != nil {
		m.markup.Dismount(m.component)
	}

	if root, err = m.markup.Mount(compo); err != nil {
		return err
	}
	m.component = compo

	_, err = driver.macos.RequestWithAsyncResponse(
		fmt.Sprintf("/menu/load?id=%s", m.id),
		bridge.NewPayload(root),
	)
	return err
}

// Contains satisfies the app.Menu interface.
func (m *Menu) Contains(compo app.Component) bool {
	return m.markup.Contains(compo)
}

// Render satisfies the app.Menu interface.
func (m *Menu) Render(compo app.Component) error {
	panic("not implemented")
}

// LastFocus satisfies the app.Menu interface.
func (m *Menu) LastFocus() time.Time {
	return m.lastFocus
}

func onMenuCallback(m *Menu, u *url.URL, p bridge.Payload) (res bridge.Payload) {
	var mapping app.Mapping
	p.Unmarshal(&mapping)

	function, err := m.markup.Map(mapping)
	if err != nil {
		app.DefaultLogger.Error(err)
		return
	}

	if function != nil {
		function()
		return
	}

	var compo app.Component
	if compo, err = m.markup.Component(mapping.CompoID); err != nil {
		app.DefaultLogger.Error(err)
		return
	}

	if err = m.Render(compo); err != nil {
		app.DefaultLogger.Error(err)
	}
	return
}

// DefaultMenuBar is a component that describes a menu bar.
// It is loaded by default if Driver.MenubarURL is not set.
type DefaultMenuBar struct {
	AppName string
}

// Render returns the markup that describes the menu bar.
func (m *DefaultMenuBar) Render() string {
	return `
<menu>
	<menu label="app">
		<menuitem label="About" selector="orderFrontStandardAboutPanel:"></menuitem>
		<menuitem separator></menuitem>	
		<menuitem label="Preferences…" keys="cmdorctrl+," disabled></menuitem>
		<menuitem separator></menuitem>		
		<menuitem label="Hide" keys="cmdorctrl+h" selector="hide:"></menuitem>
		<menuitem label="Hide Others" keys="cmdorctrl+alt+h" selector="hideOtherApplications:"></menuitem>
		<menuitem label="Show All" selector="unhideAllApplications:"></menuitem>
		<menuitem separator></menuitem>
		<menuitem label="Quit" keys="cmdorctrl+q" selector="terminate:"></menuitem>
		<menuitem label="Pouette" onclick="Pouette"></menuitem>
	</menu>
</menu>
	`
}

func (m *DefaultMenuBar) Pouette() {
	fmt.Println("Pouette:")
}

func init() {
	app.Import(&DefaultMenuBar{})
}
