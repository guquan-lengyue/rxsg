package battle

// service.go —— 战斗回合引擎的服务层：战斗快照 load/save、调度（HandleBattle）、
// 回合推进（updateBattle→return_new_place）。
//
// 1:1 对照（server/game/OLdBattleCron.php）：
//   HandleBattle      L2-13   扫 mem_battle where nexttime<=now → updateBattle
//   updateBattle      L14-22  → return_new_place
//   return_new_place  L23-530 初始化 + 单回合结算 + mem_battle 回写 + sys_battle_report
//   结束处理          L487-529 sys_battle.state=1 + troops 回程/占领
//
// 新栈差异（已声明）：
//   - PHP $_SESSION['battle'][id]（fightsoldier/resistnpc/attacknpc/battle_end）→ battles.engine_json 持久化。
//   - 无 cron，由 army 入口惰性结算（每次请求补跑到期回合，nexttime 链式 +25s，对齐 25 秒/回合）。
//   - 无战场/联盟/洛阳（type>=4）与 mem_world：target 用 fields / cities（见 create.go）。

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"rxsg/backend/internal/db"
)

// Service 战斗服务。
type Service struct {
	db *db.DB
}

func NewService(d *db.DB) *Service { return &Service{db: d} }

// roundSeconds 对齐 OLdBattleCron.php:482 `$time = time()+25`（25 秒/回合）。
const roundSeconds = 25

// maxSettleRounds 单次惰性结算补跑回合上限（防 nexttime 被回拨导致死循环）。
const maxSettleRounds = 60

// Battle 是 battles 表一行（mem_battle + sys_battle 合并快照）。
type Battle struct {
	ID             int
	Type           int
	State          int
	Result         int
	Starttime      int64
	CID            int
	AttackUID      int
	ResistUID      int
	AttackTroop    int
	ResistTroops   string
	ResistDefence  string
	Round          int
	Nexttime       int64
	AttackCID      int
	AttackHID      int
	AttackSoldiers string
	AttackPos      string
	AttackAdd      string
	ResistCID      int
	ResistHID      int
	ResistSoldiers string
	ResistPos      string
	ResistAdd      string
	WallHP         float64
	WallLevel      int
	FieldRange     float64
	Level          int
	AttackStartCID int
	ResistStartCID int
	EngineJSON     string
}

func battleFromMap(m map[string]any) *Battle {
	return &Battle{
		ID:             int(modelInt64(m, "id")),
		Type:           int(modelInt64(m, "type")),
		State:          int(modelInt64(m, "state")),
		Result:         int(modelInt64(m, "result")),
		Starttime:      modelInt64(m, "starttime"),
		CID:            int(modelInt64(m, "cid")),
		AttackUID:      int(modelInt64(m, "attackuid")),
		ResistUID:      int(modelInt64(m, "resistuid")),
		AttackTroop:    int(modelInt64(m, "attacktroop")),
		ResistTroops:   modelStr(m, "resisttroops"),
		ResistDefence:  modelStr(m, "resistdefence"),
		Round:          int(modelInt64(m, "round")),
		Nexttime:       modelInt64(m, "nexttime"),
		AttackCID:      int(modelInt64(m, "attackcid")),
		AttackHID:      int(modelInt64(m, "attackhid")),
		AttackSoldiers: modelStr(m, "attacksoldiers"),
		AttackPos:      modelStr(m, "attackpos"),
		AttackAdd:      modelStr(m, "attackadd"),
		ResistCID:      int(modelInt64(m, "resistcid")),
		ResistHID:      int(modelInt64(m, "resisthid")),
		ResistSoldiers: modelStr(m, "resistsoldiers"),
		ResistPos:      modelStr(m, "resistpos"),
		ResistAdd:      modelStr(m, "resistadd"),
		WallHP:         modelFloat(m, "wallhp"),
		WallLevel:      int(modelInt64(m, "walllevel")),
		FieldRange:     modelFloat(m, "fieldrange"),
		Level:          int(modelInt64(m, "level")),
		AttackStartCID: int(modelInt64(m, "attackstartcid")),
		ResistStartCID: int(modelInt64(m, "resiststartcid")),
		EngineJSON:     modelStr(m, "engine_json"),
	}
}

func modelStr(m map[string]any, k string) string {
	switch v := m[k].(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case nil:
		return ""
	}
	return ""
}

// loadBattle 读取一场战斗。
func (s *Service) loadBattle(ctx context.Context, id int) (*Battle, error) {
	row, err := s.db.FetchOne(ctx, "select * from battles where id=? limit 1", id)
	if err != nil {
		return nil, err
	}
	return battleFromMap(row), nil
}

// engineState 是 engine_json 的载体（替代 $_SESSION['battle'][id]['fightsoldier']）。
type engineState struct {
	Fight []*Unit `json:"f"`
	// ResistNPC/AttackNPC 对齐 $_SESSION['battle'][id]['resistnpc'|'attacknpc']（仅记首回合）。
	ResistNPC bool `json:"rn"`
	AttackNPC bool `json:"an"`
}

func (b *Battle) loadEngine() (*engineState, error) {
	if b.EngineJSON == "" {
		return nil, nil
	}
	var st engineState
	if err := json.Unmarshal([]byte(b.EngineJSON), &st); err != nil {
		return nil, err
	}
	return &st, nil
}

func (b *Battle) saveEngine(st *engineState) error {
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	b.EngineJSON = string(raw)
	return nil
}

// tacticsMap 对齐 battleTactic（cfg_js.php:684）：[attack][stype] → {action,target}。
type tacticsMap map[int]map[int]tactic

// tactic 是单条战术（action: 1前进 3后退；target: 目标兵种 type 值，15=箭塔）。
type tactic struct {
	Action float64
	Target float64
}

func (s *Service) loadTactics(ctx context.Context, battleID int) (tacticsMap, error) {
	rows, err := s.db.FetchRows(ctx, "select * from battle_tactics where battleid=?", battleID)
	if err != nil {
		return nil, err
	}
	out := tacticsMap{}
	for _, r := range rows {
		atk := int(modelInt64(r, "attack"))
		st := int(modelInt64(r, "stype"))
		if out[atk] == nil {
			out[atk] = map[int]tactic{}
		}
		out[atk][st] = tactic{
			Action: modelFloat(r, "action"),
			Target: modelFloat(r, "target"),
		}
	}
	return out, nil
}

func (t tacticsMap) get(attack, stype int) tactic {
	if t == nil {
		return tactic{}
	}
	if m, ok := t[attack]; ok {
		if v, ok := m[stype]; ok {
			return v
		}
	}
	return tactic{}
}

// isPlayer 新库适配：玩家 = uid>0 且 users 存在。
// legacy 以 uid>1000 区分 NPC/玩家；新库玩家 id 从 1 起，NPC 仅有 uid=0/无行。
func (s *Service) isPlayer(ctx context.Context, uid int) bool {
	if uid <= 0 {
		return false
	}
	n, err := s.db.FetchCellInt64(ctx, "select count(*) from users where id=?", uid)
	return err == nil && n > 0
}

// IsNPC 对齐 is_npc（OLdBattleCron.php:634）：uid 空/0 或不存在该用户。见 isPlayer 的新库适配说明。
func (s *Service) IsNPC(ctx context.Context, uid int) bool {
	return !s.isPlayer(ctx, uid)
}

// HandleBattle 对齐 OLdBattleCron.php:2：结算所有到期战斗（state=1 的删除快照）。
// 新库无 mem_battle 删除语义：state=1 表示已结束，保留 battles 行供战报查询。
func (s *Service) HandleBattle(ctx context.Context, now int64) error {
	rows, err := s.db.FetchRows(ctx,
		"select id from battles where state=0 and nexttime<=? order by nexttime, id", now)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if err := s.updateBattle(ctx, int(modelInt64(r, "id")), now); err != nil {
			return err
		}
	}
	return nil
}

// updateBattle 对齐 OLdBattleCron.php:14：补跑到期回合。
// 新栈适配：legacy 由 cron 每 25 秒 tick 一次；此处以「到期时间」为 tick 逐轮回放
// （tick = 上一次 nexttime，回合内 nexttime = tick+25），从而请求驱动下也能追上进度。
func (s *Service) updateBattle(ctx context.Context, battleID int, now int64) error {
	b, err := s.loadBattle(ctx, battleID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	for i := 0; i < maxSettleRounds && b.State == 0 && b.Nexttime <= now; i++ {
		tick := b.Nexttime
		ended, err := s.returnNewPlace(ctx, b, tick)
		if err != nil {
			return err
		}
		if ended {
			break
		}
	}
	return nil
}

// SettleForUser 惰性结算与 uid 相关的所有战斗（进攻方或防守方），返回已结束的战斗 id。
// army 入口在每次军队读写时调用（替代 legacy 全局 cron）。
func (s *Service) SettleForUser(ctx context.Context, uid int) ([]int, error) {
	now, err := s.db.Now(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.FetchRows(ctx,
		"select id from battles where state=0 and (attackuid=? or resistuid=?) and nexttime<=? order by nexttime, id",
		uid, uid, now)
	if err != nil {
		return nil, err
	}
	var done []int
	for _, r := range rows {
		id := int(modelInt64(r, "id"))
		if err := s.updateBattle(ctx, id, now); err != nil {
			return nil, err
		}
		b, err := s.loadBattle(ctx, id)
		if err != nil {
			return nil, err
		}
		if b.State == 1 {
			done = append(done, id)
		}
	}
	return done, nil
}

// dueBattleIDs 返回所有到期战斗 id（供 cron 或测试驱动）。
func (s *Service) dueBattleIDs(ctx context.Context, now int64) ([]int, error) {
	rows, err := s.db.FetchRows(ctx,
		"select id from battles where state=0 and nexttime<=? order by nexttime, id", now)
	if err != nil {
		return nil, err
	}
	out := make([]int, 0, len(rows))
	for _, r := range rows {
		out = append(out, int(modelInt64(r, "id")))
	}
	return out, nil
}
