// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package router

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/netberth/netberth/internal/api/handler"
	custommw "github.com/netberth/netberth/internal/api/middleware"
	ws "github.com/netberth/netberth/internal/api/websocket"
	"github.com/netberth/netberth/internal/auth"
	"github.com/netberth/netberth/internal/service"
	"github.com/netberth/netberth/internal/tenant"
)

// Options configures the HTTP router. Zero values fall back to safe defaults.
type Options struct {
	TrustedProxies    []string
	RateLimitRate     int
	RateLimitBurst    int
	WebhookDispatcher *service.WebhookDispatcher
}

func New(db *sql.DB, authService *auth.Service, wire *service.Wire, hub *ws.Hub, opts ...Options) http.Handler {
	cfg := Options{RateLimitRate: 100, RateLimitBurst: 200}
	if len(opts) > 0 {
		cfg = opts[0]
	}
	if cfg.RateLimitRate < 1 {
		cfg.RateLimitRate = 100
	}
	if cfg.RateLimitBurst < 1 {
		cfg.RateLimitBurst = 200
	}
	resolver, err := custommw.NewClientIPResolver(cfg.TrustedProxies)
	if err != nil {
		// config.Load validates trusted proxies before the router is built;
		// reaching this with an invalid list is a programming error.
		panic(err)
	}

	r := chi.NewRouter()

	r.Use(chimw.RequestID)
	r.Use(resolver.Middleware)
	r.Use(tenant.Middleware(&tenant.SingleTenantProvider{}))
	r.Use(custommw.SecurityHeaders)
	r.Use(custommw.CORSMiddleware)
	r.Use(custommw.LoggingMiddleware)
	r.Use(custommw.NewRateLimiter(cfg.RateLimitRate, cfg.RateLimitBurst).Middleware)
	r.Use(chimw.Recoverer)
	r.Use(custommw.AuditMiddleware(db))

	bruteForce := custommw.NewBruteForceLimiter(5, 5*time.Minute, 10*time.Minute)

	bus := wire.Bus()

	authH := handler.NewAuthHandler(db, authService)
	userH := handler.NewUserHandler(db, authService)
	webhookH := handler.NewWebhookHandler(db, cfg.WebhookDispatcher)
	auditH := handler.NewAuditHandler(db)
	forwardH := handler.NewForwardHandler(db)
	proxyH := handler.NewProxyHandler(db)
	ddnsH := handler.NewDDNSHandler(db)
	stunH := handler.NewSTUNHandler(db)
	wolH := handler.NewWOLHandler(db)
	cronH := handler.NewCronHandler(db)
	acmeH := handler.NewACMEHandler(db)
	storageH := handler.NewStorageHandler(db)

	// Connect notifiers to event bus
	forwardH.SetNotifier(busNotifier(bus, "forward"))
	proxyH.SetNotifier(busNotifier(bus, "proxy"))
	ddnsH.SetNotifier(busNotifier(bus, "ddns"))
	stunH.SetNotifier(busNotifier(bus, "stun"))
	cronH.SetNotifier(busNotifier(bus, "cron"))
	acmeH.SetNotifier(busNotifier(bus, "acme"))
	storageH.SetNotifier(busNotifier(bus, "storage"))

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(custommw.LimitRequestBody)

		// Public endpoints — no auth required
		r.Get("/system/status", handler.NewSystemHandler(db).Status)
		r.Get("/system/metrics", handler.NewMetricsHandler(db, wire.Forward).Metrics)
		r.With(bruteForce.LoginMiddleware).Post("/auth/login", authH.Login)
		r.Get("/docs", handler.DocsHandler())
		r.Post("/auth/refresh", authH.RefreshToken)

		r.Get("/ws", func(w http.ResponseWriter, r *http.Request) {
			// WebSocket with optional auth — check token if provided
			hub.HandleWS(w, r)
		})

		// Protected endpoints
		r.Group(func(r chi.Router) {
			r.Use(custommw.AuthMiddleware(authService))
			r.Use(custommw.ForcePasswordChange(db))
			r.Get("/auth/2fa/setup", authH.Setup2FA)
			r.Post("/auth/2fa/enable", authH.Enable2FA)
			r.Post("/auth/2fa/disable", authH.Disable2FA)
			r.Get("/license/status", handler.NewLicenseHandler(db).Status)
			r.Post("/license/activate", handler.NewLicenseHandler(db).Activate)

			r.Get("/auth/me", authH.Me)
			r.Post("/auth/change-password", authH.ChangePassword)
			r.Post("/auth/logout", authH.Logout)

			r.Get("/system/dashboard", handler.NewSystemHandler(db).Dashboard)

			r.Get("/system/backup", handler.NewBackupHandler(db).Download)
			r.Post("/system/restore", handler.NewBackupHandler(db).Restore)

			registerCRUD(r, "/forward-rules", forwardH)
			registerCRUD(r, "/proxy-rules", proxyH)
			registerCRUD(r, "/ddns", ddnsH)
			registerCRUD(r, "/stun", stunH)
			registerCRUD(r, "/wol", wolH)
			r.Post("/wol/{id}/wake", wolH.Wake)
			registerCRUD(r, "/cron", cronH)
			registerCRUD(r, "/acme", acmeH)
			registerCRUD(r, "/storage", storageH)

			// User management is admin-only.
			r.Group(func(r chi.Router) {
				r.Use(custommw.RequireRole("admin"))
				r.Get("/audit", auditH.List)
				r.Get("/users", userH.List)
				r.Post("/users", userH.Create)
				r.Put("/users/{id}", userH.Update)
				r.Delete("/users/{id}", userH.Delete)
				r.Post("/users/{id}/reset-password", userH.ResetPassword)
				r.Get("/webhooks", webhookH.List)
				r.Post("/webhooks", webhookH.Create)
				r.Put("/webhooks/{id}", webhookH.Update)
				r.Delete("/webhooks/{id}", webhookH.Delete)
				r.Post("/webhooks/{id}/test", webhookH.Test)
			})
		})
	})

	// Serve embedded UI for all non-API routes
	uiHandler := handler.UIHandler()
	r.Get("/*", uiHandler.ServeHTTP)

	return r
}

type CRUDLister interface {
	List(w http.ResponseWriter, r *http.Request)
	Create(w http.ResponseWriter, r *http.Request)
	Update(w http.ResponseWriter, r *http.Request)
	Delete(w http.ResponseWriter, r *http.Request)
}

func registerCRUD(r chi.Router, path string, h CRUDLister) {
	r.Get(path, h.List)
	r.Post(path, h.Create)
	r.Put(path+"/{id}", h.Update)
	r.Delete(path+"/{id}", h.Delete)
}

func busNotifier(bus *service.Bus, resource string) *handler.Notifier {
	return &handler.Notifier{
		OnCreate: func(_, id string) { bus.Publish(service.Event{Type: service.EventType(resource + ":created"), ID: id}) },
		OnUpdate: func(_, id string) { bus.Publish(service.Event{Type: service.EventType(resource + ":updated"), ID: id}) },
		OnDelete: func(_, id string) { bus.Publish(service.Event{Type: service.EventType(resource + ":deleted"), ID: id}) },
	}
}
