package main

import (
	"log"
	"net/http"
	"time"

	"ambulance-api/internal/config"
	"ambulance-api/internal/db"
	"ambulance-api/internal/handlers"
	"ambulance-api/internal/redisclient"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	cfg := config.Load()

	pg, err := db.NewPostgres(cfg.PostgresDSN)
	if err != nil {
		log.Fatalf("postgres init failed: %v", err)
	}
	defer pg.Close()

	rdb, err := redisclient.NewClient(cfg.RedisAddr, cfg.RedisPass, cfg.RedisDB, cfg.RedisTLS)
	if err != nil {
		log.Fatalf("redis init failed: %v", err)
	}
	defer rdb.Close()

	h := handlers.NewAmbulanceHandler(pg, rdb)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(10 * time.Second))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	r.Route("/ambulances", func(r chi.Router) {
		r.Post("/", h.AddAmbulance)                     // add ambulance
		r.Delete("/{id}", h.DeleteAmbulance)             // delete ambulance
		r.Patch("/{id}/location", h.UpdateLocation)      // polled location update (redis geo)
		r.Get("/nearest", h.NearestAmbulance)            // find nearest ambulance by lat/long
	})

	log.Printf("ambulance-api listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
