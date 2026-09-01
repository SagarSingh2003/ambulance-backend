package redisclient

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// GeoKey is the single sorted-set key used for GEOADD / GEOSEARCH.
// Every ambulance's location lives here as one member, keyed by ambulance ID.
const GeoKey = "ambulance:locations"

// useTLS controls whether we connect over TLS. Managed providers like
// Upstash require it; a local docker-compose Redis does not (and doesn't
// have certs to negotiate one), so this is driven by an env var rather
// than hardcoded.
func NewClient(addr, password string, dbIndex int, useTLS bool) (*redis.Client, error) {
	opts := &redis.Options{
		Addr:     addr,
		Password: password,
		DB:       dbIndex,
	}
	if useTLS {
		opts.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}
	client := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var pingErr error
	for i := 0; i < 10; i++ {
		if pingErr = client.Ping(ctx).Err(); pingErr == nil {
			return client, nil
		}
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("ping redis: %w", pingErr)
}
