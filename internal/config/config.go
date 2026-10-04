package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	ClerkSecretKey        string
	DatabaseURL           string
	Host                  string
	Port                  string
	TigrisAccessKeyID     string
	TigrisSecretAccessKey string
	TigrisEndpoint        string
	TigrisRegion          string
	TigrisBucket          string
}

func Load() (Config, error) {
	if err := godotenv.Load(".env.dev"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("load .env.dev: %w", err)
	}

	secretKey := strings.TrimSpace(os.Getenv("CLERK_SECRET_KEY"))
	if secretKey == "" {
		return Config{}, errors.New("CLERK_SECRET_KEY is required")
	}

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}

	host := strings.TrimSpace(os.Getenv("HOST"))
	if host == "" {
		host = "0.0.0.0"
	}

	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8080"
	}

	return Config{
		ClerkSecretKey:        secretKey,
		DatabaseURL:           databaseURL,
		Host:                  host,
		Port:                  port,
		TigrisAccessKeyID:     strings.TrimSpace(os.Getenv("AWS_ACCESS_KEY_ID")),
		TigrisSecretAccessKey: strings.TrimSpace(os.Getenv("AWS_SECRET_ACCESS_KEY")),
		TigrisEndpoint:        firstConfigured(os.Getenv("AWS_ENDPOINT_URL_S3"), strings.TrimSpace(os.Getenv("AWS_ENDPOINT_URL"))),
		TigrisRegion:          firstConfigured(os.Getenv("AWS_REGION"), "auto"),
		TigrisBucket:          strings.TrimSpace(os.Getenv("BUCKET_NAME")),
	}, nil
}

func firstConfigured(value, fallback string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return fallback
}
