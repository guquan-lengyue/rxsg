package tavern

import (
	"context"
	"strconv"

	"github.com/gin-gonic/gin"

	"rxsg/backend/internal/auth"
	"rxsg/backend/internal/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Register 注册客栈端点（写操作 WithUserLock 包裹）。
func (h *Handler) Register(rg *gin.RouterGroup) {
	g := rg.Group("/cities/:cid/hotel")
	g.GET("/info", h.info)
	g.POST("/recruit", h.recruit)
	g.POST("/reset", h.reset)
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
	v, err := h.svc.Info(c.Request.Context(), c.GetInt(auth.CtxUID), cid)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

type recruitReq struct {
	ID int `json:"id"`
}

func (h *Handler) recruit(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	var req recruitReq
	if err := c.ShouldBindJSON(&req); err != nil || req.ID <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := c.GetInt(auth.CtxUID)
	var out *Info
	err := h.svc.WithUserLock(c.Request.Context(), uid, "tavern", func(ctx context.Context) error {
		var err error
		out, err = h.svc.RecruitHero(ctx, uid, cid, req.ID)
		return err
	})
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}

func (h *Handler) reset(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	uid := c.GetInt(auth.CtxUID)
	var out *Info
	err := h.svc.WithUserLock(c.Request.Context(), uid, "tavern", func(ctx context.Context) error {
		var err error
		out, err = h.svc.Reset(ctx, uid, cid)
		return err
	})
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}
