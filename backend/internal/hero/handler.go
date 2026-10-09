package hero

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

// Register 在已挂 RequireAuth 的路由组上注册武将相关端点。
// 写操作统一 WithUserLock 包裹（对齐 legacy 函数体内 lockUser/unlockUser）。
func (h *Handler) Register(rg *gin.RouterGroup) {
	g := rg.Group("/cities/:cid/heroes")
	g.GET("/info", h.info)
	g.POST("/upgrade", h.upgrade)
	g.POST("/point/add", h.addPoint)
	g.POST("/point/clear", h.clearPoint)
	g.POST("/expr/start", h.startExpr)
	g.POST("/expr/cancel", h.cancelExpr)
	g.POST("/expr/faster", h.fasterExpr)
	g.POST("/office", h.office)
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

// locked 在用户锁内执行 fn 并输出结果。
func (h *Handler) locked(c *gin.Context, key string, fn func(ctx context.Context) (*Info, error)) {
	uid := c.GetInt(auth.CtxUID)
	var out *Info
	err := h.svc.WithUserLock(c.Request.Context(), uid, key, func(ctx context.Context) error {
		var err error
		out, err = fn(ctx)
		return err
	})
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
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
	h.locked(c, "hero_upgrade", func(ctx context.Context) (*Info, error) {
		return h.svc.Upgrade(ctx, c.GetInt(auth.CtxUID), cid, req.HID)
	})
}

type addPointReq struct {
	HID     int `json:"hid"`
	Affairs int `json:"affairs"`
	Bravery int `json:"bravery"`
	Wisdom  int `json:"wisdom"`
}

func (h *Handler) addPoint(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	var req addPointReq
	if err := c.ShouldBindJSON(&req); err != nil || req.HID <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	h.locked(c, "hero_point", func(ctx context.Context) (*Info, error) {
		return h.svc.AddPoint(ctx, c.GetInt(auth.CtxUID), cid, req.HID, req.Affairs, req.Bravery, req.Wisdom)
	})
}

func (h *Handler) clearPoint(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	var req hidReq
	if err := c.ShouldBindJSON(&req); err != nil || req.HID <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	h.locked(c, "hero_point", func(ctx context.Context) (*Info, error) {
		return h.svc.ClearPoint(ctx, c.GetInt(auth.CtxUID), cid, req.HID)
	})
}

type startExprReq struct {
	HID        int `json:"hid"`
	ExprType   int `json:"exprType"`
	Hours      int `json:"hours"`
	Carrymoney int `json:"carrymoney"`
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
	h.locked(c, "hero_expr", func(ctx context.Context) (*Info, error) {
		return h.svc.StartExpr(ctx, c.GetInt(auth.CtxUID), cid, req.HID, req.ExprType, req.Hours, req.Carrymoney)
	})
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
	h.locked(c, "hero_expr", func(ctx context.Context) (*Info, error) {
		return h.svc.CancelExpr(ctx, c.GetInt(auth.CtxUID), cid, req.HID)
	})
}

func (h *Handler) fasterExpr(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	var req hidReq
	if err := c.ShouldBindJSON(&req); err != nil || req.HID <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	h.locked(c, "hero_expr", func(ctx context.Context) (*Info, error) {
		return h.svc.FasterExpr(ctx, c.GetInt(auth.CtxUID), cid, req.HID)
	})
}

// officeReq 对齐 legacy setCityChief 的 param：每项 [hid, cheiftype]（cheiftype∈{0,1,7,8}）。
type officeReq struct {
	Sets [][2]int `json:"sets"`
}

func (h *Handler) office(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	var req officeReq
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Sets) == 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	h.locked(c, "hero_office", func(ctx context.Context) (*Info, error) {
		return h.svc.SetChief(ctx, c.GetInt(auth.CtxUID), cid, req.Sets)
	})
}
