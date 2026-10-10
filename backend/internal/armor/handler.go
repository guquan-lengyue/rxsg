package armor

import (
	"context"
	"strconv"

	"github.com/gin-gonic/gin"

	"rxsg/backend/internal/auth"
	"rxsg/backend/internal/httpx"
)

// handler.go 注册 M5 装备 REST 端点（写操作 WithUserLock 包裹，key="armor"）。
type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Register(rg *gin.RouterGroup) {
	g := rg.Group("/armors")
	g.GET("", h.bag)
	g.POST("/equip", h.equip)
	g.POST("/offload", h.offload)
	g.POST("/repair", h.repair)
	g.POST("/repair-all", h.repairAll)
	g.POST("/renovate", h.renovate)
	g.POST("/renovate-all", h.renovateAll)
	g.POST("/sell", h.sell)
	g.POST("/chaijie", h.chaijie)
	g.POST("/strong", h.strong)
	g.POST("/combine", h.combine)
	g.POST("/holes", h.initHoles)
	g.POST("/open-hole", h.openHole)
	g.POST("/embed", h.embed)

	// R11-3 新增（1:1 复刻 legacy BarnFunc.php 马厩/坐骑批次）：
	g.POST("/barn/goods", h.barnGoods)                 // loadBarnGoods（BarnFunc.php:5）
	g.POST("/barn/zuoji-armors", h.barnZuojiArmors)    // loadZuojiArmor（BarnFunc.php:19）
	g.POST("/barn/unlade", h.barnUnlade)               // doUnlade（BarnFunc.php:30）
	g.POST("/barn/embed-pearls", h.barnEmbedPearls)    // loadEmbedPearlByArmor（EquipmentFunc.php:859）
	g.GET("/barn/unactive-horse", h.barnUnactiveHorse) // loadUnActiveHorseArmor（EquipmentFunc.php:12）
	g.POST("/upgrade", h.upgradeArmor)                 // doUpgradeArmor（ArmorFunc.php:1190）

	rg.GET("/heroes/:hid/armors", h.heroArmors)
}

func uidOf(c *gin.Context) int { return c.GetInt(auth.CtxUID) }

func (h *Handler) bag(c *gin.Context) {
	out, err := h.svc.LoadUserArmor(c.Request.Context(), uidOf(c))
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}

func (h *Handler) heroArmors(c *gin.Context) {
	hid, err := strconv.Atoi(c.Param("hid"))
	if err != nil || hid <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_hid", "hid 非法"))
		return
	}
	out, err := h.svc.GetHeroArmor(c.Request.Context(), hid)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}

type equipReq struct {
	HID   int `json:"hid"`
	SID   int `json:"sid"`
	Spart int `json:"spart"`
}

func (h *Handler) equip(c *gin.Context) {
	var req equipReq
	if err := c.ShouldBindJSON(&req); err != nil || req.HID <= 0 || req.SID <= 0 || req.Spart <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	var out *EquipResult
	err := h.svc.WithUserLock(c.Request.Context(), uid, "armor", func(ctx context.Context) error {
		var err error
		out, err = h.svc.EquipArmor(ctx, uid, req.HID, req.SID, req.Spart)
		return err
	})
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}

type offloadReq struct {
	HID   int `json:"hid"`
	Spart int `json:"spart"`
}

func (h *Handler) offload(c *gin.Context) {
	var req offloadReq
	if err := c.ShouldBindJSON(&req); err != nil || req.HID <= 0 || req.Spart <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	var out *EquipResult
	err := h.svc.WithUserLock(c.Request.Context(), uid, "armor", func(ctx context.Context) error {
		var err error
		out, err = h.svc.OffloadArmor(ctx, uid, req.HID, req.Spart)
		return err
	})
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}

type repairReq struct {
	CID int `json:"cid"`
	SID int `json:"sid"`
}

func (h *Handler) repair(c *gin.Context) {
	var req repairReq
	if err := c.ShouldBindJSON(&req); err != nil || req.CID <= 0 || req.SID <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	var sid, hpmax int
	var gold int64
	err := h.svc.WithUserLock(c.Request.Context(), uid, "armor", func(ctx context.Context) error {
		var err error
		sid, hpmax, gold, err = h.svc.RepairArmor(ctx, uid, req.CID, req.SID)
		return err
	})
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, gin.H{"sid": sid, "hp_max": hpmax, "gold": gold})
}

type sidsReq struct {
	CID  int   `json:"cid"`
	SIDs []int `json:"sids"`
}

func (h *Handler) repairAll(c *gin.Context) {
	var req sidsReq
	if err := c.ShouldBindJSON(&req); err != nil || req.CID <= 0 || len(req.SIDs) == 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	var gold int64
	err := h.svc.WithUserLock(c.Request.Context(), uid, "armor", func(ctx context.Context) error {
		var err error
		gold, err = h.svc.RepairAllArmor(ctx, uid, req.CID, req.SIDs)
		return err
	})
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, gin.H{"gold": gold})
}

type sidReq struct {
	SID int `json:"sid"`
}

func (h *Handler) renovate(c *gin.Context) {
	var req sidReq
	if err := c.ShouldBindJSON(&req); err != nil || req.SID <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	var sid int
	err := h.svc.WithUserLock(c.Request.Context(), uid, "armor", func(ctx context.Context) error {
		var err error
		sid, err = h.svc.RenovateArmor(ctx, uid, req.SID)
		return err
	})
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, gin.H{"sid": sid})
}

func (h *Handler) renovateAll(c *gin.Context) {
	var req struct {
		SIDs []int `json:"sids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.SIDs) == 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	err := h.svc.WithUserLock(c.Request.Context(), uid, "armor", func(ctx context.Context) error {
		return h.svc.RenovateAllArmor(ctx, uid, req.SIDs)
	})
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (h *Handler) sell(c *gin.Context) {
	var req repairReq
	if err := c.ShouldBindJSON(&req); err != nil || req.CID <= 0 || req.SID <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	var sid, cid int
	var gold int64
	err := h.svc.WithUserLock(c.Request.Context(), uid, "armor", func(ctx context.Context) error {
		var err error
		sid, cid, gold, err = h.svc.SellArmor(ctx, uid, req.CID, req.SID)
		return err
	})
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, gin.H{"sid": sid, "cid": cid, "gold": gold})
}

func (h *Handler) chaijie(c *gin.Context) {
	var req sidReq
	if err := c.ShouldBindJSON(&req); err != nil || req.SID <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	var msg string
	err := h.svc.WithUserLock(c.Request.Context(), uid, "armor", func(ctx context.Context) error {
		var err error
		msg, err = h.svc.Chaijie(ctx, uid, req.SID)
		return err
	})
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, gin.H{"msg": msg})
}

type strongReq struct {
	CID        int `json:"cid"`
	GID1       int `json:"gid1"`
	Good1Count int `json:"good1_count"`
	GID2       int `json:"gid2"`
	Good2Count int `json:"good2_count"`
	SID        int `json:"sid"`
	IsZuoji    int `json:"is_zuoji"`
}

func (h *Handler) strong(c *gin.Context) {
	var req strongReq
	if err := c.ShouldBindJSON(&req); err != nil || req.CID <= 0 || req.SID <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	var out *StrongResult
	err := h.svc.WithUserLock(c.Request.Context(), uid, "armor", func(ctx context.Context) error {
		var err error
		out, err = h.svc.DoStrong(ctx, uid, req.CID, req.GID1, req.Good1Count, req.GID2, req.Good2Count, req.SID, req.IsZuoji)
		return err
	})
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}

type combineReq struct {
	MainSid   int `json:"main_sid"`
	MainFlag  int `json:"main_flag"`
	SubSid1   int `json:"sub_sid1"`
	SubSid2   int `json:"sub_sid2"`
	GoodsFlag int `json:"goods_flag"`
}

func (h *Handler) combine(c *gin.Context) {
	var req combineReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	var out *CombineResult
	err := h.svc.WithUserLock(c.Request.Context(), uid, "armor", func(ctx context.Context) error {
		var err error
		out, err = h.svc.CombineArmor(ctx, uid, req.MainSid, req.MainFlag, req.SubSid1, req.SubSid2, req.GoodsFlag)
		return err
	})
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}

func (h *Handler) initHoles(c *gin.Context) {
	var req repairReq
	if err := c.ShouldBindJSON(&req); err != nil || req.CID <= 0 || req.SID <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	var gold int
	err := h.svc.WithUserLock(c.Request.Context(), uid, "armor", func(ctx context.Context) error {
		var err error
		gold, err = h.svc.InitHoles(ctx, uid, req.CID, req.SID)
		return err
	})
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, gin.H{"reduce_gold": gold})
}

type openHoleReq struct {
	SID     int `json:"sid"`
	GID     int `json:"gid"`
	Pos     int `json:"pos"`
	UseType int `json:"use_type"`
	Count   int `json:"count"`
}

func (h *Handler) openHole(c *gin.Context) {
	var req openHoleReq
	if err := c.ShouldBindJSON(&req); err != nil || req.SID <= 0 || req.GID <= 0 || req.Pos < 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	var out map[string]any
	err := h.svc.WithUserLock(c.Request.Context(), uid, "armor", func(ctx context.Context) error {
		var err error
		out, err = h.svc.OpenHole(ctx, uid, req.SID, req.GID, req.Pos, req.UseType, req.Count)
		return err
	})
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}

// ── R11-3 马厩/坐骑端点（参数序对齐 legacy array_shift）──────────────────────

// barnGoodsReq loadBarnGoods(xilianIndex)：0/1/2。
type barnGoodsReq struct {
	XilianIndex int `json:"xilian_index"`
}

func (h *Handler) barnGoods(c *gin.Context) {
	var req barnGoodsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	out, err := h.svc.LoadBarnGoods(c.Request.Context(), uid, req.XilianIndex)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}

// barnZuojiReq loadZuojiArmor(zuoji_type, armorid)。
type barnZuojiReq struct {
	ZuojiType int `json:"zuoji_type"`
	ArmorID   int `json:"armorid"`
}

func (h *Handler) barnZuojiArmors(c *gin.Context) {
	var req barnZuojiReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	out, err := h.svc.LoadZuojiArmor(c.Request.Context(), uid, req.ZuojiType, req.ArmorID)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}

// barnUnladeReq doUnlade(sid, gid, pos)。
type barnUnladeReq struct {
	SID int `json:"sid"`
	GID int `json:"gid"`
	Pos int `json:"pos"`
}

func (h *Handler) barnUnlade(c *gin.Context) {
	var req barnUnladeReq
	if err := c.ShouldBindJSON(&req); err != nil || req.SID <= 0 || req.GID <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	var out *UnladeResult
	err := h.svc.WithUserLock(c.Request.Context(), uid, "armor", func(ctx context.Context) error {
		var err error
		out, err = h.svc.DoUnlade(ctx, uid, req.SID, req.GID, req.Pos)
		return err
	})
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}

// barnEmbedPearlsReq loadEmbedPearlByArmor(gidstr)。
type barnEmbedPearlsReq struct {
	GIDStr string `json:"gid_str"`
}

func (h *Handler) barnEmbedPearls(c *gin.Context) {
	var req barnEmbedPearlsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	out, err := h.svc.LoadEmbedPearlByArmor(c.Request.Context(), uid, req.GIDStr)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}

func (h *Handler) barnUnactiveHorse(c *gin.Context) {
	uid := uidOf(c)
	out, err := h.svc.LoadUnActiveHorseArmor(c.Request.Context(), uid)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}

// upgradeArmorReq doUpgradeArmor(sid, isProtected)。
type upgradeArmorReq struct {
	SID         int  `json:"sid"`
	IsProtected bool `json:"is_protected"`
}

func (h *Handler) upgradeArmor(c *gin.Context) {
	var req upgradeArmorReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	var out *UpgradeArmorResult
	err := h.svc.WithUserLock(c.Request.Context(), uid, "armor", func(ctx context.Context) error {
		var err error
		out, err = h.svc.DoUpgradeArmor(ctx, uid, req.SID, req.IsProtected)
		return err
	})
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}

type embedReq struct {
	SID     int `json:"sid"`
	Pos     int `json:"pos"`
	GID     int `json:"gid"`
	IsZuoji int `json:"is_zuoji"`
}

func (h *Handler) embed(c *gin.Context) {
	var req embedReq
	if err := c.ShouldBindJSON(&req); err != nil || req.SID <= 0 || req.GID <= 0 || req.Pos < 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	var out *EmbedResult
	err := h.svc.WithUserLock(c.Request.Context(), uid, "armor", func(ctx context.Context) error {
		var err error
		out, err = h.svc.DoEmbed(ctx, uid, req.SID, req.Pos, req.GID, req.IsZuoji)
		return err
	})
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}
