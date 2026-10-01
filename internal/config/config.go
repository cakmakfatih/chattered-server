package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	ClerkSecretKey string
	Port           string
}

func Load() (Config, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("load .env: %w", err)
	}

	secretKey := strings.TrimSpace(os.Getenv("CLERK_SECRET_KEY"))
	if secretKey == "" {
		return Config{}, errors.New("CLERK_SECRET_KEY is required")
	}

	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8080"
	}

	return Config{ClerkSecretKey: secretKey, Port: port}, nil
}
