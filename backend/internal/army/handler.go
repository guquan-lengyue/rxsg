package army

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

// Register 在已挂 RequireAuth 的路由组上注册军事相关端点。
func (h *Handler) Register(rg *gin.RouterGroup) {
	g := rg.Group("/cities/:cid/army")
	g.GET("/info", h.info)
	g.POST("/draft", h.draft)
	g.POST("/draft/stop", h.stopDraft)
	g.POST("/dissolve", h.dissolve)
	g.GET("/fields", h.fields)
	g.GET("/marches", h.marches)
	g.POST("/dispatch", h.dispatch)
	g.POST("/recall", h.recall)
}

func cidOf(c *gin.Context) (int, bool) {
	n, err := strconv.Atoi(c.Param("cid"))
	if err != nil || n <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_cid", "cid 非法"))
		return 0, false
	}
	return n, true
}

// xyQuery 读取可选的 x/y 查询参数。
func xyQuery(c *gin.Context) (x, y int, has bool) {
	xs, ys := c.Query("x"), c.Query("y")
	if xs == "" || ys == "" {
		return 0, 0, false
	}
	x, err1 := strconv.Atoi(xs)
	y, err2 := strconv.Atoi(ys)
	if err1 != nil || err2 != nil || x < 0 || y < 0 {
		return 0, 0, false
	}
	return x, y, true
}

type xyReq struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type draftReq struct {
	X     *int `json:"x"`
	Y     *int `json:"y"`
	SID   int  `json:"sid"`
	Count int  `json:"count"`
}

type stopDraftReq struct {
	X   *int `json:"x"`
	Y   *int `json:"y"`
	QID int  `json:"qid"`
}

type dissolveReq struct {
	SID   int `json:"sid"`
	Count int `json:"count"`
}

type dispatchReq struct {
	HeroID     int            `json:"hero_id"`
	TargetType int            `json:"target_type"`
	TargetID   int            `json:"target_id"`
	Task       int            `json:"task"`
	Soldiers   map[int]int64  `json:"soldiers"`
}

type recallReq struct {
	TroopID int `json:"troop_id"`
}

func (h *Handler) info(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	x, y, has := xyQuery(c)
	v, err := h.svc.Info(c.Request.Context(), c.GetInt(auth.CtxUID), cid, x, y, has)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

func (h *Handler) draft(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	var req draftReq
	if err := c.ShouldBindJSON(&req); err != nil || req.SID <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	x, y, has := 0, 0, false
	if req.X != nil && req.Y != nil {
		x, y, has = *req.X, *req.Y, true
	}
	v, err := h.svc.StartDraft(c.Request.Context(), c.GetInt(auth.CtxUID), cid, x, y, has, req.SID, req.Count)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

func (h *Handler) stopDraft(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	var req stopDraftReq
	if err := c.ShouldBindJSON(&req); err != nil || req.QID <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	x, y, has := 0, 0, false
	if req.X != nil && req.Y != nil {
		x, y, has = *req.X, *req.Y, true
	}
	v, err := h.svc.StopDraft(c.Request.Context(), c.GetInt(auth.CtxUID), cid, x, y, has, req.QID)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

func (h *Handler) dissolve(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	var req dissolveReq
	if err := c.ShouldBindJSON(&req); err != nil || req.SID <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	v, err := h.svc.Dissolve(c.Request.Context(), c.GetInt(auth.CtxUID), cid, req.SID, req.Count)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

func (h *Handler) fields(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	v, err := h.svc.Fields(c.Request.Context(), c.GetInt(auth.CtxUID), cid)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

func (h *Handler) marches(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	v, err := h.svc.Marches(c.Request.Context(), c.GetInt(auth.CtxUID), cid)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

func (h *Handler) dispatch(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	var req dispatchReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	v, err := h.svc.Dispatch(c.Request.Context(), c.GetInt(auth.CtxUID), cid,
		req.HeroID, req.TargetType, req.TargetID, req.Task, req.Soldiers)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

func (h *Handler) recall(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	var req recallReq
	if err := c.ShouldBindJSON(&req); err != nil || req.TroopID <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	v, err := h.svc.Recall(c.Request.Context(), c.GetInt(auth.CtxUID), cid, req.TroopID)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}