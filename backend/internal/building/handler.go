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
	// R11-1 新增（1:1 复刻 legacy BuildingFunc.php）：
	g.POST("/create", h.build)                 // startUpgradeBuilding（建造新建筑）
	g.POST("/destroy", h.destroy)              // startDestroyBuilding（拆除一级）
	g.POST("/destroy-all", h.destroyAll)       // startDestroyBuildingAll（彻底拆除）
	g.POST("/cancel-destroy", h.cancelDestroy) // stopDestroyBuilding（取消拆除）
	g.POST("/exchange", h.exchange)            // startChangeBuilding（资源地转换）
	g.GET("/queue", h.queue)                   // 建筑队列查询（state>0）
	// R11-2 新增（1:1 复刻 legacy BuildingFunc.php getAllValidBuilding）：
	g.GET("/valid", h.valid) // 建造候选列表（?inner=0/1/2）
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

// ── R11-1 新增端点 ─────────────────────────────────────────────────────────

// buildReq 对齐 legacy [cid, inner, x, y, bid]（含内外城标记 inner）。
type buildReq struct {
	Inner int `json:"inner"`
	X     int `json:"x"`
	Y     int `json:"y"`
	BID   int `json:"bid"`
}

// exchangeReq 对齐 legacy [cid, inner, x, y, targetbid, bid]（资源地转换）。
type exchangeReq struct {
	Inner     int `json:"inner"`
	X         int `json:"x"`
	Y         int `json:"y"`
	BID       int `json:"bid"`
	TargetBID int `json:"targetbid"`
}

func (h *Handler) build(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	var req buildReq
	if err := c.ShouldBindJSON(&req); err != nil || req.BID <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	list, err := h.svc.Build(c.Request.Context(), c.GetInt(auth.CtxUID), cid, req.BID, req.Inner, req.X, req.Y)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, list)
}

func (h *Handler) destroy(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	var req buildReq
	if err := c.ShouldBindJSON(&req); err != nil || req.BID <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	list, err := h.svc.Destroy(c.Request.Context(), c.GetInt(auth.CtxUID), cid, req.BID, req.Inner, req.X, req.Y)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, list)
}

func (h *Handler) destroyAll(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	var req buildReq
	if err := c.ShouldBindJSON(&req); err != nil || req.BID <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	list, err := h.svc.DestroyAll(c.Request.Context(), c.GetInt(auth.CtxUID), cid, req.BID, req.Inner, req.X, req.Y)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, list)
}

func (h *Handler) cancelDestroy(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	var req buildReq
	if err := c.ShouldBindJSON(&req); err != nil || req.BID < 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	list, err := h.svc.CancelDestroy(c.Request.Context(), c.GetInt(auth.CtxUID), cid, req.BID, req.Inner, req.X, req.Y)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, list)
}

func (h *Handler) exchange(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	var req exchangeReq
	if err := c.ShouldBindJSON(&req); err != nil || req.BID <= 0 || req.TargetBID <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	list, err := h.svc.Exchange(c.Request.Context(), c.GetInt(auth.CtxUID), cid, req.BID, req.TargetBID, req.Inner, req.X, req.Y)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, list)
}

// valid 建造候选列表：对齐 legacy getAllValidBuilding（要求携带 inner 参数，缺参原文案报错）。
func (h *Handler) valid(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	raw, present := c.GetQuery("inner")
	if !present {
		httpx.WriteError(c, httpx.BadRequest("no_param",
			"no param indicate whether get inner city building list."))
		return
	}
	inner, _ := strconv.Atoi(raw) // 非数字按 0（else）处理，对齐 PHP 宽松比较
	list, err := h.svc.ValidBuildings(c.Request.Context(), c.GetInt(auth.CtxUID), cid, inner)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, list)
}

func (h *Handler) queue(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	items, err := h.svc.Queue(c.Request.Context(), c.GetInt(auth.CtxUID), cid)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, items)
}
