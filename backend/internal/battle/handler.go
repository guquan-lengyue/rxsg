package battle

// handler.go —— 战斗查询端点（进行中战斗、逐回合战报、战斗战报）。

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

// Register 在已挂 RequireAuth 的路由组上注册战斗端点。
func (h *Handler) Register(rg *gin.RouterGroup) {
	g := rg.Group("/cities/:cid/battles")
	g.GET("", h.list)
	g.GET("/reports", h.reports)
	rg.GET("/battles/:bid", h.detail)
}

// BattleBrief 进行中/已结束战斗摘要。
type BattleBrief struct {
	ID         int   `json:"id"`
	Type       int   `json:"type"`
	State      int   `json:"state"`
	Result     int   `json:"result"`
	Round      int   `json:"round"`
	AttackUID  int   `json:"attack_uid"`
	ResistUID  int   `json:"resist_uid"`
	AttackCID  int   `json:"attack_cid"`
	ResistCID  int   `json:"resist_cid"`
	Nexttime   int64 `json:"nexttime"`
	TimeLeft   int64 `json:"time_left"`
}

// RoundReport 逐回合原始战报行（sys_battle_report）。
type RoundReport struct {
	Round  int    `json:"round"`
	Report string `json:"report"`
}

// ListByCity 返回与 uid 相关、且属于该城的战斗。
func (s *Service) ListByCity(ctx context.Context, uid, cid int) ([]BattleBrief, error) {
	if err := s.ensureCityOwner(ctx, uid, cid); err != nil {
		return nil, err
	}
	now, err := s.db.Now(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.FetchRows(ctx,
		"select * from battles where attackcid=? or resistcid=? order by id desc limit 200", cid, cid)
	if err != nil {
		return nil, err
	}
	out := make([]BattleBrief, 0, len(rows))
	for _, r := range rows {
		bb := BattleBrief{
			ID:        int(modelInt64(r, "id")),
			Type:      int(modelInt64(r, "type")),
			State:     int(modelInt64(r, "state")),
			Result:    int(modelInt64(r, "result")),
			Round:     int(modelInt64(r, "round")),
			AttackUID: int(modelInt64(r, "attackuid")),
			ResistUID: int(modelInt64(r, "resistuid")),
			AttackCID: int(modelInt64(r, "attackcid")),
			ResistCID: int(modelInt64(r, "resistcid")),
			Nexttime:  modelInt64(r, "nexttime"),
		}
		if bb.State == 0 && bb.Nexttime > now {
			bb.TimeLeft = bb.Nexttime - now
		}
		out = append(out, bb)
	}
	return out, nil
}

// Detail 返回一场战斗及其逐回合战报（仅限参战双方）。
func (s *Service) Detail(ctx context.Context, uid, battleID int) (*BattleBrief, []RoundReport, error) {
	b, err := s.loadBattle(ctx, battleID)
	if err != nil {
		return nil, nil, err
	}
	if b.AttackUID != uid && b.ResistUID != uid {
		return nil, nil, httpx.Forbidden("not_battle_member", "非本场战斗参战方")
	}
	bb := &BattleBrief{
		ID: b.ID, Type: b.Type, State: b.State, Result: b.Result, Round: b.Round,
		AttackUID: b.AttackUID, ResistUID: b.ResistUID, AttackCID: b.AttackCID,
		ResistCID: b.ResistCID, Nexttime: b.Nexttime,
	}
	rows, err := s.db.FetchRows(ctx,
		"select round, report from battle_rounds where battleid=? order by round", battleID)
	if err != nil {
		return nil, nil, err
	}
	rounds := make([]RoundReport, 0, len(rows))
	for _, r := range rows {
		rounds = append(rounds, RoundReport{
			Round:  int(modelInt64(r, "round")),
			Report: modelStr(r, "report"),
		})
	}
	return bb, rounds, nil
}

// Reports 返回该城相关的战斗战报（reports 表 battleid>0）。
func (s *Service) Reports(ctx context.Context, uid, cid int) ([]map[string]any, error) {
	if err := s.ensureCityOwner(ctx, uid, cid); err != nil {
		return nil, err
	}
	return s.db.FetchRows(ctx,
		"select id, title, `type`, `time`, battleid, content from reports where battleid>0 and (origincid=? or happencid=?) order by id desc limit 100",
		cid, cid)
}

func (s *Service) ensureCityOwner(ctx context.Context, uid, cid int) error {
	ok, err := s.db.Exists(ctx, "select 1 from cities where id=? and user_id=? limit 1", cid, uid)
	if err != nil {
		return err
	}
	if !ok {
		return httpx.Forbidden("not_user_city", "该城池不属于当前用户")
	}
	return nil
}

func (h *Handler) list(c *gin.Context) {
	cid, ok := cityParam(c)
	if !ok {
		return
	}
	v, err := h.svc.ListByCity(c.Request.Context(), c.GetInt(auth.CtxUID), cid)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

func (h *Handler) reports(c *gin.Context) {
	cid, ok := cityParam(c)
	if !ok {
		return
	}
	v, err := h.svc.Reports(c.Request.Context(), c.GetInt(auth.CtxUID), cid)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, v)
}

func (h *Handler) detail(c *gin.Context) {
	bid, err := strconv.Atoi(c.Param("bid"))
	if err != nil || bid <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_bid", "bid 非法"))
		return
	}
	bb, rounds, err := h.svc.Detail(c.Request.Context(), c.GetInt(auth.CtxUID), bid)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(200, gin.H{"battle": bb, "rounds": rounds})
}

func cityParam(c *gin.Context) (int, bool) {
	n, err := strconv.Atoi(c.Param("cid"))
	if err != nil || n <= 0 {
		httpx.WriteError(c, httpx.BadRequest("invalid_cid", "cid 非法"))
		return 0, false
	}
	return n, true
}
