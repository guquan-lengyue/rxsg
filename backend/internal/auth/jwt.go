package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims 是 JWT 载荷：uid + sid。sid 与 sys_sessions.sid 一致，便于失效校验。
type Claims struct {
	UID int   `json:"uid"`
	SID int64 `json:"sid"`
	jwt.RegisteredClaims
}

type JWTManager struct {
	secret []byte
	ttl    time.Duration
}

func NewJWTManager(secret string, ttlSeconds int) *JWTManager {
	if ttlSeconds <= 0 {
		ttlSeconds = 86400
	}
	return &JWTManager{secret: []byte(secret), ttl: time.Duration(ttlSeconds) * time.Second}
}

func (m *JWTManager) TTLSeconds() int { return int(m.ttl.Seconds()) }

func (m *JWTManager) Sign(uid int, sid int64) (string, error) {
	now := time.Now()
	claims := Claims{
		UID: uid,
		SID: sid,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

func (m *JWTManager) Parse(tokenString string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, err
	}
	return claims, nil
}