// Package api exposes the lantern HTTP API, websocket stream, and embedded web UI.
package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/Meru143/lantern/internal/bus"
	webassets "github.com/Meru143/lantern/web"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/websocket"
)

var (
	serverMu   sync.Mutex
	httpServer *http.Server
)

type serverState struct {
	db       *sql.DB
	eventBus *bus.EventBus
	upgrader websocket.Upgrader
}

func newServerState(db *sql.DB, eventBus *bus.EventBus) *serverState {
	return &serverState{
		db:       db,
		eventBus: eventBus,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(*http.Request) bool {
				return true
			},
		},
	}
}

// Start starts the lantern HTTP API and embedded web UI server.
func Start(ctx context.Context, db *sql.DB, eventBus *bus.EventBus, port int) error {
	state := newServerState(db, eventBus)
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		if isAddressInUse(err) {
			return fmt.Errorf("LT002: Port %d is already in use. Use --ui-port to specify a different port.", port)
		}
		return err
	}

	server := &http.Server{
		Addr:    listener.Addr().String(),
		Handler: state.routes(),
	}

	serverMu.Lock()
	httpServer = server
	serverMu.Unlock()

	go func() {
		_ = server.Serve(listener)
	}()

	return nil
}

// Shutdown stops the HTTP API server if it is running.
func Shutdown(ctx context.Context) error {
	serverMu.Lock()
	server := httpServer
	httpServer = nil
	serverMu.Unlock()

	if server == nil {
		return nil
	}

	return server.Shutdown(ctx)
}

func (s *serverState) routes() http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)
	router.Use(middleware.Compress(5, "text/html", "text/css", "application/javascript", "application/json", "image/svg+xml"))
	router.Use(corsMiddleware)
	router.Use(securityHeadersMiddleware)

	router.Route("/api", func(api chi.Router) {
		api.Get("/traces", s.listTraces)
		api.Get("/traces/{traceId}", s.getTrace)
		api.Get("/logs", s.listLogs)
		api.Get("/services", s.listServices)
		api.Get("/errors", s.listErrors)
		api.Get("/errors/{fingerprint}", s.getError)
		api.Get("/queries", s.listQueries)
		api.Get("/stream", s.streamEvents)
	})

	router.Get("/", s.serveIndex)
	router.Get("/about", s.servePage("about.html", true))
	router.Get("/contact", s.servePage("contact.html", true))
	router.Get("/privacy", s.servePage("privacy.html", true))
	router.Get("/robots.txt", s.servePage("robots.txt", true))
	router.Get("/sitemap.xml", s.serveSitemap)

	router.Get("/app.js", s.servePage("app.js", false))
	router.Get("/style.css", s.servePage("style.css", false))
	router.Get("/favicon.svg", s.servePage("favicon.svg", false))

	return router
}

func (s *serverState) serveIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=300")
	http.ServeFileFS(w, r, webassets.Files, "index.html")
}

func (s *serverState) servePage(path string, html bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if html {
			w.Header().Set("Cache-Control", "public, max-age=300")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
		}
		http.ServeFileFS(w, r, webassets.Files, path)
	}
}

type urlSet struct {
	XMLName xml.Name  `xml:"urlset"`
	Xmlns   string    `xml:"xmlns,attr"`
	URLs    []siteURL `xml:"url"`
}

type siteURL struct {
	Loc string `xml:"loc"`
}

func (s *serverState) serveSitemap(w http.ResponseWriter, r *http.Request) {
	baseURL := "http://" + r.Host
	payload := urlSet{
		Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs: []siteURL{
			{Loc: baseURL + "/"},
			{Loc: baseURL + "/about"},
			{Loc: baseURL + "/contact"},
			{Loc: baseURL + "/privacy"},
		},
	}

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	if _, err := w.Write([]byte(xml.Header)); err != nil {
		return
	}
	_ = xml.NewEncoder(w).Encode(payload)
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func parsePositiveInt(raw string, fallback int) (int, error) {
	if raw == "" {
		return fallback, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	if value < 0 {
		return 0, fmt.Errorf("value must be >= 0")
	}
	return value, nil
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; connect-src 'self' ws: wss:; base-uri 'self'; form-action 'self'; frame-ancestors 'none'")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}

func isAddressInUse(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "address already in use") || strings.Contains(message, "only one usage of each socket address")
}
