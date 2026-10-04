package hero

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"rxsg/backend/internal/auth"
	"rxsg/backend/internal/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Register 在已挂 RequireAuth 的路由组上注册武将相关端点。
func (h *Handler) Register(rg *gin.RouterGroup) {
	g := rg.Group("/cities/:cid/heroes")
	g.GET("/info", h.info)
	g.POST("/upgrade", h.upgrade)
	g.POST("/expr/start", h.startExpr)
	g.POST("/expr/cancel", h.cancelExpr)
}

func cidOf(c *gin.Context) (int, bool) {
	n, err := strconv.Atoi(c.Param("cid"))
	if err != nil || n <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_cid", "cid 非法"))
		return 0, false
	}
	return n, true
}

type hidReq struct {
	HID int `json:"hid"`
}

type startExprReq struct {
	HID   int `json:"hid"`
	Hours int `json:"hours"`
}

func (h *Handler) info(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	v, err := h.svc.Info(c.Request.Context(), c.GetInt(auth.CtxUID), cid)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

func (h *Handler) upgrade(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	var req hidReq
	if err := c.ShouldBindJSON(&req); err != nil || req.HID <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	v, err := h.svc.Upgrade(c.Request.Context(), c.GetInt(auth.CtxUID), cid, req.HID)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

func (h *Handler) startExpr(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	var req startExprReq
	if err := c.ShouldBindJSON(&req); err != nil || req.HID <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	v, err := h.svc.StartExpr(c.Request.Context(), c.GetInt(auth.CtxUID), cid, req.HID, req.Hours)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

func (h *Handler) cancelExpr(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	var req hidReq
	if err := c.ShouldBindJSON(&req); err != nil || req.HID <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	v, err := h.svc.CancelExpr(c.Request.Context(), c.GetInt(auth.CtxUID), cid, req.HID)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}