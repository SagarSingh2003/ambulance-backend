# Ambulance API

Simple Go REST API for tracking ambulances and finding the nearest one via
Redis geospatial commands (`GEOADD` / `GEOSEARCH`). Ambulance metadata lives
in Postgres; live location lives in Redis (source of truth for "where is it
right now").

## Stack
- Go 1.22, `chi` router
- Postgres 16 (ambulance details)
- Redis 7 (geo index, key `ambulance:locations`)
- Docker Compose

## Routes

| Method | Path                          | Purpose                                   |
|--------|-------------------------------|--------------------------------------------|
| POST   | `/ambulances`                 | Add a new ambulance                       |
| DELETE | `/ambulances/{id}`            | Delete an ambulance                       |
| PATCH  | `/ambulances/{id}/location`   | Update location (poll this from the ambulance client) |
| GET    | `/ambulances/nearest`         | Find nearest ambulance(s) to a lat/long   |

### POST /ambulances
```json
{
  "driver_name": "Ramesh Kumar",
  "vehicle_number": "WB-06-AB-1234",
  "phone": "9876543210",
  "lat": 22.5726,
  "long": 88.3639
}
```
`lat`/`long` are optional at creation — you can add them later via the
location endpoint. Returns `201` with the created ambulance (includes `id`).

### DELETE /ambulances/{id}
Removes the row from Postgres and its entry from the Redis geo index.
Returns `204`.

### PATCH /ambulances/{id}/location
This is what the ambulance client polls in periodically.
```json
{ "lat": 22.5744, "long": 88.3629 }
```
Internally this is just `GEOADD` on the same member — updating an existing
member's coordinates instead of creating a duplicate.

### GET /ambulances/nearest?lat=22.57&long=88.36&radius_km=10&count=5
Runs `GEOSEARCH` from the given point, sorted ascending by distance, then
joins each hit back to Postgres for details. Only ambulances with
`status = 'available'` are returned. `radius_km` defaults to 10, `count`
defaults to 5.

```json
[
  {
    "ambulance": { "id": "...", "driver_name": "...", "status": "available", ... },
    "distance_km": 0.42,
    "lat": 22.5744,
    "long": 88.3629
  }
]
```

## Running (Docker, WSL)

From WSL, in the project directory:

```bash
docker compose up --build
```

The API will be on `http://localhost:8080`. Postgres on `5432`, Redis on
`6379` (both exposed for debugging — drop the `ports:` mappings in
`docker-compose.yml` for prod).

First run: since this repo doesn't ship a `go.sum`, generate one once
(needs internet access to the Go module proxy):

```bash
go mod tidy
```

Then `docker compose up --build` again so the build stage picks it up.
Alternatively, if you have Go installed locally, just run `go mod tidy`
before the first `docker compose build`.

## Quick test

```bash
# add
curl -X POST localhost:8080/ambulances \
  -H "Content-Type: application/json" \
  -d '{"driver_name":"Ramesh","vehicle_number":"WB-06-AB-1234","phone":"9876543210","lat":22.5726,"long":88.3639}'

# update location (poll loop on the ambulance client would call this)
curl -X PATCH localhost:8080/ambulances/<id>/location \
  -H "Content-Type: application/json" \
  -d '{"lat":22.5744,"long":88.3629}'

# find nearest
curl "localhost:8080/ambulances/nearest?lat=22.5730&long=88.3630&radius_km=5&count=3"

# delete
curl -X DELETE localhost:8080/ambulances/<id>
```

## Notes / things you'll likely want to change
- No auth on any route yet — add a middleware (API key or JWT) before this
  touches anything real.
- `status` defaults to `available` on creation; nothing currently flips it
  to `busy`/`offline` — you'll want a route or trigger for that once a
  dispatch happens.
- Geo cleanup on delete is best-effort (`ZREM` after the Postgres delete
  succeeds); if Redis is down the ambulance row is still gone but the geo
  entry lingers — `nearest` already skips stale entries defensively via
  the `sql.ErrNoRows` check.
- No pagination on stored ambulances / no GET-all endpoint yet since it
  wasn't asked for — easy to add if needed.
