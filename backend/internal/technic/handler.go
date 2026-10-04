package technic

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

// Register 在已挂 RequireAuth 的路由组上注册科技相关端点。
func (h *Handler) Register(rg *gin.RouterGroup) {
	g := rg.Group("/cities/:cid/technics")
	g.GET("/info", h.info)
	g.POST("/upgrade", h.upgrade)
	g.POST("/stop", h.stop)
}

type technicReq struct {
	TID int `json:"tid"`
}

func cidOf(c *gin.Context) (int, bool) {
	n, err := strconv.Atoi(c.Param("cid"))
	if err != nil || n <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_cid", "cid 非法"))
		return 0, false
	}
	return n, true
}

func (h *Handler) info(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	v, err := h.svc.List(c.Request.Context(), c.GetInt(auth.CtxUID), cid)
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
	var req technicReq
	if err := c.ShouldBindJSON(&req); err != nil || req.TID <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	v, err := h.svc.Upgrade(c.Request.Context(), c.GetInt(auth.CtxUID), cid, req.TID)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

func (h *Handler) stop(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	var req technicReq
	if err := c.ShouldBindJSON(&req); err != nil || req.TID <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	v, err := h.svc.Stop(c.Request.Context(), c.GetInt(auth.CtxUID), cid, req.TID)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}