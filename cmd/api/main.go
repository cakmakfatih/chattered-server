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
	"github.com/cakmakfatih/chattered-server/internal/storage"
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
	profilePhotos, err := storage.NewTigrisProfilePhotoStore(
		context.Background(),
		cfg.TigrisEndpoint,
		cfg.TigrisBucket,
		cfg.TigrisRegion,
		cfg.TigrisAccessKeyID,
		cfg.TigrisSecretAccessKey,
	)
	if err != nil {
		log.Fatal(err)
	}

	router := server.NewWithDependencies(server.Dependencies{
		Verifier:      clerkSessionVerifier{},
		Users:         database.NewUserRepository(db),
		ProfilePhotos: profilePhotos,
		Validator:     server.NewOnboardingValidator(),
	})
	listenAddress := net.JoinHostPort(cfg.Host, cfg.Port)
	log.Printf("API server listening on http://%s", listenAddress)
	if err := router.Run(listenAddress); err != nil {
		log.Fatal(err)
	}
}
