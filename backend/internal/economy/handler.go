package economy

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

// Register 注册 M6 经济端点（写操作 WithUserLock 包裹，对齐 legacy lockUser 区间）。
func (h *Handler) Register(rg *gin.RouterGroup) {
	m := rg.Group("/cities/:cid/market")
	m.GET("/info", h.marketInfo)
	m.POST("/cancel-sell", h.cancelSell)
	m.POST("/cancel-autotrans", h.cancelAutoTrans)
	m.POST("/accelerate", h.accelerateSell)
	m.GET("/merchant", h.merchantInfo)
	m.POST("/buy-merchant", h.buyFromMerchant)
	m.POST("/sell-merchant", h.sellToMerchant)
	m.POST("/sell-user", h.sellToUser)
	m.POST("/buy-user", h.buyFromUser)
	m.GET("/buylist", h.userBuyList)
	m.GET("/selldata", h.userSellData)
	m.GET("/autotrans", h.autoTransList)
	m.POST("/autotrans", h.autoTransAdd)
	m.POST("/autotrans/remove", h.autoTransRemove)
	m.GET("/autotrans/has", h.autoTransHas)

	st := rg.Group("/cities/:cid/store")
	st.GET("/info", h.storeInfo)
	st.POST("/rate", h.storeRate)
	st.POST("/pack", h.storePack)

	ws := rg.Group("/workshop")
	ws.GET("/info", h.workshopInfo)
	ws.POST("/refresh", h.workshopRefresh)
	ws.POST("/buy", h.workshopBuy)

	sh := rg.Group("/shop")
	sh.GET("/info", h.shopInfo)
	sh.POST("/buy", h.buyGoods)
	sh.POST("/buy-before-use", h.buyGoodsBeforeUse)
	sh.POST("/exchange", h.exchangeLiquan)
	sh.GET("/hero-attr", h.heroAttr)
	sh.GET("/armor-attr", h.armorAttr)

	rg.POST("/goods/sell", h.sellGoods)
	rg.POST("/cities/:cid/product-rate", h.productRate)
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

// locked 以 "economy" 为键包裹写操作。
func (h *Handler) locked(c *gin.Context, fn func(context.Context) error) {
	err := h.svc.WithUserLock(c.Request.Context(), uidOf(c), "economy", fn)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
}

// ── 市场 ────────────────────────────────────────────────────────────────

func (h *Handler) marketInfo(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	info, err := h.svc.GetMarketInfo(c.Request.Context(), uidOf(c), cid)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, info)
}

type idReq struct {
	ID int `json:"id"`
}

func (h *Handler) cancelSell(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	var req idReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	var out *MarketInfo
	h.locked(c, func(ctx context.Context) error {
		var err error
		out, err = h.svc.CancelSell(ctx, uid, cid, req.ID)
		return err
	})
	if out != nil {
		c.JSON(200, out)
	}
}

func (h *Handler) cancelAutoTrans(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	var req idReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	var out *MarketInfo
	h.locked(c, func(ctx context.Context) error {
		var err error
		out, err = h.svc.CancelAutoTrans(ctx, uid, cid, req.ID)
		return err
	})
	if out != nil {
		c.JSON(200, out)
	}
}

func (h *Handler) accelerateSell(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	var req idReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	var out *MarketInfo
	h.locked(c, func(ctx context.Context) error {
		var err error
		out, err = h.svc.AccelerateSell(ctx, uid, cid, req.ID)
		return err
	})
	if out != nil {
		c.JSON(200, out)
	}
}

func (h *Handler) merchantInfo(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	var out map[string]any
	h.locked(c, func(ctx context.Context) error {
		var err error
		out, err = h.svc.GetMerchantInfo(ctx, cid)
		return err
	})
	if out != nil {
		c.JSON(200, out)
	}
}

type merchantTradeReq struct {
	Food    int64 `json:"food"`
	Wood    int64 `json:"wood"`
	Rock    int64 `json:"rock"`
	Iron    int64 `json:"iron"`
	Times   int64 `json:"times"`
	PayType int64 `json:"paytype"`
}

func (h *Handler) buyFromMerchant(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	var req merchantTradeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	h.locked(c, func(ctx context.Context) error {
		return h.svc.BuyFromMerchant(ctx, uid, cid, req.Food, req.Wood, req.Rock, req.Iron, req.Times, req.PayType)
	})
}

func (h *Handler) sellToMerchant(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	var req merchantTradeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	h.locked(c, func(ctx context.Context) error {
		return h.svc.SellToMerchant(ctx, uid, cid, req.Food, req.Wood, req.Rock, req.Iron, req.Times, req.PayType)
	})
}

type sellUserReq struct {
	ResType  int64 `json:"resType"`
	Count    int64 `json:"count"`
	Gold     int64 `json:"gold"`
	Hour     int64 `json:"hour"`
	UnionSel int64 `json:"unionOnly"`
}

func (h *Handler) sellToUser(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	var req sellUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	var out *MarketInfo
	h.locked(c, func(ctx context.Context) error {
		var err error
		out, err = h.svc.SellToUser(ctx, uid, cid, req.ResType, req.Count, req.Gold, req.Hour, req.UnionSel)
		return err
	})
	if out != nil {
		c.JSON(200, out)
	}
}

func (h *Handler) buyFromUser(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	var req idReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	var out *MarketInfo
	h.locked(c, func(ctx context.Context) error {
		var err error
		out, err = h.svc.BuyFromUser(ctx, uid, cid, req.ID)
		return err
	})
	if out != nil {
		c.JSON(200, out)
	}
}

func (h *Handler) userBuyList(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "0"))
	filter, _ := strconv.Atoi(c.DefaultQuery("filter", "0"))
	unionOnly, _ := strconv.Atoi(c.DefaultQuery("unionOnly", "0"))
	out, err := h.svc.GetUserBuyList(c.Request.Context(), uidOf(c), cid, page, filter, unionOnly, c.Query("sellName"))
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}

func (h *Handler) userSellData(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	out, err := h.svc.GetUserSellData(c.Request.Context(), cid)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}

func (h *Handler) autoTransList(c *gin.Context) {
	out, err := h.svc.GetAutoTrans(c.Request.Context(), uidOf(c))
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}

type autoTransReq struct {
	FromCid   int64   `json:"fromcid"`
	ToCid     int64   `json:"tocid"`
	ResType   int64   `json:"resType"`
	Count     int64   `json:"count"`
	TransType int64   `json:"transType"`
	StartTime float64 `json:"startTime"` // 客户端毫秒时间戳
}

func (h *Handler) autoTransAdd(c *gin.Context) {
	_, ok := cidOf(c)
	if !ok {
		return
	}
	var req autoTransReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	var out []map[string]any
	h.locked(c, func(ctx context.Context) error {
		var err error
		out, err = h.svc.AddAutoTrans(ctx, uid, req.FromCid, req.ToCid, req.ResType, req.Count, req.TransType, req.StartTime)
		return err
	})
	if out != nil {
		c.JSON(200, out)
	}
}

func (h *Handler) autoTransRemove(c *gin.Context) {
	var req idReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	out, err := h.svc.RemoveAutoTrans(c.Request.Context(), req.ID)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}

func (h *Handler) autoTransHas(c *gin.Context) {
	out, err := h.svc.HasAutoTrans(c.Request.Context(), uidOf(c))
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}

// ── 仓库 ────────────────────────────────────────────────────────────────

func (h *Handler) storeInfo(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	out, err := h.svc.DoGetStoreInfo(c.Request.Context(), uidOf(c), cid)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}

type storeRateReq struct {
	Food int64 `json:"food"`
	Wood int64 `json:"wood"`
	Rock int64 `json:"rock"`
	Iron int64 `json:"iron"`
}

func (h *Handler) storeRate(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	var req storeRateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	// 成功也经由 errLegacy 返回文案（原版 throw），HTTP 400 + message。
	if err := h.svc.ModifyStoreRate(c.Request.Context(), cid, req.Food, req.Wood, req.Rock, req.Iron); err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

type packReq struct {
	Items []PackItem `json:"items"`
}

func (h *Handler) storePack(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	var req packReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	var out []int
	h.locked(c, func(ctx context.Context) error {
		var err error
		out, err = h.svc.PayToPack(ctx, uid, cid, req.Items)
		return err
	})
	if out != nil {
		c.JSON(200, out)
	}
}

// ── 工匠作坊 ────────────────────────────────────────────────────────────

func (h *Handler) workshopInfo(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	out, err := h.svc.LoadInitWorkShopInfo(c.Request.Context(), uidOf(c), cid)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}

type workshopRefreshReq struct {
	CID         int `json:"cid"`
	IsNeedWuzhu int `json:"isNeedWuzhu"`
}

func (h *Handler) workshopRefresh(c *gin.Context) {
	var req workshopRefreshReq
	if err := c.ShouldBindJSON(&req); err != nil || req.CID <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	var out []any
	h.locked(c, func(ctx context.Context) error {
		var err error
		out, err = h.svc.RefreshWorkShop(ctx, uid, req.CID, req.IsNeedWuzhu)
		return err
	})
	if out != nil {
		c.JSON(200, out)
	}
}

type workshopBuyReq struct {
	CID int `json:"cid"`
	GID int `json:"gid"`
}

func (h *Handler) workshopBuy(c *gin.Context) {
	var req workshopBuyReq
	if err := c.ShouldBindJSON(&req); err != nil || req.CID <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	var out []any
	h.locked(c, func(ctx context.Context) error {
		var err error
		out, err = h.svc.BuyWorkShopGood(ctx, uid, req.CID, req.GID)
		return err
	})
	if out != nil {
		c.JSON(200, out)
	}
}

// ── 商城 ────────────────────────────────────────────────────────────────

func (h *Handler) shopInfo(c *gin.Context) {
	out, err := h.svc.GetShopInfo(c.Request.Context(), uidOf(c))
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}

type buyGoodsReq struct {
	ID      int `json:"id"`
	Cnt     int `json:"cnt"`
	PayType int `json:"paytype"`
	GID     int `json:"gid"`
}

func (h *Handler) buyGoods(c *gin.Context) {
	var req buyGoodsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	var out []any
	h.locked(c, func(ctx context.Context) error {
		var err error
		out, err = h.svc.BuyGoods(ctx, uid, req.ID, req.Cnt, req.PayType, req.GID)
		return err
	})
	if out != nil {
		c.JSON(200, out)
	}
}

func (h *Handler) buyGoodsBeforeUse(c *gin.Context) {
	var req buyGoodsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	var out []any
	h.locked(c, func(ctx context.Context) error {
		var err error
		out, err = h.svc.BuyGoodsBeforeUse(ctx, uid, req.ID, req.Cnt, req.PayType)
		return err
	})
	if out != nil {
		c.JSON(200, out)
	}
}

type exchangeReq struct {
	Code string `json:"code"`
}

func (h *Handler) exchangeLiquan(c *gin.Context) {
	var req exchangeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	var out []any
	h.locked(c, func(ctx context.Context) error {
		var err error
		out, err = h.svc.ExchangeLiquan(ctx, uid, req.Code)
		return err
	})
	if out != nil {
		c.JSON(200, out)
	}
}

func (h *Handler) heroAttr(c *gin.Context) {
	hid, _ := strconv.Atoi(c.Query("hid"))
	if err := h.svc.GetGoodsHeroAttr(c.Request.Context(), hid); err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (h *Handler) armorAttr(c *gin.Context) {
	aid, _ := strconv.Atoi(c.Query("aid"))
	out, err := h.svc.GetGoodsArmorAttr(c.Request.Context(), aid)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}

type sellGoodsReq struct {
	CID   int `json:"cid"`
	GID   int `json:"gid"`
	Count int `json:"count"`
}

func (h *Handler) sellGoods(c *gin.Context) {
	var req sellGoodsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	uid := uidOf(c)
	var out []any
	h.locked(c, func(ctx context.Context) error {
		var err error
		out, err = h.svc.SellGoods(ctx, uid, req.CID, req.GID, req.Count)
		return err
	})
	if out != nil {
		c.JSON(200, out)
	}
}

type productRateReq struct {
	Food int64 `json:"food"`
	Wood int64 `json:"wood"`
	Rock int64 `json:"rock"`
	Iron int64 `json:"iron"`
}

func (h *Handler) productRate(c *gin.Context) {
	cid, ok := cidOf(c)
	if !ok {
		return
	}
	var req productRateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	out, err := h.svc.SetCityProductRate(c.Request.Context(), uidOf(c), cid, req.Food, req.Wood, req.Rock, req.Iron)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, out)
}
