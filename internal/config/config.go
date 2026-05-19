package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Addr          string
	DatabaseURL   string
	SessionTTL    time.Duration
	CookieName    string
	CookieSecure  bool
	RunMigrations bool
	SeedData      bool
}

func Load() Config {
	defaultDB := "postgres://rehab:rehab@localhost:5432/rehab_app?sslmode=disable"
	return Config{
		Addr:          getEnv("APP_ADDR", ":8080"),
		DatabaseURL:   getEnv("DATABASE_URL", defaultDB),
		SessionTTL:    getDuration("SESSION_TTL", 24*time.Hour*7),
		CookieName:    getEnv("COOKIE_NAME", "rehab_session"),
		CookieSecure:  getEnvBool("COOKIE_SECURE", false),
		RunMigrations: getEnvBool("RUN_MIGRATIONS", true),
		SeedData:      getEnvBool("SEED_DATA", false),
	}
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func getEnvBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
