package goods

import (
	"github.com/gin-gonic/gin"

	"rxsg/backend/internal/auth"
	"rxsg/backend/internal/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Register 在已挂 RequireAuth 的路由组上注册道具端点。
func (h *Handler) Register(rg *gin.RouterGroup) {
	g := rg.Group("/goods")
	g.GET("", h.list)
	g.POST("/use", h.use)
	g.POST("/compose", h.compose)             // goodsCompose（GoodsFunc.php:4836）
	g.POST("/multi-use-card", h.multiUseCard) // multiUseCard（GoodsFunc.php:4647）
}

// composeReq 宝物合成入参（客户端 goods.goodsCompose [gid]）。
type composeReq struct {
	GID int `json:"gid"`
}

func (h *Handler) compose(c *gin.Context) {
	var req composeReq
	if err := c.ShouldBindJSON(&req); err != nil || req.GID == 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	res, err := h.svc.GoodsCompose(c.Request.Context(), c.GetInt(auth.CtxUID), req.GID)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, res)
}

// multiUseCard 名将卡批量使用（客户端 goods.multiUseCard，无入参；新库恒抛"该功能未开放"）。
func (h *Handler) multiUseCard(c *gin.Context) {
	res, err := h.svc.MultiUseCard(c.Request.Context(), c.GetInt(auth.CtxUID))
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, res)
}

func (h *Handler) list(c *gin.Context) {
	rows, err := h.svc.LoadUserGoods(c.Request.Context(), c.GetInt(auth.CtxUID))
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, rows)
}

type useReq struct {
	GID      int `json:"gid"`
	Hid      int `json:"hid"`
	UseCount int `json:"useCount"`
}

func (h *Handler) use(c *gin.Context) {
	var req useReq
	if err := c.ShouldBindJSON(&req); err != nil || req.GID == 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	res, err := h.svc.UseGoods(c.Request.Context(), c.GetInt(auth.CtxUID), req.GID, req.Hid, req.UseCount)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, res)
}
