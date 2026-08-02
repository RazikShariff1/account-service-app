package jwt

import (
	"errors"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const tokenTTL = 24 * time.Hour

var ErrInvalidToken = errors.New("invalid token")

var ErrExpiredToken = errors.New("token is expired")

// secretKey is read from JWT_SECRET so it can be overridden outside of tests;
// falls back to a fixed dev value since there's no real secret store yet.
func secretKey() []byte {
	if s := os.Getenv("JWT_SECRET"); s != "" {
		return []byte(s)
	}

	return []byte("students_professionals")
}

type Claims struct {
	AId int `json:"a_id"`
	HId int `json:"h_id"`
	MId int `json:"m_id"`
	jwt.RegisteredClaims
}

func Generate(aId, hId, mId int) (string, error) {
	now := time.Now()

	claims := Claims{
		AId: aId,
		HId: hId,
		MId: mId,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(tokenTTL)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	return token.SignedString(secretKey())
}

func Parse(tokenString string) (*Claims, error) {
	claims := &Claims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}

		return secretKey(), nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}

		return nil, ErrInvalidToken
	}

	if !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}
