package httperr_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"clericot/internal/platform/httperr"
)

func TestProblem_Constructors(t *testing.T) {
	notFound := httperr.NewNotFound("User 123 not found")
	assert.Equal(t, http.StatusNotFound, notFound.GetStatus())
	assert.Equal(t, "Not Found", notFound.Title)
	assert.Equal(t, "User 123 not found", notFound.Detail)

	conflict := httperr.NewConflict("Email already exists")
	assert.Equal(t, http.StatusConflict, conflict.GetStatus())

	validation := httperr.NewValidation("Invalid request parameters", httperr.ErrorDetail{
		Location: "body.email",
		Message:  "must be a valid email",
	})
	assert.Equal(t, http.StatusUnprocessableEntity, validation.GetStatus())
	assert.Len(t, validation.Errors, 1)

	unauthorized := httperr.NewUnauthorized("Missing bearer token")
	assert.Equal(t, http.StatusUnauthorized, unauthorized.GetStatus())

	forbidden := httperr.NewForbidden("Insufficient permissions")
	assert.Equal(t, http.StatusForbidden, forbidden.GetStatus())
}

func TestProblem_Transform(t *testing.T) {
	assert.Nil(t, httperr.Transform(nil))

	origProblem := httperr.NewNotFound("Resource not found")
	assert.Equal(t, origProblem, httperr.Transform(origProblem))

	genericErr := errors.New("database connection failed")
	transformed := httperr.Transform(genericErr)
	assert.Equal(t, http.StatusInternalServerError, transformed.GetStatus())
}
