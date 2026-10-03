package main

import (
	"context"
	"log"
	"net"

	"github.com/clerk/clerk-sdk-go/v2"
	clerkjwt "github.com/clerk/clerk-sdk-go/v2/jwt"

	"github.com/cakmakfatih/chattered-server/internal/config"
	"github.com/cakmakfatih/chattered-server/internal/database"
	"github.com/cakmakfatih/chattered-server/internal/server"
)

type clerkSessionVerifier struct{}

func (clerkSessionVerifier) VerifySession(ctx context.Context, token string) (*clerk.SessionClaims, error) {
	return clerkjwt.Verify(ctx, &clerkjwt.VerifyParams{Token: token})
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	clerk.SetKey(cfg.ClerkSecretKey)

	db, err := database.OpenPostgres(cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}

	router := server.NewWithDependencies(server.Dependencies{
		Verifier:  clerkSessionVerifier{},
		Users:     database.NewUserRepository(db),
		Validator: server.NewOnboardingValidator(),
	})
	listenAddress := net.JoinHostPort(cfg.Host, cfg.Port)
	log.Printf("API server listening on http://%s", listenAddress)
	if err := router.Run(listenAddress); err != nil {
		log.Fatal(err)
	}
}
