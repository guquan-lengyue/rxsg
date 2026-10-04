package auth

import (
	"github.com/gin-gonic/gin"

	"rxsg/backend/internal/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterPublic 注册无需登录的路由。
func (h *Handler) RegisterPublic(rg *gin.RouterGroup) {
	rg.POST("/auth/login", h.login)
	rg.GET("/announcements/login", h.announce)
}

// RegisterProtected 注册需 RequireAuth 的路由。
func (h *Handler) RegisterProtected(rg *gin.RouterGroup) {
	rg.POST("/auth/logout", h.logout)
	rg.GET("/auth/me", h.me)
}

func (h *Handler) login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "请求体格式错误"))
		return
	}
	rawIP := c.ClientIP()
	resp, err := h.svc.Login(c.Request.Context(), req, IPToInt(rawIP), rawIP)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, resp)
}

func (h *Handler) logout(c *gin.Context) {
	if err := h.svc.Logout(c.Request.Context(), c.GetInt(CtxUID)); err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.Status(204)
}

func (h *Handler) me(c *gin.Context) {
	user, err := h.svc.Me(c.Request.Context(), c.GetInt(CtxUID))
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, user)
}

func (h *Handler) announce(c *gin.Context) {
	content, err := h.svc.Announce(c.Request.Context())
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, gin.H{"content": content})
}