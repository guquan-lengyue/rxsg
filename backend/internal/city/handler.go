package city

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

// Register 在已挂 RequireAuth 的路由组上注册城市相关端点。
func (h *Handler) Register(rg *gin.RouterGroup) {
	g := rg.Group("/cities")
	g.GET("", h.list)
	g.GET("/:cid", h.detail)
	g.GET("/:cid/resources", h.resources)
	g.GET("/:cid/buildings", h.buildings)
	g.GET("/:cid/technics", h.technics)
	g.GET("/:cid/troops", h.troops)
	g.GET("/:cid/defences", h.defences)
	g.GET("/:cid/heroes", h.heroes)
	g.GET("/:cid/alarms", h.alarms)
}

// cidParam 解析并校验路径中的 cid。
func cidParam(c *gin.Context) (int, bool) {
	n, err := strconv.Atoi(c.Param("cid"))
	if err != nil || n <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_cid", "cid 非法"))
		return 0, false
	}
	return n, true
}

func (h *Handler) list(c *gin.Context) {
	cities, err := h.svc.ListCities(c.Request.Context(), c.GetInt(auth.CtxUID))
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, cities)
}

func (h *Handler) detail(c *gin.Context) {
	cid, ok := cidParam(c)
	if !ok {
		return
	}
	detail, err := h.svc.GetCityDetail(c.Request.Context(), c.GetInt(auth.CtxUID), cid)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, detail)
}

func (h *Handler) resources(c *gin.Context) {
	cid, ok := cidParam(c)
	if !ok {
		return
	}
	res, err := h.svc.Resources(c.Request.Context(), c.GetInt(auth.CtxUID), cid)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, res)
}

func (h *Handler) buildings(c *gin.Context) {
	cid, ok := cidParam(c)
	if !ok {
		return
	}
	v, err := h.svc.GetBuildings(c.Request.Context(), c.GetInt(auth.CtxUID), cid)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

func (h *Handler) technics(c *gin.Context) {
	cid, ok := cidParam(c)
	if !ok {
		return
	}
	v, err := h.svc.GetTechnics(c.Request.Context(), c.GetInt(auth.CtxUID), cid)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

func (h *Handler) troops(c *gin.Context) {
	cid, ok := cidParam(c)
	if !ok {
		return
	}
	v, err := h.svc.GetTroops(c.Request.Context(), c.GetInt(auth.CtxUID), cid)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

func (h *Handler) defences(c *gin.Context) {
	cid, ok := cidParam(c)
	if !ok {
		return
	}
	v, err := h.svc.GetDefences(c.Request.Context(), c.GetInt(auth.CtxUID), cid)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

func (h *Handler) heroes(c *gin.Context) {
	cid, ok := cidParam(c)
	if !ok {
		return
	}
	v, err := h.svc.GetHeroes(c.Request.Context(), c.GetInt(auth.CtxUID), cid)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

func (h *Handler) alarms(c *gin.Context) {
	cid, ok := cidParam(c)
	if !ok {
		return
	}
	v, err := h.svc.GetAlarms(c.Request.Context(), c.GetInt(auth.CtxUID), cid)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}