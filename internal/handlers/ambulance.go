package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"ambulance-api/internal/models"
	"ambulance-api/internal/redisclient"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type AmbulanceHandler struct {
	DB    *sql.DB
	Redis *redis.Client
}

func NewAmbulanceHandler(db *sql.DB, rdb *redis.Client) *AmbulanceHandler {
	return &AmbulanceHandler{DB: db, Redis: rdb}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// -----------------------------------------------------------------------
// POST /ambulances  — add a new ambulance
// -----------------------------------------------------------------------
func (h *AmbulanceHandler) AddAmbulance(w http.ResponseWriter, r *http.Request) {
	var req models.AddAmbulanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if req.DriverName == "" || req.VehicleNumber == "" || req.Phone == "" {
		writeErr(w, http.StatusBadRequest, "driver_name, vehicle_number and phone are required")
		return
	}

	id := uuid.NewString()
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	const q = `
		INSERT INTO ambulances (id, driver_name, vehicle_number, phone, status)
		VALUES ($1, $2, $3, $4, 'available')
		RETURNING created_at`

	var createdAt time.Time
	if err := h.DB.QueryRowContext(ctx, q, id, req.DriverName, req.VehicleNumber, req.Phone).Scan(&createdAt); err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to insert ambulance: "+err.Error())
		return
	}

	// if an initial location was given, seed the geo index right away
	if req.Lat != nil && req.Long != nil {
		if err := h.Redis.GeoAdd(ctx, redisclient.GeoKey, &redis.GeoLocation{
			Name:      id,
			Longitude: *req.Long,
			Latitude:  *req.Lat,
		}).Err(); err != nil {
			writeErr(w, http.StatusInternalServerError, "ambulance created but failed to set location: "+err.Error())
			return
		}
	}

	ambulance := models.Ambulance{
		ID:            id,
		DriverName:    req.DriverName,
		VehicleNumber: req.VehicleNumber,
		Phone:         req.Phone,
		Status:        "available",
		CreatedAt:     createdAt,
	}
	writeJSON(w, http.StatusCreated, ambulance)
}

// -----------------------------------------------------------------------
// DELETE /ambulances/{id} — remove an ambulance
// -----------------------------------------------------------------------
func (h *AmbulanceHandler) DeleteAmbulance(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid ambulance id")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	res, err := h.DB.ExecContext(ctx, `DELETE FROM ambulances WHERE id = $1`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to delete ambulance: "+err.Error())
		return
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		writeErr(w, http.StatusNotFound, "ambulance not found")
		return
	}

	// best-effort cleanup of the geo index; ambulance row is already gone
	// either way, so don't fail the request if this errors.
	_ = h.Redis.ZRem(ctx, redisclient.GeoKey, id).Err()

	w.WriteHeader(http.StatusNoContent)
}

// -----------------------------------------------------------------------
// PATCH /ambulances/{id}/location — polled by the ambulance client
// -----------------------------------------------------------------------
func (h *AmbulanceHandler) UpdateLocation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid ambulance id")
		return
	}

	var req models.UpdateLocationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if req.Lat < -90 || req.Lat > 90 || req.Long < -180 || req.Long > 180 {
		writeErr(w, http.StatusBadRequest, "lat/long out of range")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// confirm ambulance actually exists before writing a geo entry for it
	var exists bool
	if err := h.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM ambulances WHERE id = $1)`, id).Scan(&exists); err != nil {
		writeErr(w, http.StatusInternalServerError, "lookup failed: "+err.Error())
		return
	}
	if !exists {
		writeErr(w, http.StatusNotFound, "ambulance not found")
		return
	}

	// GEOADD on an existing member just updates its position — this is
	// exactly what we want for polled location updates.
	if err := h.Redis.GeoAdd(ctx, redisclient.GeoKey, &redis.GeoLocation{
		Name:      id,
		Longitude: req.Long,
		Latitude:  req.Lat,
	}).Err(); err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to update location: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":         id,
		"lat":        req.Lat,
		"long":       req.Long,
		"updated_at": time.Now().UTC(),
	})
}

// -----------------------------------------------------------------------
// GET /ambulances/nearest?lat=..&long=..&radius_km=..&count=..
// -----------------------------------------------------------------------
func (h *AmbulanceHandler) NearestAmbulance(w http.ResponseWriter, r *http.Request) {
	lat, err := parseFloatQuery(r, "lat")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "missing/invalid 'lat' query param")
		return
	}
	long, err := parseFloatQuery(r, "long")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "missing/invalid 'long' query param")
		return
	}

	radiusKM := 10.0
	if v := r.URL.Query().Get("radius_km"); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			radiusKM = parsed
		}
	}
	count := 5
	if v := r.URL.Query().Get("count"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			count = parsed
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	geoResults, err := h.Redis.GeoSearchLocation(ctx, redisclient.GeoKey, &redis.GeoSearchLocationQuery{
		GeoSearchQuery: redis.GeoSearchQuery{
			Longitude:  long,
			Latitude:   lat,
			Radius:     radiusKM,
			RadiusUnit: "km",
			Sort:       "ASC", // nearest first
			Count:      count,
		},
		WithCoord: true,
		WithDist:  true,
	}).Result()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "geo search failed: "+err.Error())
		return
	}
	if len(geoResults) == 0 {
		writeJSON(w, http.StatusOK, []models.NearestAmbulanceResult{})
		return
	}

	results := make([]models.NearestAmbulanceResult, 0, len(geoResults))
	for _, g := range geoResults {
		amb, err := h.fetchAmbulance(ctx, g.Name)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// stale geo entry (ambulance deleted but geo cleanup missed) — skip it
				continue
			}
			writeErr(w, http.StatusInternalServerError, "failed to load ambulance details: "+err.Error())
			return
		}
		// only surface ambulances that are actually available to dispatch
		if amb.Status != "available" {
			continue
		}
		results = append(results, models.NearestAmbulanceResult{
			Ambulance:  amb,
			DistanceKM: g.Dist,
			Lat:        g.Latitude,
			Long:       g.Longitude,
		})
	}

	writeJSON(w, http.StatusOK, results)
}

func (h *AmbulanceHandler) fetchAmbulance(ctx context.Context, id string) (models.Ambulance, error) {
	const q = `SELECT id, driver_name, vehicle_number, phone, status, created_at FROM ambulances WHERE id = $1`
	var a models.Ambulance
	err := h.DB.QueryRowContext(ctx, q, id).Scan(&a.ID, &a.DriverName, &a.VehicleNumber, &a.Phone, &a.Status, &a.CreatedAt)
	return a, err
}

func parseFloatQuery(r *http.Request, key string) (float64, error) {
	v := r.URL.Query().Get(key)
	if v == "" {
		return 0, errors.New("missing " + key)
	}
	return strconv.ParseFloat(v, 64)
}
