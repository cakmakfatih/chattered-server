package server

import (
	"errors"
	"net/http"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
)

func userCreationError(err error) (int, apiError) {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == pgerrcode.UniqueViolation {
		if status, apiErr, ok := uniqueConstraintError(postgresError.ConstraintName); ok {
			return status, apiErr
		}
	}
	return http.StatusInternalServerError, apiError{
		code:    "internal_error",
		message: "Could not create the user profile",
	}
}

func uniqueConstraintError(constraint string) (int, apiError, bool) {
	switch constraint {
	case "ux_users_username":
		return http.StatusConflict, *usernameAlreadyInUse(), true
	case "ux_users_clerk_user_id":
		return http.StatusConflict, *profileAlreadyExists(), true
	default:
		return 0, apiError{}, false
	}
}
