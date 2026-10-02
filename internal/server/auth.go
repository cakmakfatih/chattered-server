package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/gin-gonic/gin"
)

type sessionVerifier interface {
	VerifySession(context.Context, string) (*clerk.SessionClaims, error)
}

func authGuard(verifier sessionVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := bearerToken(c.GetHeader("Authorization"))
		if !ok {
			rejectUnauthorized(c)
			return
		}

		claims, err := verifier.VerifySession(c.Request.Context(), token)
		if !validSessionClaims(claims, err) {
			rejectUnauthorized(c)
			return
		}

		c.Set("clerk_user_id", claims.Subject)
		c.Next()
	}
}

func bearerToken(header string) (string, bool) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	return parts[1], true
}

func validSessionClaims(claims *clerk.SessionClaims, err error) bool {
	return err == nil && claims != nil && claims.Subject != "" && claims.SessionID != ""
}

func rejectUnauthorized(c *gin.Context) {
	respondError(c, http.StatusUnauthorized, apiError{
		code:    "unauthorized",
		message: "A valid Clerk session is required",
	})
	c.Abort()
}
