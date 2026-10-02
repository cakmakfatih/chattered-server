package main

import (
	"log"

	"github.com/clerk/clerk-sdk-go/v2"

	"github.com/cakmakfatih/chattered-server/internal/config"
	"github.com/cakmakfatih/chattered-server/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	clerk.SetKey(cfg.ClerkSecretKey)

	router := server.New()
	log.Printf("API server listening on :%s", cfg.Port)
	if err := router.Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
