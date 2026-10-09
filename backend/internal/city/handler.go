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

	// M1 城市内政（CityFunc.php changeTax/levyResource/pacifyPeople/getCityProduct）
	g.POST("/:cid/tax", h.tax)
	g.POST("/:cid/levy", h.levy)
	g.POST("/:cid/pacify", h.pacify)
	g.GET("/:cid/product", h.product)
}

type taxReq struct {
	Tax int `json:"tax"`
}

type levyReq struct {
	ResID int `json:"resid"`
}

type pacifyReq struct {
	Action int `json:"action"`
}

func (h *Handler) tax(c *gin.Context) {
	cid, ok := cidParam(c)
	if !ok {
		return
	}
	var req taxReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	res, err := h.svc.ChangeTax(c.Request.Context(), c.GetInt(auth.CtxUID), cid, req.Tax)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, res)
}

func (h *Handler) levy(c *gin.Context) {
	cid, ok := cidParam(c)
	if !ok {
		return
	}
	var req levyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	msg, res, err := h.svc.LevyResource(c.Request.Context(), c.GetInt(auth.CtxUID), cid, req.ResID)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, gin.H{"message": msg, "resource": res})
}

func (h *Handler) pacify(c *gin.Context) {
	cid, ok := cidParam(c)
	if !ok {
		return
	}
	var req pacifyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	msg, res, err := h.svc.PacifyPeople(c.Request.Context(), c.GetInt(auth.CtxUID), cid, req.Action)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, gin.H{"message": msg, "resource": res})
}

func (h *Handler) product(c *gin.Context) {
	cid, ok := cidParam(c)
	if !ok {
		return
	}
	p, err := h.svc.GetCityProduct(c.Request.Context(), c.GetInt(auth.CtxUID), cid)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, p)
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