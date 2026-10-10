package world

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

// Register 注册 M11 世界地图端点。
// 入参来源按 legacy WorldFunc.php 逐个 array_shift($param) 语义设计：
//   - doGetWorldInfo($uid,$cid)：cid 为入城上下文 → 路径 /cities/:cid/world/heroes。
//   - 其余函数的客户端入参（array_shift 顺序）一律经 body/query 承接，不臆造路径参数。
func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.GET("/cities/:cid/world/heroes", h.doGetWorldInfo)

	g := rg.Group("/world")
	g.POST("/blocks", h.getBlockData)
	g.POST("/city-info", h.getWorldCityInfo)
	g.POST("/field-info", h.getWorldFieldInfo)
	g.POST("/start-war", h.startWar)
	g.POST("/build-city", h.createCityFromLand)
	g.POST("/favourites", h.addFavourites)
	g.GET("/favourites", h.getFavouritesList)
	g.POST("/favourites/delete", h.deleteFavourites)
	g.POST("/favourites/comments", h.setFavouritesComments)
	g.POST("/govern-info", h.getGovernInfo)
	g.POST("/govern", h.governOthers)
	g.GET("/map-city", h.getMapCity)
	g.POST("/check-invade", h.checkCanInvade)
	g.POST("/mark", h.markCity)
	g.GET("/action-field", h.getActionField)
	g.GET("/user-fields", h.getUserFields)
}

func cidOf(c *gin.Context) (int, bool) {
	n, err := strconv.Atoi(c.Param("cid"))
	if err != nil || n <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_cid", "cid 非法"))
		return 0, false
	}
	return n, true
}

func uidOf(c *gin.Context) int { return c.GetInt(auth.CtxUID) }

// locked 以 "world" 为键包裹写操作（对齐 legacy lockUser 区间）。
func (h *Handler) locked(c *gin.Context, uid int, fn func() error) {
	if err := h.svc.WithUserLock(c.Request.Context(), uid, "world", func(context.Context) error { return fn() }); err != nil {
		httpx.WriteError(c, err)
	}
}

type idReq struct {
	ID int `json:"id"`
}

type cidReq struct {
	Cid int `json:"cid"`
}

// ── doGetWorldInfo ─────────────────────────────────────────────────────────

func (h *Handler) doGetWorldInfo(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	out, err := h.svc.DoGetWorldInfo(c.Request.Context(), uidOf(c), cid)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}

// ── getBlockData ───────────────────────────────────────────────────────────

type blocksReq struct {
	Blocks []int `json:"blocks"`
}

func (h *Handler) getBlockData(c *gin.Context) {
	var req blocksReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	out, err := h.svc.GetBlockData(c.Request.Context(), uidOf(c), req.Blocks)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}

// ── getWorldCityInfo ───────────────────────────────────────────────────────

type cityInfoReq struct {
	Cities []int `json:"cities"`
}

func (h *Handler) getWorldCityInfo(c *gin.Context) {
	var req cityInfoReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	out, err := h.svc.GetWorldCityInfo(c.Request.Context(), uidOf(c), req.Cities)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}

// ── getWorldFieldInfo ──────────────────────────────────────────────────────

type widReq struct {
	Wid int `json:"wid"`
}

func (h *Handler) getWorldFieldInfo(c *gin.Context) {
	var req widReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	out, err := h.svc.GetWorldFieldInfo(c.Request.Context(), uidOf(c), req.Wid)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}

// ── startWar ───────────────────────────────────────────────────────────────

type startWarReq struct {
	TargetUID int `json:"targetuid"`
	TargetCid int `json:"targetcid"`
}

func (h *Handler) startWar(c *gin.Context) {
	var req startWarReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	var out []map[string]any
	h.locked(c, uid, func() error {
		var err error
		out, err = h.svc.StartWar(c.Request.Context(), uid, req.TargetUID, req.TargetCid)
		return err
	})
	if out != nil {
		c.JSON(200, out)
	}
}

// ── createCityFromLand ─────────────────────────────────────────────────────

type buildCityReq struct {
	TargetWid int `json:"targetwid"`
}

func (h *Handler) createCityFromLand(c *gin.Context) {
	var req buildCityReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	h.locked(c, uid, func() error {
		out, err := h.svc.CreateCityFromLand(c.Request.Context(), uid, req.TargetWid)
		if err != nil {
			return err
		}
		c.JSON(200, out)
		return nil
	})
}

// ── 收藏四件套 ─────────────────────────────────────────────────────────────

type favAddReq struct {
	TargetCid int `json:"targetcid"`
}

func (h *Handler) addFavourites(c *gin.Context) {
	var req favAddReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	h.locked(c, uid, func() error {
		return h.svc.AddFavourites(c.Request.Context(), uid, req.TargetCid)
	})
}

func (h *Handler) getFavouritesList(c *gin.Context) {
	out, err := h.svc.GetFavouritesList(c.Request.Context(), uidOf(c))
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}

func (h *Handler) deleteFavourites(c *gin.Context) {
	var req idReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	h.locked(c, uid, func() error {
		out, err := h.svc.DeleteFavourites(c.Request.Context(), uid, req.ID)
		if err != nil {
			return err
		}
		c.JSON(200, out)
		return nil
	})
}

type favCommentsReq struct {
	ID       int    `json:"id"`
	Comments string `json:"comments"`
}

func (h *Handler) setFavouritesComments(c *gin.Context) {
	var req favCommentsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	h.locked(c, uid, func() error {
		return h.svc.SetFavouritesComments(c.Request.Context(), uid, req.ID, req.Comments)
	})
}

// ── getGovernInfo / governOthers ───────────────────────────────────────────

type governInfoReq struct {
	Cid int `json:"cid"`
}

func (h *Handler) getGovernInfo(c *gin.Context) {
	var req governInfoReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	out, err := h.svc.GetGovernInfo(c.Request.Context(), uidOf(c), req.Cid)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}

type governReq struct {
	Type     int    `json:"type"`
	Tcid     int    `json:"tcid"`
	Tuid     int    `json:"tuid"`
	Cid      int    `json:"cid"`
	Cityname string `json:"cityname"`
}

func (h *Handler) governOthers(c *gin.Context) {
	var req governReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	h.locked(c, uid, func() error {
		return h.svc.GovernOthers(c.Request.Context(), uid, req.Type, req.Tcid, req.Tuid, req.Cid, req.Cityname)
	})
}

// ── getMapCity ─────────────────────────────────────────────────────────────

func (h *Handler) getMapCity(c *gin.Context) {
	out, err := h.svc.GetMapCity(c.Request.Context(), uidOf(c))
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}

// ── checkCanInvade ─────────────────────────────────────────────────────────

func (h *Handler) checkCanInvade(c *gin.Context) {
	var req governInfoReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	out, err := h.svc.CheckCanInvade(c.Request.Context(), uidOf(c), req.Cid)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}

// ── markCity / clearMark ───────────────────────────────────────────────────

func (h *Handler) markCity(c *gin.Context) {
	var req cidReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	h.locked(c, uid, func() error {
		return h.svc.MarkCity(c.Request.Context(), uid, req.Cid)
	})
}

// ── getActionField ─────────────────────────────────────────────────────────

func (h *Handler) getActionField(c *gin.Context) {
	typ, _ := strconv.Atoi(c.DefaultQuery("type", "0"))
	out, err := h.svc.GetActionField(c.Request.Context(), uidOf(c), typ)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}

// ── getUserFields ──────────────────────────────────────────────────────────

func (h *Handler) getUserFields(c *gin.Context) {
	out, err := h.svc.GetUserFields(c.Request.Context(), uidOf(c))
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}
