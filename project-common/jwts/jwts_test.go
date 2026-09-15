package jwts

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

func TestParseToken(t *testing.T) {
	tokens := CreateToken("1014", time.Hour, "access-secret", 24*time.Hour, "refresh-secret", "127.0.0.1")
	got, err := ParseToken(tokens.AccessToken, "access-secret", "127.0.0.1")
	if err != nil {
		t.Fatalf("ParseToken() error = %v", err)
	}
	if got != "1014" {
		t.Fatalf("ParseToken() = %q, want %q", got, "1014")
	}
	if _, err = ParseToken(tokens.AccessToken, "access-secret", "192.0.2.1"); err == nil {
		t.Fatal("ParseToken() accepted a token with the wrong IP")
	}
}

func TestCreateTokenUsesRefreshExpiration(t *testing.T) {
	start := time.Now().Unix()
	tokens := CreateToken("1014", time.Hour, "access-secret", 24*time.Hour, "refresh-secret", "127.0.0.1")

	parsed, err := jwt.Parse(tokens.RefreshToken, func(token *jwt.Token) (interface{}, error) {
		return []byte("refresh-secret"), nil
	})
	if err != nil || !parsed.Valid {
		t.Fatalf("refresh token is invalid: token=%v err=%v", parsed, err)
	}
	claims := parsed.Claims.(jwt.MapClaims)
	exp, ok := claims["exp"].(float64)
	if !ok {
		t.Fatalf("refresh exp has unexpected type %T", claims["exp"])
	}
	if int64(exp) < start+23*60*60 {
		t.Fatalf("refresh token expires too early: %v", exp)
	}
}

func TestParseTokenRejectsMalformedClaims(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"token": 123,
		"exp":   time.Now().Add(time.Hour).Unix(),
		"ip":    "127.0.0.1",
	})
	tokenString, err := token.SignedString([]byte("access-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ParseToken(tokenString, "access-secret", "127.0.0.1"); err == nil {
		t.Fatal("ParseToken() accepted malformed token claims")
	}
}
