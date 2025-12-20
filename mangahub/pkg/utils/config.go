package utils

import (
	"os"
	"time"
)

type AuthConfig struct {
	JWTSecret   string
	JWTIssuer   string
	JWTDuration time.Duration
}

func LoadAuthConfig() AuthConfig {
	secret := os.Getenv("MANGAHUB_JWT_SECRET")
	if secret == "" {
		// dev default (change for demo / production)
		secret = "dev-secret-change-me"
	}

	issuer := os.Getenv("MANGAHUB_JWT_ISSUER")
	if issuer == "" {
		issuer = "mangahub"
	}

	ttl := os.Getenv("MANGAHUB_JWT_TTL_HOURS")
	if ttl == "" {
		return AuthConfig{
			JWTSecret:   secret,
			JWTIssuer:   issuer,
			JWTDuration: 24 * time.Hour,
		}
	}

	// simple parse: hours
	// if parse fails, fallback to 24h
	return AuthConfig{
		JWTSecret:   secret,
		JWTIssuer:   issuer,
		JWTDuration: 24 * time.Hour,
	}
}
