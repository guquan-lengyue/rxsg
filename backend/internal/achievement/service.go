package achievement

// service.go 1:1 复刻 legacy server/game/AchivementFunc.php（全文 98 行）：
//   getOverviewStat(:7) / getAchivementsByGroup(:29) / getAchivementDetail(:56)
// 表映射：cfg_achivement→cfg_achivements、cfg_achivement_group→cfg_achivement_groups、
//   cfg_achivement_goal→cfg_achivement_goals、cfg_achivement_goal_mapping→cfg_achivement_goal_mappings、
//   sys_user_achivement→user_achivements、sys_user.achivement_point→users.achivement_point。
//
// 裁剪：open_sql 里 mem_state state=5（黄巾史诗完成）恒不满足（无 mem_state 表），只保留"脱离新手保护"分支；
//   因此可见 open_time 集合为 {0} 或 {0,1}，与原版 mem_state 缺省为 0 的行为一致。
//   ACHIVE_PAGE_CPP=5。

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"rxsg/backend/internal/db"
	"rxsg/backend/internal/model"
)

const achivePageCPP = 5

type Service struct {
	db *db.DB
}

func NewService(d *db.DB) *Service { return &Service{db: d} }

// GetOverviewStat 对应 getOverviewStat：返回 [总成就点数, 最近两项成就, 分组统计(含 finish_count)]。
func (s *Service) GetOverviewStat(ctx context.Context, uid int) ([]any, error) {
	point, err := s.db.FetchCellInt64(ctx, "select achivement_point from users where id=?", uid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	recent, err := s.db.FetchRows(ctx,
		"SELECT b.id,b.name,b.content,b.point,CASE WHEN a.time IS NULL THEN '--' ELSE a.time END AS achieveGetTime,b.image "+
			"FROM user_achivements a,cfg_achivements b WHERE a.achivement_id=b.id AND a.uid=? ORDER BY a.time DESC LIMIT 2", uid)
	if err != nil {
		return nil, err
	}
	totals, err := s.db.FetchRows(ctx,
		"SELECT a.`group`,b.name AS group_name,COUNT(0) AS total_count,0 AS finish_count "+
			"FROM cfg_achivements a,cfg_achivement_groups b WHERE a.`group`=b.id AND a.state=1 GROUP BY a.`group`")
	if err != nil {
		return nil, err
	}
	finishRows, err := s.db.FetchRows(ctx,
		"SELECT b.`group`,COUNT(0) AS finish_count FROM user_achivements a,cfg_achivements b "+
			"WHERE b.state=1 AND a.achivement_id=b.id AND a.uid=? GROUP BY b.`group`", uid)
	if err != nil {
		return nil, err
	}
	finishMap := map[int64]int64{}
	for _, r := range finishRows {
		finishMap[model.Int64(r, "group")] = model.Int64(r, "finish_count")
	}
	for _, item := range totals {
		fc := finishMap[model.Int64(item, "group")]
		if fc == 0 {
			fc = 0
		}
		item["finish_count"] = fc
	}
	return []any{point, recent, totals}, nil
}

// openClause 构造 open_sql（原版 mem_state 缺省 0 → 只含 {0} 或 {0,1}）。
func (s *Service) openClause(ctx context.Context, uid int) (string, error) {
	inNewbie, err := s.db.Exists(ctx, "select state from users where id=? and state in (0,2)", uid)
	if err != nil {
		return "", err
	}
	if inNewbie {
		return " and b.open_time in (0,1)", nil
	}
	return " and b.open_time in (0)", nil
}

// GetAchivementsByGroup 对应 getAchivementsByGroup：返回 [count, rows]。
// type1=已完成(子组过滤) type2=未完成 type0=全部。
func (s *Service) GetAchivementsByGroup(ctx context.Context, uid, group, subgroup, typ, page int) ([]any, error) {
	start := page * achivePageCPP
	open, err := s.openClause(ctx, uid)
	if err != nil {
		return nil, err
	}
	limit := " LIMIT " + strconv.Itoa(start) + ", " + strconv.Itoa(achivePageCPP)

	switch typ {
	case 1:
		cnt, err := s.db.FetchCellInt64(ctx,
			"SELECT count(*) FROM user_achivements a,cfg_achivements b WHERE b.state=1 AND a.achivement_id=b.id AND a.uid=? AND b.`group`=? AND b.sub_group=?"+open,
			uid, group, subgroup)
		if err != nil {
			return nil, err
		}
		rows, err := s.db.FetchRows(ctx,
			"SELECT b.id,b.name,b.content,b.point,b.image,a.time as achieveGetTime FROM user_achivements a,cfg_achivements b "+
				"WHERE b.state=1 AND a.achivement_id=b.id AND a.uid=? AND b.`group`=? AND b.sub_group=?"+open+" ORDER BY a.time DESC"+limit,
			uid, group, subgroup)
		if err != nil {
			return nil, err
		}
		return []any{cnt, rows}, nil
	case 2:
		cnt, err := s.db.FetchCellInt64(ctx,
			"SELECT count(*) FROM cfg_achivements b LEFT JOIN user_achivements a ON a.achivement_id=b.id AND a.uid=? "+
				"WHERE b.state=1 AND b.`group`=? AND a.uid IS null"+open, uid, group)
		if err != nil {
			return nil, err
		}
		rows, err := s.db.FetchRows(ctx,
			"SELECT b.id,b.name,b.content,b.point,b.image,'--' as achieveGetTime FROM cfg_achivements b "+
				"LEFT JOIN user_achivements a ON a.achivement_id=b.id AND a.uid=? "+
				"WHERE b.state=1 AND b.`group`=? AND a.uid IS null"+open+" ORDER BY b.id"+limit, uid, group)
		if err != nil {
			return nil, err
		}
		return []any{cnt, rows}, nil
	case 0:
		cnt, err := s.db.FetchCellInt64(ctx,
			"SELECT count(*) FROM cfg_achivements b WHERE b.state=1 AND b.`group`=?"+open, group)
		if err != nil {
			return nil, err
		}
		rows, err := s.db.FetchRows(ctx,
			"SELECT b.id,b.name,b.content,b.point,b.image,CASE WHEN a.time IS NULL THEN '--' ELSE a.time END AS achieveGetTime "+
				"FROM cfg_achivements b LEFT JOIN user_achivements a ON a.achivement_id=b.id AND a.uid=? "+
				"WHERE b.state=1 AND b.`group`=?"+open+" ORDER BY a.time DESC,b.id ASC"+limit, uid, group)
		if err != nil {
			return nil, err
		}
		return []any{cnt, rows}, nil
	}
	return []any{}, nil
}

// GetAchivementDetail 对应 getAchivementDetail：返回 [cfg_achivement, progress, 完成人数]。
func (s *Service) GetAchivementDetail(ctx context.Context, uid, aid int) ([]any, error) {
	info, err := s.db.FetchOne(ctx,
		"SELECT id,name,content,todo,image,type,sql_current_value,target_value FROM cfg_achivements WHERE id=?", aid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ret := []any{info}
	progress := []map[string]any{}

	got, err := s.db.Exists(ctx, "select 1 from user_achivements where uid=? and achivement_id=?", uid, aid)
	if err != nil {
		return nil, err
	}
	typ := model.Int(info, "type")
	if got {
		switch typ {
		case 2:
			progress = append(progress, map[string]any{
				"targetValue": model.Int64(info, "target_value"),
				"userValue":   model.Int64(info, "target_value"),
			})
		case 3:
			rows, err := s.db.FetchRows(ctx,
				"SELECT a.content,1 as isDone FROM cfg_achivement_goals a,cfg_achivement_goal_mappings b "+
					"WHERE a.id=b.achivement_goal_id AND b.achivement_id=?", aid)
			if err != nil {
				return nil, err
			}
			progress = rows
		}
	} else {
		switch typ {
		case 2:
			sqlText := formatUIDSQL(model.Str(info, "sql_current_value"), uid)
			userValue := int64(0)
			if sqlText != "" {
				userValue, err = s.cellInt(ctx, sqlText)
				if err != nil {
					return nil, err
				}
			}
			progress = append(progress, map[string]any{
				"targetValue": model.Int64(info, "target_value"),
				"userValue":   userValue,
			})
		case 3:
			goals, err := s.db.FetchRows(ctx,
				"SELECT a.content,a.sql_check_goal FROM cfg_achivement_goals a,cfg_achivement_goal_mappings b "+
					"WHERE a.id=b.achivement_goal_id AND b.achivement_id=?", aid)
			if err != nil {
				return nil, err
			}
			for _, goal := range goals {
				sqlText := formatUIDSQL(model.Str(goal, "sql_check_goal"), uid)
				isDone := false
				if sqlText != "" {
					isDone, err = s.db.Exists(ctx, sqlText)
					if err != nil {
						return nil, err
					}
				}
				progress = append(progress, map[string]any{
					"content": model.Str(goal, "content"),
					"isDone":  isDone,
				})
			}
		}
	}
	ret = append(ret, progress)
	cnt, err := s.cellInt(ctx, "SELECT COUNT(0) FROM user_achivements WHERE achivement_id=?", aid)
	if err != nil {
		return nil, err
	}
	ret = append(ret, cnt)
	info["sql_current_value"] = ""
	return ret, nil
}

func (s *Service) cellInt(ctx context.Context, q string, args ...any) (int64, error) {
	v, err := s.db.FetchCellInt64(ctx, q, args...)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return v, err
}

// formatUIDSQL 把配置 SQL 模板中的 %d/%s 占位替换为 uid（AchivementFunc.php sprintf 语义）。
func formatUIDSQL(tmpl string, uid int) string {
	if tmpl == "" {
		return ""
	}
	s := strconv.Itoa(uid)
	s = strings.ReplaceAll(tmpl, "%d", s)
	return strings.ReplaceAll(s, "%s", strconv.Itoa(uid))
}
