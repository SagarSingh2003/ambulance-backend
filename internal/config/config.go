package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port        string
	PostgresDSN string
	RedisAddr   string
	RedisPass   string
	RedisDB     int
	RedisTLS    bool
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func Load() Config {
	redisTLS, _ := strconv.ParseBool(getEnv("REDIS_TLS", "false"))

	return Config{
		Port: getEnv("PORT", "8080"),
		PostgresDSN: getEnv("POSTGRES_DSN",
			"host=postgres port=5432 user=postgres password=postgres dbname=ambulance_db sslmode=disable"),
		RedisAddr: getEnv("REDIS_ADDR", "redis:6379"),
		RedisPass: getEnv("REDIS_PASSWORD", ""),
		RedisDB:   0,
		RedisTLS:  redisTLS,
	}
}
