package auth

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func HashPassword(password string) (string, error) {
	return argon2id.CreateHash(password, argon2id.DefaultParams)
}

func CheckPasswordHash(password, hash string) (bool, error) {
	return argon2id.ComparePasswordAndHash(password, hash)
}

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error) {
	currentTime := time.Now().UTC()
	expires := currentTime.Add(expiresIn)
	claims := jwt.RegisteredClaims{
		Issuer:    "chirpy-access",
		IssuedAt:  jwt.NewNumericDate(currentTime),
		ExpiresAt: jwt.NewNumericDate(expires),
		Subject:   userID.String(),
	}
	newToken := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return newToken.SignedString([]byte(tokenSecret))
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
	claims := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims,
		func(token *jwt.Token) (interface{}, error) {
			return []byte(tokenSecret), nil
		},
	)
	if err != nil {
		return uuid.Nil, err
	}

	if token.Valid {
		id, _ := token.Claims.GetSubject()
		return uuid.Parse(id)
	}
	return uuid.Nil, errors.New("invalid token")
}

func GetBearerToken(headers http.Header) (string, error) {
	authorization := headers.Get("Authorization")
	if authorization == "" {
		return "", errors.New("no authorization in header")
	}
	head, authorization, found := strings.Cut(authorization, " ")
	authorization = strings.TrimSpace(authorization)
	if head != "Bearer" || !found || authorization == "" {
		return "", errors.New("no token in header")
	}
	return authorization, nil
}
