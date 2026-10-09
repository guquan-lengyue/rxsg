package achievement

// handler.go 暴露 M8 成就 HTTP 端点（legacy 经 AMF 网关分派 AchivementFunc.php）。
// uid 默认取登录态，可显式传 ?uid= 查看他人（对齐 legacy 以 param uid 为准的语义）。

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

func (h *Handler) Register(rg *gin.RouterGroup) {
	g := rg.Group("/achievements")
	g.GET("/overview", h.overview) // getOverviewStat
	g.GET("/group", h.byGroup)     // getAchivementsByGroup
	g.GET("/detail", h.detail)     // getAchivementDetail
}

func uidOf(c *gin.Context) int {
	if s := c.Query("uid"); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			return n
		}
	}
	return c.GetInt(auth.CtxUID)
}

func (h *Handler) overview(c *gin.Context) {
	v, err := h.svc.GetOverviewStat(c.Request.Context(), uidOf(c))
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

func (h *Handler) byGroup(c *gin.Context) {
	group, _ := strconv.Atoi(c.DefaultQuery("group", "0"))
	subgroup, _ := strconv.Atoi(c.DefaultQuery("subgroup", "0"))
	typ, _ := strconv.Atoi(c.DefaultQuery("type", "0"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "0"))
	v, err := h.svc.GetAchivementsByGroup(c.Request.Context(), uidOf(c), group, subgroup, typ, page)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

func (h *Handler) detail(c *gin.Context) {
	aid, err := strconv.Atoi(c.Query("aid"))
	if err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "aid 非法"))
		return
	}
	v, err := h.svc.GetAchivementDetail(c.Request.Context(), uidOf(c), aid)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}
