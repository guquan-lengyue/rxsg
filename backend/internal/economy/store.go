package economy

// store.go 复刻 legacy utils.php 的元宝/礼金/道具底层（与 goods/armor 包同名实现相互独立，
// 口径逐字对齐，避免跨包耦合）：
//   addMoney(1119) / addGift(1135) / checkMoney(1106) / checkGift(1113) /
//   addGoods(971) / reduceGoods(938) / checkGoods(933) / checkGoodsCount(1159) /
//   addThings(1030) / addArmor(1067) / logUserAction(1808) / sendReport(134)+sendReportDetail(182) /
//   addCityResources(193) / getBufferNobility(1534)。
// 降级：log_day_money/log_money_type 无 → 每日消耗统计省略；finishAchivement/updateBattleOpenState/
// sendDesignation（M7/M8）省略；log_qiyue 无 → addGoods(160052) 按普通道具处理。

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
)

// addMoney 对齐 utils.php:1119（money==0 返回；log_money 流水 + users.money 累加，带符号）。
func (s *Service) addMoney(ctx context.Context, uid int, money int64, typ int) error {
	if money == 0 {
		return nil
	}
	if _, err := s.db.Exec(ctx,
		"insert into log_money (user_id, `count`, `time`, `type`) values (?,?,unix_timestamp(),?)",
		uid, money, typ); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, "update users set money=money+? where id=?", money, uid)
	return err
}

// addGift 对齐 utils.php:1135（gift==0 返回；log_gifts 流水 + users.gift 累加，带符号）。
func (s *Service) addGift(ctx context.Context, uid int, gift int64, typ int) error {
	if gift == 0 {
		return nil
	}
	if _, err := s.db.Exec(ctx,
		"insert into log_gifts (user_id, gid, `count`, `time`, `type`) values (?,0,?,unix_timestamp(),?)",
		uid, gift, typ); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, "update users set gift=gift+? where id=?", gift, uid)
	return err
}

// checkMoney 对齐 utils.php:1106：money 不足（含无行/0）返回 false。
func (s *Service) checkMoney(ctx context.Context, uid int, need int64) (bool, error) {
	v, err := s.cellInt(ctx, "select money from users where id=? limit 1", uid)
	if err != nil {
		return false, err
	}
	if v == 0 { // legacy empty($usermoney)
		return false, nil
	}
	return v >= need, nil
}

// checkGift 对齐 utils.php:1113。
func (s *Service) checkGift(ctx context.Context, uid int, need int64) (bool, error) {
	v, err := s.cellInt(ctx, "select gift from users where id=? limit 1", uid)
	if err != nil {
		return false, err
	}
	if v == 0 {
		return false, nil
	}
	return v >= need, nil
}

// goodsCount 读取 user_goods.count；无行返回 0（对齐 legacy empty→0）。
func (s *Service) goodsCount(ctx context.Context, uid, gid int) (int64, error) {
	return s.cellInt(ctx, "select `count` from user_goods where user_id=? and gid=?", uid, gid)
}

// addGoods 对齐 utils.php:971。原版怪癖 1:1 保留：
//   - gid==0：不入背包，直接 users.gift+=cnt 并写 log_gifts（buyWorkShopGood 以此扣礼金）。
//   - gid==152（铜钱）：入账后把 cnt 改写为入账后的铜钱总数、gid 改写为 888888 再记一份
//     ——原版双记账 bug。
//   - gid==160052：log_qiyue 表缺失 → 按普通道具处理（降级）。
func (s *Service) addGoods(ctx context.Context, uid, gid int, cnt int64, typ int) error {
	if cnt == 0 {
		return nil
	}
	if gid == 0 {
		if _, err := s.db.Exec(ctx, "update users set gift=gift+? where id=?", cnt, uid); err != nil {
			return err
		}
		_, err := s.db.Exec(ctx,
			"insert into log_gifts (user_id, gid, `count`, `time`, `type`) values (?,0,?,unix_timestamp(),?)",
			uid, cnt, typ)
		return err
	}
	if _, err := s.db.Exec(ctx,
		"insert into user_goods (user_id, gid, `count`) values (?,?,?) on duplicate key update `count`=`count`+?",
		uid, gid, cnt, cnt); err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx,
		"insert into log_goods (user_id, gid, `count`, `time`, `type`) values (?,?,?,unix_timestamp(),?)",
		uid, gid, cnt, typ); err != nil {
		return err
	}
	if gid == 152 {
		total, err := s.goodsCount(ctx, uid, 152)
		if err != nil {
			return err
		}
		if _, err := s.db.Exec(ctx,
			"insert into user_goods (user_id, gid, `count`) values (?,?,?) on duplicate key update `count`=`count`+?",
			uid, 888888, total, total); err != nil {
			return err
		}
		_, err = s.db.Exec(ctx,
			"insert into log_goods (user_id, gid, `count`, `time`, `type`) values (?,?,?,unix_timestamp(),?)",
			uid, 888888, total, typ)
		return err
	}
	return nil
}

// reduceGoods 对齐 utils.php:938（finishAchivement 属 M8 省略；unlockUser 由上层锁覆盖）。
func (s *Service) reduceGoods(ctx context.Context, uid, gid int, cnt int64, typ int) error {
	if cnt <= 0 {
		return nil
	}
	if _, err := s.db.Exec(ctx,
		"insert into log_goods (user_id, gid, `count`, `time`, `type`) values (?,?,?,unix_timestamp(),?)",
		uid, gid, -cnt, typ); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx,
		"update user_goods set `count`=GREATEST(0,`count`-?) where user_id=? and gid=?", cnt, uid, gid)
	return err
}

// checkGoods 对齐 utils.php:933（>=1）。
func (s *Service) checkGoods(ctx context.Context, uid, gid int) (bool, error) {
	return s.checkGoodsCount(ctx, uid, gid, 1)
}

// checkGoodsCount 对齐 utils.php:1159（legacy 内部 lockUser 由上层 WithUserLock 统一持有）。
func (s *Service) checkGoodsCount(ctx context.Context, uid, gid int, need int64) (bool, error) {
	cnt, err := s.goodsCount(ctx, uid, gid)
	if err != nil {
		return false, err
	}
	return cnt >= need, nil
}

// addThings 对齐 utils.php:1030（cnt==0 返回；log_things 流水 + things upsert 带符号）。
func (s *Service) addThings(ctx context.Context, uid, tid int, cnt int64, typ int) error {
	if cnt == 0 {
		return nil
	}
	if _, err := s.db.Exec(ctx,
		"insert into log_things (user_id, tid, `count`, `time`, `type`) values (?,?,?,unix_timestamp(),?)",
		uid, tid, cnt, typ); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx,
		"insert into things (user_id, tid, `count`) values (?,?,?) on duplicate key update `count`=`count`+?",
		uid, tid, cnt, cnt)
	return err
}

// addArmor 对齐 utils.php:1067（逐件插入 user_armors；hp=ori_hp_max*10；
// updateBattleOpenState/sendDesignation 属 M7/M8 → 省略）。
func (s *Service) addArmor(ctx context.Context, uid int, armor map[string]any, cnt int64, typ int) error {
	if cnt == 0 {
		return nil
	}
	aid := modelInt(armor, "id")
	oriHP := modelInt(armor, "ori_hp_max")
	strongvalue, err := s.cellInt(ctx, "select strong_value from cfg_strong_probability where level=0 limit 1")
	if err != nil {
		return err
	}
	for i := int64(0); i < cnt; i++ {
		if _, err := s.db.Exec(ctx,
			"insert into user_armors (user_id, armorid, hp, hp_max, ori_hp_max, hid, strong_level, strong_value, combine_level) "+
				"values (?,?,?,?,?,0,0,?,0)",
			uid, aid, oriHP*10, oriHP, oriHP, strongvalue); err != nil {
			return err
		}
	}
	_, err = s.db.Exec(ctx,
		"insert into log_armor (user_id, armorid, `count`, `time`, `type`) values (?,?,?,unix_timestamp(),?)",
		uid, aid, cnt, typ)
	return err
}

// logUserAction 对齐 utils.php:1808。
func (s *Service) logUserAction(ctx context.Context, uid, aid int) error {
	if _, err := s.db.Exec(ctx,
		"insert into log_user_actions (user_id, aid, `time`) values (?,?,unix_timestamp())", uid, aid); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx,
		"insert into log_action_counts (user_id, aid, `count`) values (?,?,1) on duplicate key update `count`=`count`+1",
		uid, aid)
	return err
}

// sendReportDetail 对齐 utils.php:182（title 分档 stype；alarms 置位）。
func (s *Service) sendReportDetail(ctx context.Context, touid int, origincid int, origincity string,
	happencid int, happencity, content string, title int) error {
	stype := 3
	if title <= 11 {
		stype = 0
	} else if title >= 12 && title <= 14 {
		stype = 1
	} else if title == 19 {
		stype = 2
	}
	if _, err := s.db.Exec(ctx,
		"insert into reports (user_id, origincid, origincity, happencid, happencity, title, `type`, `time`, `read`, battleid, content) "+
			"values (?,?,?,?,?,?,?,unix_timestamp(),0,0,?)",
		touid, origincid, origincity, happencid, happencity, title, stype, content); err != nil { // legacy read='0'
		return err
	}
	_, err := s.db.Exec(ctx,
		"insert into alarms (user_id, report) values (?,1) on duplicate key update report=1", touid)
	return err
}

// sendReport 对齐 utils.php:134（origincity/happencity 按城名解析；mem_world 野地名缺失→空）。
func (s *Service) sendReport(ctx context.Context, touid, origincid, happencid, title int, content string) error {
	origincity := ""
	if origincid > 0 {
		v, err := s.db.FetchCellString(ctx, "select name from cities where id=?", origincid)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		origincity = v
	}
	happencity := ""
	if origincid == happencid {
		happencity = origincity
	} else if happencid > 0 {
		v, err := s.db.FetchCellString(ctx, "select name from cities where id=?", happencid)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		happencity = v
	}
	return s.sendReportDetail(ctx, touid, origincid, origincity, happencid, happencity, content, title)
}

// addCityResources 对齐 utils.php:193（参数顺序 wood,rock,iron,food,gold）。
func (s *Service) addCityResources(ctx context.Context, cid int, wood, rock, iron, food, gold int64) error {
	if cid <= 0 {
		return nil
	}
	_, err := s.db.Exec(ctx, `update city_resources set
		wood=wood+?, rock=rock+?, iron=iron+?, food=food+?, gold=gold+? where city_id=?`,
		wood, rock, iron, food, gold, cid)
	return err
}

// getBufNobility 对齐 utils.php:1534 getBufferNobility：
// user_buffers buftype∈{16,18} 按 bufparam 降序取 1 条；bufparam=5 封顶 19、=2 封顶 18。
func (s *Service) getBufNobility(ctx context.Context, uid int, realNobility float64) (int, error) {
	bufparam, err := s.db.FetchCellInt64(ctx,
		"select bufparam from user_buffers where user_id=? and (buftype=16 or buftype=18) order by bufparam desc limit 1", uid)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
		bufparam = 0
	}
	if bufparam != 0 {
		n := int64(realNobility) + bufparam
		if bufparam == 5 && n > 19 {
			n = 19
		}
		if bufparam == 2 && n > 18 {
			n = 18
		}
		if float64(n) > realNobility {
			return int(n), nil
		}
	}
	return int(realNobility), nil
}

// modelInt 从 FetchOne 的 map 中取 int（[]byte/string 数值兼容）。
func modelInt(m map[string]any, k string) int {
	if m == nil {
		return 0
	}
	switch v := m[k].(type) {
	case int64:
		return int(v)
	case []byte:
		n, _ := strconv.Atoi(string(v))
		return n
	case string:
		n, _ := strconv.Atoi(v)
		return n
	}
	return 0
}

// modelInt64 从 FetchOne 的 map 中取 int64。
func modelInt64(m map[string]any, k string) int64 {
	if m == nil {
		return 0
	}
	switch v := m[k].(type) {
	case int64:
		return v
	case []byte:
		n, _ := strconv.ParseInt(string(v), 10, 64)
		return n
	case string:
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	case float64:
		return int64(v)
	}
	return 0
}

// modelFloat 从 FetchOne 的 map 中取 float64（对齐 legacy 数值列松散解析）。
func modelFloat(m map[string]any, k string) float64 {
	if m == nil {
		return 0
	}
	switch v := m[k].(type) {
	case int64:
		return float64(v)
	case float64:
		return v
	case []byte:
		f, _ := strconv.ParseFloat(string(v), 64)
		return f
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return f
	}
	return 0
}

// itoa 对齐 PHP 字符串拼接中的整数格式化。
func itoa(v int64) string { return strconv.FormatInt(v, 10) }
