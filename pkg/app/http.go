package app

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/maxence-charriere/go-app/v9/pkg/errors"
)

const (
	defaultThemeColor = "#2d2c2c"
)

// Handler is an HTTP handler that serves an HTML page that loads a Go wasm app
// and its resources.
type Handler struct {
	// The name of the web application as it is usually displayed to the user.
	Name string

	// The name of the web application displayed to the user when there is not
	// enough space to display Name.
	ShortName string

	// The icon that is used for the PWA, favicon, loading and default not
	// found component.
	Icon Icon

	// A placeholder background color for the application page to display before
	// its stylesheets are loaded.
	//
	// Default: #2d2c2c.
	BackgroundColor string

	// The theme color for the application. This affects how the OS displays the
	// app (e.g., PWA title bar or Android's task switcher).
	//
	// DEFAULT: #2d2c2c.
	ThemeColor string

	// The text displayed while loading a page. Load progress can be inserted by
	// including "{progress}" in the loading label.
	//
	// DEFAULT: "{progress}%".
	LoadingLabel string

	// The page language.
	//
	// DEFAULT: en.
	Lang string

	// The custom libraries to load with the page.
	Libraries []Library

	// The page title.
	Title string

	// The page description.
	Description string

	// Domain specifies the domain name of the server. It is primarily used
	// to resolve page metadata such as 'go:url', ensuring accurate reference
	// and representation of URLs within the application.
	Domain string

	// The page authors.
	Author string

	// The page keywords.
	Keywords []string

	// The path of the default image that is used by social networks when
	// linking the app.
	Image string

	// The paths or urls of the CSS files to use with the page.
	//
	// eg:
	//  app.Handler{
	//      Styles: []string{
	//          "/web/test.css",            // Static resource
	//          "https://foo.com/test.css", // External resource
	//      },
	//  },
	Styles []string

	// The paths or urls of the font files to preload with the page.
	//
	// eg:
	//  app.Handler{
	//      Fonts: []string{
	//          "/web/test.woff2",            // Static resource
	//          "https://foo.com/test.woff2", // External resource
	//      },
	//  },
	Fonts []string

	// The paths or urls of the JavaScript files to use with the page.
	//
	// eg:
	//  app.Handler{
	//      Scripts: []string{
	//          "/web/test.js",            // Static resource
	//          "https://foo.com/test.js", // External resource
	//      },
	//  },
	Scripts []string

	// The path of the static resources that the browser is caching in order to
	// provide offline mode.
	//
	// Note that Icon, Styles and Scripts are already cached by default.
	//
	// Paths are relative to the root directory.
	CacheableResources []string

	// Additional headers to be added in head element.
	RawHeaders []string

	// The page HTML element.
	//
	// Default: Html().
	HTML func() HTMLHtml

	// The page body element.
	//
	// Note that the lang attribute is always overridden by the Handler.Lang
	// value.
	//
	// Default: Body().
	Body func() HTMLBody

	// The interval between each app auto-update while running in a web browser.
	// Zero or negative values deactivates the auto-update mechanism.
	//
	// Default is 0.
	AutoUpdateInterval time.Duration

	// The environment variables that are passed to the progressive web app.
	//
	// Reserved keys:
	// - GOAPP_VERSION
	// - GOAPP_GOAPP_STATIC_RESOURCES_URL
	Env Environment

	// The URLs that are launched in the app tab or window.
	//
	// By default, URLs with a different domain are launched in another tab.
	// Specifying internal URLs is to override that behavior. A good use case
	// would be the URL for an OAuth authentication.
	InternalURLs []string

	// The URLs of the origins to preconnect in order to improve the user
	// experience by preemptively initiating a connection to those origins.
	// Preconnecting speeds up future loads from a given origin by preemptively
	// performing part or all of the handshake (DNS+TCP for HTTP, and
	// DNS+TCP+TLS for HTTPS origins).
	Preconnect []string

	// The static resources that are accessible from custom paths. Files that
	// are proxied by default are /robots.txt, /sitemap.xml and /ads.txt.
	ProxyResources []ProxyResource

	// Resources is a ResourceResolver responsible for resolving static resource
	// paths. It specifically handles paths that begin with "/web/", ensuring that
	// static resources such as stylesheets, scripts, and images are correctly
	// located and served.
	//
	// For example, a resource path like "/web/main.css" will be resolved to its
	// full path or URL by the ResourceResolver.
	//
	// Default: LocalDir("")
	Resources ResourceResolver

	// The version number. This is used in order to update the PWA application
	// in the browser. It must be set when deployed on a live system in order to
	// prevent recurring updates.
	//
	// Default: Auto-generated in order to trigger pwa update on a local
	// development system.
	Version string

	// WasmContentLength specifies the length, in bytes, of the WebAssembly (WASM)
	// file. This length is used to calculate the loading progress when serving the
	// WASM binary.
	//
	// If this field is not set, the handler will attempt to use the
	// WasmContentLengthHeader to determine the content length.
	WasmContentLength string

	// WasmContentLengthHeader defines the HTTP header used to obtain the content
	// length of the WebAssembly file. This is used as a fallback mechanism to
	// determine the loading progress if WasmContentLength is not set.
	//
	// The default fallback HTTP header is "Content-Length".
	WasmContentLengthHeader string

	// The template used to generate app-worker.js. The template follows the
	// text/template package model.
	//
	// By default set to DefaultAppWorkerJS, changing the template have very
	// high chances to mess up go-app usage. Any issue related to a custom app
	// worker template is not supported and will be closed.
	ServiceWorkerTemplate string

	once                 sync.Once
	etag                 string
	libraries            map[string][]byte
	proxyResources       map[string]ProxyResource
	cachedProxyResources *memoryCache
	cachedPWAResources   *memoryCache
}

func (h *Handler) init() {
	h.initVersion()
	h.initStaticResources()
	h.initLibraries()
	h.initLinks()
	h.initServiceWorker()
	h.initIcon()
	h.initPWA()
	h.initPageContent()
	h.initPWAResources()
	h.initProxyResources()
}

func (h *Handler) initVersion() {
	if h.Version == "" {
		t := time.Now().UTC().String()
		h.Version = fmt.Sprintf(`%x`, sha1.Sum([]byte(t)))
	}
	h.etag = `"` + h.Version + `"`
}

func (h *Handler) initStaticResources() {
	if h.Resources == nil {
		h.Resources = LocalDir("")
	}
}

func (h *Handler) initLibraries() {
	libs := make(map[string][]byte)
	for _, l := range h.Libraries {
		path, styles := l.Styles()
		if !strings.HasPrefix(path, "/") || len(styles) == 0 {
			continue
		}
		libs[path] = []byte(styles)
	}
	h.libraries = libs
}

func (h *Handler) initLinks() {
	styles := []string{"/app.css"}
	for path := range h.libraries {
		styles = append(styles, path)
	}
	h.Styles = append(styles, h.Styles...)
}

func (h *Handler) initServiceWorker() {
	if h.ServiceWorkerTemplate == "" {
		h.ServiceWorkerTemplate = DefaultAppWorkerJS
	}
}

func (h *Handler) initIcon() {
	if h.Icon.Default == "" {
		h.Icon.Default = "https://raw.githubusercontent.com/maxence-charriere/go-app/master/docs/web/icon.png"
		h.Icon.Large = "https://raw.githubusercontent.com/maxence-charriere/go-app/master/docs/web/icon.png"
	}

	if h.Icon.AppleTouch == "" {
		h.Icon.AppleTouch = h.Icon.Default
	}

	if h.Icon.SVG == "" {
		h.Icon.SVG = "https://raw.githubusercontent.com/maxence-charriere/go-app/master/docs/web/icon.svg"
	}
}

func (h *Handler) initPWA() {
	if h.Name == "" && h.ShortName == "" && h.Title == "" {
		h.Name = "App PWA"
	}
	if h.ShortName == "" {
		h.ShortName = h.Name
	}
	if h.Name == "" {
		h.Name = h.ShortName
	}

	if h.BackgroundColor == "" {
		h.BackgroundColor = defaultThemeColor
	}
	if h.ThemeColor == "" {
		h.ThemeColor = defaultThemeColor
	}

	if h.Lang == "" {
		h.Lang = "en"
	}

	if h.LoadingLabel == "" {
		h.LoadingLabel = "{progress}%"
	}
}

func (h *Handler) initPageContent() {
	if h.HTML == nil {
		h.HTML = Html
	}

	if h.Body == nil {
		h.Body = Body
	}

}

func (h *Handler) initPWAResources() {
	h.cachedPWAResources = newMemoryCache(5)

	h.cachedPWAResources.Set(cacheItem{
		Path:        "/wasm_exec.js",
		ContentType: "application/javascript",
		Body:        []byte(wasmExecJS()),
	})

	h.cachedPWAResources.Set(cacheItem{
		Path:        "/app.js",
		ContentType: "application/javascript",
		Body:        h.makeAppJS(),
	})

	h.cachedPWAResources.Set(cacheItem{
		Path:        "/app-worker.js",
		ContentType: "application/javascript",
		Body:        h.makeAppWorkerJS(),
	})

	h.cachedPWAResources.Set(cacheItem{
		Path:        "/manifest.webmanifest",
		ContentType: "application/manifest+json",
		Body:        h.makeManifestJSON(),
	})

	h.cachedPWAResources.Set(cacheItem{
		Path:        "/app.css",
		ContentType: "text/css",
		Body:        []byte(appCSS),
	})
}

func (h *Handler) makeAppJS() []byte {
	if h.Env == nil {
		h.Env = make(map[string]string)
	}
	internalURLs, _ := json.Marshal(h.InternalURLs)
	h.Env["GOAPP_INTERNAL_URLS"] = string(internalURLs)
	h.Env["GOAPP_VERSION"] = h.Version
	h.Env["GOAPP_STATIC_RESOURCES_URL"] = h.Resources.Resolve("/web")
	h.Env["GOAPP_ROOT_PREFIX"] = h.Resources.Resolve("/")

	for k, v := range h.Env {
		if err := os.Setenv(k, v); err != nil {
			Log(errors.New("setting app env variable failed").
				WithTag("name", k).
				WithTag("value", v).
				Wrap(err))
		}
	}

	var b bytes.Buffer
	if err := template.
		Must(template.New("app.js").Parse(appJS)).
		Execute(&b, struct {
			Env                     string
			LoadingLabel            string
			Wasm                    string
			WasmContentLength       string
			WasmContentLengthHeader string
			WorkerJS                string
			AutoUpdateInterval      int64
		}{
			Env:                     jsonString(h.Env),
			LoadingLabel:            h.LoadingLabel,
			Wasm:                    h.Resources.Resolve("/web/app.wasm"),
			WasmContentLength:       h.WasmContentLength,
			WasmContentLengthHeader: h.WasmContentLengthHeader,
			WorkerJS:                h.Resources.Resolve("/app-worker.js"),
			AutoUpdateInterval:      h.AutoUpdateInterval.Milliseconds(),
		}); err != nil {
		panic(errors.New("initializing app.js failed").Wrap(err))
	}
	return b.Bytes()
}

func (h *Handler) makeAppWorkerJS() []byte {
	resources := make(map[string]struct{})
	setResources := func(res ...string) {
		for _, r := range res {
			if resource := parseHTTPResource(r); resource.URL != "" {
				resources[resource.URL] = struct{}{}
			}
		}
	}
	setResources(
		"/app.css",
		"/app.js",
		"/manifest.webmanifest",
		"/wasm_exec.js",
		"/",
		"/web/app.wasm",
	)
	setResources(h.Icon.Default, h.Icon.Large, h.Icon.AppleTouch)
	setResources(h.Styles...)
	setResources(h.Fonts...)
	setResources(h.Scripts...)
	setResources(h.CacheableResources...)

	resourcesTocache := make([]string, 0, len(resources))
	for k := range resources {
		resourcesTocache = append(resourcesTocache, h.Resources.Resolve(k))
	}
	sort.Slice(resourcesTocache, func(a, b int) bool {
		return strings.Compare(resourcesTocache[a], resourcesTocache[b]) < 0
	})

	var b bytes.Buffer
	if err := template.
		Must(template.New("app-worker.js").Parse(h.ServiceWorkerTemplate)).
		Execute(&b, struct {
			Version          string
			ResourcesToCache string
		}{
			Version:          h.Version,
			ResourcesToCache: jsonString(resourcesTocache),
		}); err != nil {
		panic(errors.New("initializing app-worker.js failed").Wrap(err))
	}
	return b.Bytes()
}

func (h *Handler) makeManifestJSON() []byte {
	scope := h.Resources.Resolve("/")
	if scope != "/" && !strings.HasSuffix(scope, "/") {
		scope += "/"
	}

	var b bytes.Buffer
	if err := template.
		Must(template.New("manifest.webmanifest").Parse(manifestJSON)).
		Execute(&b, struct {
			ShortName       string
			Name            string
			Description     string
			DefaultIcon     string
			LargeIcon       string
			SVGIcon         string
			BackgroundColor string
			ThemeColor      string
			Scope           string
			StartURL        string
		}{
			ShortName:       h.ShortName,
			Name:            h.Name,
			Description:     h.Description,
			DefaultIcon:     h.Resources.Resolve(h.Icon.Default),
			LargeIcon:       h.Resources.Resolve(h.Icon.Large),
			SVGIcon:         h.Resources.Resolve(h.Icon.SVG),
			BackgroundColor: h.BackgroundColor,
			ThemeColor:      h.ThemeColor,
			Scope:           scope,
			StartURL:        h.Resources.Resolve("/"),
		}); err != nil {
		panic(errors.New("initializing manifest.webmanifest failed").Wrap(err))
	}
	return b.Bytes()
}

func (h *Handler) initProxyResources() {
	h.cachedProxyResources = newMemoryCache(len(h.ProxyResources))
	resources := make(map[string]ProxyResource)

	for _, r := range h.ProxyResources {
		switch r.Path {
		case "/wasm_exec.js",
			"/goapp.js",
			"/app.js",
			"/app-worker.js",
			"/manifest.json",
			"/manifest.webmanifest",
			"/app.css",
			"/app.wasm",
			"/goapp.wasm",
			"/":
			continue

		default:
			if strings.HasPrefix(r.Path, "/") && strings.HasPrefix(r.ResourcePath, "/web/") {
				resources[r.Path] = r
			}
		}
	}

	if _, ok := resources["/robots.txt"]; !ok {
		resources["/robots.txt"] = ProxyResource{
			Path:         "/robots.txt",
			ResourcePath: "/web/robots.txt",
		}
	}
	if _, ok := resources["/sitemap.xml"]; !ok {
		resources["/sitemap.xml"] = ProxyResource{
			Path:         "/sitemap.xml",
			ResourcePath: "/web/sitemap.xml",
		}
	}
	if _, ok := resources["/ads.txt"]; !ok {
		resources["/ads.txt"] = ProxyResource{
			Path:         "/ads.txt",
			ResourcePath: "/web/ads.txt",
		}
	}

	h.proxyResources = resources
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.once.Do(h.init)

	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", h.etag)

	etag := r.Header.Get("If-None-Match")
	if etag == h.etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	path := r.URL.Path
	if strings.HasPrefix(path, "/"+h.Version+"/") {
		path = strings.TrimPrefix(path, "/"+h.Version)
	}

	fileHandler, isServingStaticResources := h.Resources.(http.Handler)
	if isServingStaticResources && strings.HasPrefix(path, "/web/") {
		fileHandler.ServeHTTP(w, r)
		return
	}

	switch path {
	case "/goapp.js":
		path = "/app.js"

	case "/manifest.json":
		path = "/manifest.webmanifest"

	case "/app.wasm", "/goapp.wasm":
		if isServingStaticResources {
			r2 := *r
			r2.URL.Path = h.Resources.Resolve("/web/app.wasm")
			fileHandler.ServeHTTP(w, &r2)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		return

	}

	if res, ok := h.cachedPWAResources.Get(path); ok {
		h.serveCachedItem(w, res)
		return
	}

	if proxyResource, ok := h.proxyResources[path]; ok {
		h.serveProxyResource(proxyResource, w, r)
		return
	}

	if library, ok := h.libraries[path]; ok {
		h.serveLibrary(w, r, library)
		return
	}

	h.servePage(w, r)
}

func (h *Handler) serveCachedItem(w http.ResponseWriter, i cacheItem) {
	w.Header().Set("Content-Length", strconv.Itoa(i.Len()))
	w.Header().Set("Content-Type", i.ContentType)

	if i.ContentEncoding != "" {
		w.Header().Set("Content-Encoding", i.ContentEncoding)
	}

	w.WriteHeader(http.StatusOK)
	w.Write(i.Body)
}

func (h *Handler) serveProxyResource(resource ProxyResource, w http.ResponseWriter, r *http.Request) {
	var u string
	if _, ok := h.Resources.(http.Handler); ok {
		var protocol string
		if r.TLS != nil {
			protocol = "https://"
		} else {
			protocol = "http://"
		}
		u = protocol + r.Host + resource.ResourcePath
	} else {
		u = h.Resources.Resolve(resource.ResourcePath)
	}

	if i, ok := h.cachedProxyResources.Get(resource.Path); ok {
		h.serveCachedItem(w, i)
		return
	}

	res, err := http.Get(u)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		Log(errors.New("getting proxy static resource failed").
			WithTag("url", u).
			WithTag("proxy-path", resource.Path).
			WithTag("static-resource-path", resource.ResourcePath).
			Wrap(err),
		)
		return
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		Log(errors.New("reading proxy static resource failed").
			WithTag("url", u).
			WithTag("proxy-path", resource.Path).
			WithTag("static-resource-path", resource.ResourcePath).
			Wrap(err),
		)
		return
	}

	item := cacheItem{
		Path:            resource.Path,
		ContentType:     res.Header.Get("Content-Type"),
		ContentEncoding: res.Header.Get("Content-Encoding"),
		Body:            body,
	}
	h.cachedProxyResources.Set(item)
	h.serveCachedItem(w, item)
}

func (h *Handler) servePage(w http.ResponseWriter, r *http.Request) {
	if routed := routes.routed(r.URL.Path); !routed {
		http.NotFound(w, r)
		return
	}

	ctx := context.Background()

	origin := *r.URL
	origin.Scheme = "http"

	page := makeRequestPage(&origin, h.Resources.Resolve)
	page.SetTitle(h.Title)
	page.SetLang(h.Lang)
	page.SetDescription(h.Description)
	page.SetAuthor(h.Author)
	page.SetKeywords(h.Keywords...)
	page.SetLoadingLabel(strings.ReplaceAll(h.LoadingLabel, "{progress}", "0"))
	page.SetImage(h.Image)

	engine := newEngine(ctx,
		&routes,
		h.Resources.Resolve,
		&page,
		actionHandlers,
	)
	engine.Navigate(page.URL(), false)
	engine.ConsumeAll()

	icon := h.Icon.SVG
	if icon == "" {
		icon = h.Icon.Default
	}

	var b bytes.Buffer
	err := engine.Encode(&b, h.HTML().
		Lang(page.Lang()).
		privateBody(
			Head().Body(
				Meta().Charset("UTF-8"),
				Meta().
					Name("author").
					Content(page.Author()),
				Meta().
					Name("description").
					Content(page.Description()),
				If(page.Keywords() != "", func() UI {
					return Meta().
						Name("keywords").
						Content(page.Keywords())
				}),
				Meta().
					Name("theme-color").
					Content(h.ThemeColor),
				Meta().
					Name("viewport").
					Content("width=device-width, initial-scale=1, maximum-scale=1, user-scalable=0, viewport-fit=cover"),
				Meta().
					Property("og:url").
					Content(resolveOGResource(h.Domain, h.Resources.Resolve(page.URL().Path))),
				Meta().
					Property("og:title").
					Content(page.Title()),
				Meta().
					Property("og:description").
					Content(page.Description()),
				Meta().
					Property("og:type").
					Content("website"),
				Meta().
					Property("og:image").
					Content(resolveOGResource(h.Domain, page.Image())),
				Range(page.twitterCardMap).Map(func(k string) UI {
					v := page.twitterCardMap[k]
					if v == "" {
						return nil
					}
					if k == "twitter:image" {
						v = resolveOGResource(h.Domain, v)
					}
					return Meta().
						Name(k).
						Content(v)
				}),
				Title().Text(page.Title()),
				Range(h.Preconnect).Slice(func(i int) UI {
					if resource := parseHTTPResource(h.Preconnect[i]); resource.URL != "" {
						return resource.toLink().Rel("preconnect")
					}
					return nil
				}),
				Range(h.Fonts).Slice(func(i int) UI {
					if resource := parseHTTPResource(h.Fonts[i]); resource.URL != "" {
						return resource.toLink().
							Type("font/" + strings.TrimPrefix(filepath.Ext(resource.URL), ".")).
							Rel("preload").
							As("font")
					}
					return nil
				}),
				Range(page.Preloads()).Slice(func(i int) UI {
					p := page.Preloads()[i]
					if p.Href == "" || p.As == "" {
						return nil
					}

					if resource := parseHTTPResource(p.Href); resource.URL != "" {
						return resource.toLink().
							Type(p.Type).
							Rel("preload").
							As(p.As).
							FetchPriority(p.FetchPriority)
					}
					return nil
				}),
				Range(h.Styles).Slice(func(i int) UI {
					if resource := parseHTTPResource(h.Styles[i]); resource.URL != "" {
						return resource.toLink().
							Type("text/css").
							Rel("preload").
							As("style")
					}
					return nil
				}),
				Link().
					Rel("icon").
					Href(icon),
				Link().
					Rel("apple-touch-icon").
					Href(h.Icon.AppleTouch),
				Link().
					Rel("manifest").
					Href("/manifest.webmanifest"),
				Range(h.Styles).Slice(func(i int) UI {
					if resource := parseHTTPResource(h.Styles[i]); resource.URL != "" {
						return resource.toLink().
							Type("text/css").
							Rel("stylesheet")
					}
					return nil
				}),
				Script().
					Defer(true).
					Src("/wasm_exec.js"),
				Script().
					Defer(true).
					Src("/app.js"),
				Range(h.Scripts).Slice(func(i int) UI {
					if resource := parseHTTPResource(h.Scripts[i]); resource.URL != "" {
						return resource.toScript()
					}
					return nil

				}),
				Range(h.RawHeaders).Slice(func(i int) UI {
					return Raw(h.RawHeaders[i])
				}),
			),
			h.Body().privateBody(
				Aside().
					ID("app-wasm-loader").
					Class("goapp-app-info").
					Body(
						Img().
							ID("app-wasm-loader-icon").
							Class("goapp-logo goapp-spin").
							Src(h.Icon.Default),
						P().
							ID("app-wasm-loader-label").
							Class("goapp-label").
							Text(page.loadingLabel),
					),
			),
		))
	if err != nil {
		Log(errors.New("encoding html document failed").Wrap(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Length", strconv.Itoa(b.Len()))
	w.Header().Set("Content-Type", "text/html")
	w.Write(b.Bytes())
}

func (h *Handler) serveLibrary(w http.ResponseWriter, r *http.Request, library []byte) {
	w.Header().Set("Content-Length", strconv.Itoa(len(library)))
	w.Header().Set("Content-Type", "text/css")
	w.Write(library)
}

// Icon describes a square image that is used in various places such as
// application icon, favicon or loading icon.
type Icon struct {
	// The path or url to a square image/png file. It must have a side of 192px.
	//
	// Path is relative to the root directory.
	Default string

	// The path or url to larger square image/png file. It must have a side of
	// 512px.
	//
	// Path is relative to the root directory.
	Large string

	// The path or url to a svg file.
	SVG string

	// The path or url to a square image/png file that is used for IOS/IPadOS
	// home screen icon. It must have a side of 192px.
	//
	// Path is relative to the root directory.
	//
	// DEFAULT: Icon.Default
	AppleTouch string
}

// Environment describes the environment variables to pass to the progressive
// web app.
type Environment map[string]string

func isRemoteLocation(path string) bool {
	return strings.HasPrefix(path, "https://") ||
		strings.HasPrefix(path, "http://")
}

func isStaticResourcePath(path string) bool {
	return strings.HasPrefix(path, "/web/") ||
		strings.HasPrefix(path, "web/")
}

// func parseSrc(link string) (url, crossOrigin, loading string) {
// 	for _, p := range strings.Split(link, " ") {
// 		p = strings.TrimSpace(p)
// 		if p == "" {
// 			continue
// 		}

// 		switch {
// 		case p == "crossorigin":
// 			crossOrigin = "true"

// 		case strings.HasPrefix(p, "crossorigin="):
// 			crossOrigin = strings.TrimPrefix(p, "crossorigin=")

// 		case p == "defer":
// 			loading = "defer"

// 		case p == "async":
// 			loading = "async"

// 		default:
// 			url = p
// 		}
// 	}

// 	return url, crossOrigin, loading
// }

// ResourceString represents a string that encapsulates a resource URL along with attributes
// specifying how to load it. This includes attributes like 'async', 'defer', and 'crossorigin'.
// The type is used to configure resource loading behavior in HTTP Handlers.
//
// Examples of ResourceString values include:
//   - "https://hello.world async"
//   - "https://hello.world defer"
//   - "https://hello.world crossorigin"
//   - "https://hello.world crossorigin=anonymous"
//   - "https://hello.world async crossorigin"
type ResourceString string

type httpResource struct {
	URL         string
	LoadingMode string
	CrossOrigin string
}

func (r httpResource) toLink() HTMLLink {
	link := Link().Href(r.URL)
	if r.CrossOrigin != "" {
		link = link.CrossOrigin(strings.Trim(r.CrossOrigin, "true"))
	}
	return link
}

func (r httpResource) toScript() HTMLScript {
	script := Script().Src(r.URL)
	if r.CrossOrigin != "" {
		script = script.CrossOrigin(strings.Trim(r.CrossOrigin, "true"))
	}

	switch r.LoadingMode {
	case "defer":
		script = script.Defer(true)

	case "async":
		script = script.Async(true)
	}
	return script
}

func parseHTTPResource(v string) httpResource {
	var res httpResource
	for _, elem := range strings.Split(v, " ") {
		if elem = strings.TrimSpace(elem); elem == "" {
			continue
		}
		elem = strings.ToLower(elem)

		switch {
		case elem == "crossorigin":
			res.CrossOrigin = "true"

		case strings.HasPrefix(elem, "crossorigin="):
			res.CrossOrigin = strings.TrimPrefix(elem, "crossorigin=")

		case elem == "defer":
			res.LoadingMode = "defer"

		case elem == "async":
			res.LoadingMode = "async"

		default:
			res.URL = elem
		}
	}
	return res
}
