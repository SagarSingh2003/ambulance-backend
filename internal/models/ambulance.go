package models

import "time"

// Ambulance represents an ambulance record stored in Postgres.
type Ambulance struct {
	ID            string    `json:"id"`
	DriverName    string    `json:"driver_name"`
	VehicleNumber string    `json:"vehicle_number"`
	Phone         string    `json:"phone"`
	Status        string    `json:"status"` // available | busy | offline
	CreatedAt     time.Time `json:"created_at"`
}

// AddAmbulanceRequest is the payload for creating a new ambulance.
// Lat/Long are optional at creation time — if provided, the ambulance
// is also registered in the Redis geo index immediately.
type AddAmbulanceRequest struct {
	DriverName    string  `json:"driver_name"`
	VehicleNumber string  `json:"vehicle_number"`
	Phone         string  `json:"phone"`
	Lat           *float64 `json:"lat,omitempty"`
	Long          *float64 `json:"long,omitempty"`
}

// UpdateLocationRequest is the payload the ambulance client polls in with.
type UpdateLocationRequest struct {
	Lat  float64 `json:"lat"`
	Long float64 `json:"long"`
}

// NearestAmbulanceResult is a single entry in the nearest-ambulance response.
type NearestAmbulanceResult struct {
	Ambulance  Ambulance `json:"ambulance"`
	DistanceKM float64   `json:"distance_km"`
	Lat        float64   `json:"lat"`
	Long       float64   `json:"long"`
}
