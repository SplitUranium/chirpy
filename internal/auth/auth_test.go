package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMakeJWTValidateJWT(t *testing.T) {
	userID := uuid.New()
	secret := "test"
	duration := time.Hour
	jwt, err := MakeJWT(userID, secret, duration)
	if err != nil {
		t.Error("error making JWT")
	}
	valID, err := ValidateJWT(jwt, secret)
	if err != nil {
		t.Error("error validating JWT")
	}
	if valID != userID {
		t.Errorf("got %v, want %v", valID, userID)
	}
}

func TestExpiredToken(t *testing.T) {
	userID := uuid.New()
	secret := "secret"
	duration := -time.Hour
	jwt, err := MakeJWT(userID, secret, duration)
	if err != nil {
		t.Error("error making JWT")
	}
	_, err = ValidateJWT(jwt, secret)
	if err == nil {
		t.Error("error in duration check")
	}
}

func TestWrongSecret(t *testing.T) {
	userID := uuid.New()
	secret := "secret"
	duration := time.Hour
	jwt, err := MakeJWT(userID, secret, duration)
	if err != nil {
		t.Error("error making JWT")
	}
	_, err = ValidateJWT(jwt, "test")
	if err == nil {
		t.Error("error in secret check")
	}
}
