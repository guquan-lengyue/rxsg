package lottery

// handler.go 暴露 M9 抽奖 HTTP 端点。路由形态对齐 army/task handler：protected 组已挂 RequireAuth。
// legacy 通过 AMF 网关按函数名分派 LotteryFunc.php，RPC 白名单（lang.php:2675）：
//   startLottery / getLotteryReward / autoGetReward / getTodayCount / getWin / addCount / restart / useMoney
// 对应端点如下（getWin 为内部函数，不入白名单）。

import (
	"github.com/gin-gonic/gin"

	"rxsg/backend/internal/auth"
	"rxsg/backend/internal/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Register(rg *gin.RouterGroup) {
	g := rg.Group("/lottery")
	g.GET("/today-count", h.todayCount) // getTodayCount
	g.GET("/check-money", h.checkMoney) // checkLotteryMoney
	g.POST("/start", h.start)           // startLottery
	g.POST("/reward", h.reward)         // getLotteryReward
	g.POST("/auto-reward", h.autoReward) // autoGetReward
	g.POST("/win", h.getWin)            // addCount（领取当前盘面奖励）
	g.POST("/restart", h.restart)       // restart
	g.POST("/use-money", h.useMoney)    // useMoney
}

func (h *Handler) todayCount(c *gin.Context) {
	v, err := h.svc.GetTodayCount(c.Request.Context(), c.GetInt(auth.CtxUID))
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

func (h *Handler) checkMoney(c *gin.Context) {
	v, err := h.svc.CheckLotteryMoney(c.Request.Context(), c.GetInt(auth.CtxUID))
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

func (h *Handler) start(c *gin.Context) {
	v, err := h.svc.StartLottery(c.Request.Context(), c.GetInt(auth.CtxUID))
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

func (h *Handler) reward(c *gin.Context) {
	v, err := h.svc.GetLotteryReward(c.Request.Context(), c.GetInt(auth.CtxUID))
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

func (h *Handler) autoReward(c *gin.Context) {
	v, err := h.svc.AutoGetReward(c.Request.Context(), c.GetInt(auth.CtxUID))
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

func (h *Handler) getWin(c *gin.Context) {
	v, err := h.svc.AddCount(c.Request.Context(), c.GetInt(auth.CtxUID))
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

type restartReq struct {
	WinID   int `json:"win_id"`
	WinType int `json:"win_type"`
}

func (h *Handler) restart(c *gin.Context) {
	var req restartReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	v, err := h.svc.Restart(c.Request.Context(), c.GetInt(auth.CtxUID), req.WinID, req.WinType)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

type useMoneyReq struct {
	Count int `json:"count"`
}

func (h *Handler) useMoney(c *gin.Context) {
	var req useMoneyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	v, err := h.svc.UseMoney(c.Request.Context(), c.GetInt(auth.CtxUID), req.Count)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}
