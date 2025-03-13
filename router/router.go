package router

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/AsterZephyr/Scree-go-AZlearn/auth"
	"github.com/AsterZephyr/Scree-go-AZlearn/config"
	"github.com/AsterZephyr/Scree-go-AZlearn/ui"
	"github.com/AsterZephyr/Scree-go-AZlearn/ws"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/hlog"
	"github.com/rs/zerolog/log"
)

type Health struct {
	Status  string `json:"status"`
	Clients int    `json:"clients"`
	Reason  string `json:"reason,omitempty"`
}

type UIConfig struct {
	AuthMode                 string `json:"authMode"`
	User                     string `json:"user"`
	LoggedIn                 bool   `json:"loggedIn"`
	Version                  string `json:"version"`
	RoomName                 string `json:"roomName"`
	CloseRoomWhenOwnerLeaves bool   `json:"closeRoomWhenOwnerLeaves"`
}

func Router(conf config.Config, rooms *ws.Rooms, users *auth.Users, version string) *mux.Router {
	router := mux.NewRouter()
	router.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// https://github.com/gorilla/mux/issues/416
		accessLogger(r, 404, 0, 0)
	})
	router.Use(hlog.AccessHandler(accessLogger))
	router.Use(handlers.CORS(handlers.AllowedMethods([]string{"GET", "POST"}), handlers.AllowedOriginValidator(conf.CheckOrigin)))

	// 添加权限策略头，允许屏幕共享
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Permissions-Policy", "display-capture=*")
			next.ServeHTTP(w, r)
		})
	})

	// 添加一个新的健康检查端点，专门用于代理服务器的健康检查
	router.Methods("GET").Path("/proxy-health").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	router.HandleFunc("/stream", rooms.Upgrade)
	router.Methods("POST").Path("/login").HandlerFunc(users.Authenticate)
	router.Methods("POST").Path("/logout").HandlerFunc(users.Logout)
	router.Methods("GET").Path("/config").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, loggedIn := users.CurrentUser(r)
		_ = json.NewEncoder(w).Encode(&UIConfig{
			AuthMode:                 conf.AuthMode,
			LoggedIn:                 loggedIn,
			User:                     user,
			Version:                  version,
			RoomName:                 rooms.RandRoomName(),
			CloseRoomWhenOwnerLeaves: conf.CloseRoomWhenOwnerLeaves,
		})
	})
	router.Methods("GET").Path("/health").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i, err := rooms.Count()
		status := "up"
		if err != "" {
			status = "down"
			w.WriteHeader(500)
		}
		_ = json.NewEncoder(w).Encode(Health{
			Status:  status,
			Clients: i,
			Reason:  err,
		})
	})
	if conf.Prometheus {
		log.Info().Msg("Prometheus enabled")
		router.Methods("GET").Path("/metrics").Handler(basicAuth(promhttp.Handler(), users))
	}

	ui.Register(router)

	return router
}

func accessLogger(r *http.Request, status, size int, dur time.Duration) {
	log.Debug().
		Str("host", r.Host).
		Int("status", status).
		Int("size", size).
		Str("ip", r.RemoteAddr).
		Str("path", r.URL.Path).
		Str("duration", dur.String()).
		Msg("HTTP")
}

func basicAuth(handler http.Handler, users *auth.Users) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()

		if !ok || !users.Validate(user, pass) {
			w.Header().Set("WWW-Authenticate", `Basic realm="screego"`)
			w.WriteHeader(401)
			_, _ = w.Write([]byte("Unauthorized.\n"))
			return
		}

		handler.ServeHTTP(w, r)
	}
}
