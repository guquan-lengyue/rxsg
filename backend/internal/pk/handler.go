package pk

// handler.go 暴露 M9 单机 PK 征战 HTTP 端点。protected 组已挂 RequireAuth。
// legacy 通过 AMF 网关按函数名分派 PKFunc.php，RPC 白名单（lang.php:2682）：
//   loadCampaignInitData / loadPKRewardRank / getPkFirstReward / getOneBattleRet /
//   regetCampaignMaxData / buyJunlingFunc / getAllHeroByUid
// 沙场/竞技场相关 RPC（getCrossShaChangInfo/startArenaChallenge/...）按用户确认裁剪，不建路由。

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"rxsg/backend/internal/auth"
	"rxsg/backend/internal/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Register(rg *gin.RouterGroup) {
	g := rg.Group("/pk")
	g.GET("/campaign", h.campaign)        // loadCampaignInitData
	g.GET("/campaign-max", h.campaignMax) // regetCampaignMaxData
	g.GET("/reward-rank", h.rewardRank)   // loadPKRewardRank
	g.GET("/heroes", h.heroes)            // getAllHeroByUid
	g.POST("/first-reward", h.firstReward) // getPkFirstReward
	g.POST("/battle", h.battle)           // getOneBattleRet
	g.POST("/buy-junling", h.buyJunling)  // buyJunlingFunc
}

func (h *Handler) campaign(c *gin.Context) {
	v, err := h.svc.loadCampaignInitData(c.Request.Context(), c.GetInt(auth.CtxUID))
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

func (h *Handler) campaignMax(c *gin.Context) {
	v, err := h.svc.regetCampaignMaxData(c.Request.Context(), c.GetInt(auth.CtxUID))
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

func (h *Handler) rewardRank(c *gin.Context) {
	battleID, err := strconv.Atoi(c.Query("battle_id"))
	if err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	typ, _ := strconv.Atoi(c.DefaultQuery("type", "0"))
	v, err := h.svc.loadPKRewardRank(c.Request.Context(), c.GetInt(auth.CtxUID), battleID, typ)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

func (h *Handler) heroes(c *gin.Context) {
	page, err := strconv.Atoi(c.Query("page"))
	if err != nil || page <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	var hids []int
	if s := strings.TrimSpace(c.Query("hids")); s != "" {
		for _, p := range strings.Split(s, ",") {
			if n, err := strconv.Atoi(strings.TrimSpace(p)); err == nil {
				hids = append(hids, n)
			}
		}
	}
	v, err := h.svc.getAllHeroByUid(c.Request.Context(), c.GetInt(auth.CtxUID), hids, page)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

type firstRewardReq struct {
	BattleID int `json:"battle_id"`
	Flag     int `json:"flag"`
	RankID   int `json:"rank_id"`
}

func (h *Handler) firstReward(c *gin.Context) {
	var req firstRewardReq
	if err := c.ShouldBindJSON(&req); err != nil || req.BattleID <= 0 || req.RankID <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	v, err := h.svc.getPkFirstReward(c.Request.Context(), c.GetInt(auth.CtxUID), req.BattleID, req.Flag, req.RankID)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

type battleReq struct {
	BattleID int   `json:"battle_id"`
	Flag     int   `json:"flag"`
	Level    int   `json:"level"`
	Hids     []int `json:"hids"`
}

func (h *Handler) battle(c *gin.Context) {
	var req battleReq
	if err := c.ShouldBindJSON(&req); err != nil || req.BattleID <= 0 || len(req.Hids) < 3 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	v, err := h.svc.getOneBattleRet(c.Request.Context(), c.GetInt(auth.CtxUID), req.BattleID, req.Flag, req.Level, req.Hids)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

type buyJunlingReq struct {
	Type  int `json:"type"`
	Count int `json:"count"`
}

func (h *Handler) buyJunling(c *gin.Context) {
	var req buyJunlingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	v, err := h.svc.buyJunlingFunc(c.Request.Context(), c.GetInt(auth.CtxUID), req.Type, req.Count)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}
