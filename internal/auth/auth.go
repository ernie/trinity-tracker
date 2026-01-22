package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrInvalidToken       = errors.New("invalid or expired token")
)

// Claims represents the JWT claims for an authenticated user
type Claims struct {
	Username               string `json:"username"`
	UserID                 int64  `json:"user_id"`
	IsAdmin                bool   `json:"is_admin"`
	PlayerID               *int64 `json:"player_id,omitempty"`
	PasswordChangeRequired bool   `json:"password_change_required"`
	jwt.RegisteredClaims
}

// Service handles authentication operations
type Service struct {
	jwtSecret     []byte
	tokenDuration time.Duration
}

// NewService creates a new auth service
func NewService(jwtSecret string, tokenDuration time.Duration) *Service {
	if tokenDuration == 0 {
		tokenDuration = 24 * time.Hour
	}
	return &Service{
		jwtSecret:     []byte(jwtSecret),
		tokenDuration: tokenDuration,
	}
}

// HashPassword creates a bcrypt hash of a password
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

// CheckPassword compares a password against a hash
func CheckPassword(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// GenerateToken creates a JWT for an authenticated user
func (s *Service) GenerateToken(userID int64, username string, isAdmin bool, playerID *int64, passwordChangeRequired bool) (string, error) {
	claims := Claims{
		Username:               username,
		UserID:                 userID,
		IsAdmin:                isAdmin,
		PlayerID:               playerID,
		PasswordChangeRequired: passwordChangeRequired,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.tokenDuration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

// ValidateToken validates a JWT and returns the claims
func (s *Service) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		return s.jwtSecret, nil
	})

	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, ErrInvalidToken
	}

	return claims, nil
}
