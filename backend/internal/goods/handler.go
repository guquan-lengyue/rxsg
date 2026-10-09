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
