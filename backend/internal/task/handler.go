package task

// handler.go 暴露 M8 任务 HTTP 端点。路由形态对齐 army/handler.go：
//   /cities/:cid/tasks...（protected 组已挂 RequireAuth）。
// legacy 是通过 AMF 网关按函数名分派 TaskFunc.php，这里按端点逐一映射。

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
	g := rg.Group("/cities/:cid/tasks")
	g.GET("/groups", h.typeGroupList) // getTaskTypeGroupList
	g.GET("", h.allByType)            // getAllTaskByType
	g.GET("/list", h.taskList)        // getTaskList
	g.GET("/detail", h.detail)        // getTaskDetail
	g.POST("/reward", h.reward)       // getReward
	g.POST("/drop", h.drop)           // dropTask
	g.POST("/sys/drop", h.dropSys)    // dropSysTask
}

func (h *Handler) typeGroupList(c *gin.Context) {
	typ, _ := strconv.Atoi(c.DefaultQuery("type", "0"))
	v, err := h.svc.GetTaskTypeGroupList(c.Request.Context(), c.GetInt(auth.CtxUID), typ)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

func (h *Handler) allByType(c *gin.Context) {
	typ, _ := strconv.Atoi(c.DefaultQuery("type", "0"))
	v, err := h.svc.GetAllTaskByType(c.Request.Context(), c.GetInt(auth.CtxUID), typ)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

func (h *Handler) taskList(c *gin.Context) {
	group, err := strconv.Atoi(c.Query("group"))
	if err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "group 非法"))
		return
	}
	v, err := h.svc.GetTaskList(c.Request.Context(), c.GetInt(auth.CtxUID), group)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

func (h *Handler) detail(c *gin.Context) {
	tid, err := strconv.Atoi(c.Query("tid"))
	if err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "tid 非法"))
		return
	}
	v, err := h.svc.GetTaskDetail(c.Request.Context(), c.GetInt(auth.CtxUID), tid)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

type rewardReq struct {
	TID       int `json:"tid"`
	SelectID  int `json:"select_id"`
	SelectID2 int `json:"select_id2"`
}

func (h *Handler) reward(c *gin.Context) {
	var req rewardReq
	if err := c.ShouldBindJSON(&req); err != nil || req.TID <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	v, err := h.svc.GetReward(c.Request.Context(), c.GetInt(auth.CtxUID), req.TID, req.SelectID, req.SelectID2)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

type dropReq struct {
	TaskGroup int `json:"taskgroup"`
}

func (h *Handler) drop(c *gin.Context) {
	var req dropReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	v, err := h.svc.DropTask(c.Request.Context(), c.GetInt(auth.CtxUID), req.TaskGroup)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

type dropSysReq struct {
	TID int `json:"tid"`
}

func (h *Handler) dropSys(c *gin.Context) {
	var req dropSysReq
	if err := c.ShouldBindJSON(&req); err != nil || req.TID <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_param", "参数非法"))
		return
	}
	v, err := h.svc.DropSysTask(c.Request.Context(), c.GetInt(auth.CtxUID), req.TID)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}
