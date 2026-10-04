package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	"rxsg/backend/internal/auth"
	"rxsg/backend/internal/httpx"
)

// RequireAuth 校验 Bearer JWT，并确认 (uid,sid) 与内存会话一致
// （对齐 server/game/global.php:4 checkUserAuth）。
func RequireAuth(jwt *auth.JWTManager, session *auth.SessionStore) gin.HandlerFunc {
	const prefix = "Bearer "
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, prefix) {
			httpx.WriteError(c, httpx.Unauthorized("invalid_user_auth", "缺少登录令牌"))
			c.Abort()
			return
		}
		claims, err := jwt.Parse(strings.TrimSpace(header[len(prefix):]))
		if err != nil {
			httpx.WriteError(c, httpx.Unauthorized("invalid_user_auth", "登录令牌无效或已过期"))
			c.Abort()
			return
		}
		if !session.Valid(claims.UID, claims.SID) {
			httpx.WriteError(c, httpx.Unauthorized("invalid_user_auth", "会话已失效，请重新登录"))
			c.Abort()
			return
		}
		c.Set(auth.CtxUID, claims.UID)
		c.Set(auth.CtxSID, claims.SID)
		c.Next()
	}
}