package building

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

// Register 在已挂 RequireAuth 的路由组上注册建筑相关端点。
func (h *Handler) Register(rg *gin.RouterGroup) {
	g := rg.Group("/cities/:cid/buildings")
	g.GET("/info", h.detail)
	g.POST("/upgrade", h.upgrade)
	g.POST("/stop", h.stop)
}

type gridReq struct {
	X   int `json:"x"`
	Y   int `json:"y"`
	BID int `json:"bid"`
}

func cidOf(c *gin.Context) (int, bool) {
	n, err := strconv.Atoi(c.Param("cid"))
	if err != nil || n <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_cid", "cid 非法"))
		return 0, false
	}
	return n, true
}

func (h *Handler) detail(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	x, _ := strconv.Atoi(c.Query("x"))
	y, _ := strconv.Atoi(c.Query("y"))
	bid, err := strconv.Atoi(c.Query("bid"))
	if err != nil || bid <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_bid", "bid 非法"))
		return
	}
	d, err := h.svc.Detail(c.Request.Context(), c.GetInt(auth.CtxUID), cid, bid, x, y)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, d)
}

func (h *Handler) upgrade(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	var req gridReq
	if err := c.ShouldBindJSON(&req); err != nil || req.BID <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	list, err := h.svc.Upgrade(c.Request.Context(), c.GetInt(auth.CtxUID), cid, req.BID, req.X, req.Y)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, list)
}

func (h *Handler) stop(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	var req gridReq
	if err := c.ShouldBindJSON(&req); err != nil || req.BID <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	list, err := h.svc.Stop(c.Request.Context(), c.GetInt(auth.CtxUID), cid, req.BID, req.X, req.Y)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, list)
}