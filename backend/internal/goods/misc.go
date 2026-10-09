package goods

// misc.go 复刻 legacy server/game/GoodsFunc.php 的"功能类道具"分支（1:1，原版 bug/怪癖保留）。
// 函数 ↔ PHP 行号映射见各函数头注释。文案逐字取自 server/game/lang.php。
// 通用降级（新库缺表/外部服务，按任务约定）：
//   - users 无 union_id → 14/15 盟主令/密诏 checkGoods 通过后走"未加入联盟"文案。
//   - users 无 prestige/gift/armor_column、sys_user_level/mem_state/sys_question/cfg_npc_hero/
//     sys_user_state/reinforcequeue/cfg_soldier_special_city/mem_world/cfg_province/sys_union/
//     sys_mail_* 等表无 → 各函数内注释说明。
//   - completeTaskWithTaskid/logUserAction/成就/sendSysInform/sendReport/session 文件属 M7/M8，未接线。

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"rxsg/backend/internal/model"
)

// makeEndTimeStr 对齐 utils.php:55 MakeEndTime（from_unixtime '%Y年%m月%d %H:%i:%s'）。
func makeEndTimeStr(ts int64) string {
	return time.Unix(ts, 0).Format("2006年01月02日 15:04:05")
}

// miscMtRand 对齐 PHP mt_rand(a,b)（含端点）。
func miscMtRand(a, b int64) int64 {
	return rand.Int63n(b-a+1) + a
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

// miscCityHasHeroPosition 对齐 utils.php:757 cityHasHeroPosition：
// 招贤馆（buildings building_id=11）level > 该城将领数；level 空/0 视为无位置。
func (s *Service) miscCityHasHeroPosition(ctx context.Context, uid, cid int) (bool, error) {
	officeLevel, err := s.db.FetchCellInt64(ctx,
		"select level from buildings where city_id=? and building_id=11 limit 1", cid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if officeLevel == 0 { // legacy empty($officeLevel) → false
		return false, nil
	}
	heroCount, err := s.db.FetchCellInt64(ctx,
		"select count(*) from heroes where city_id=? and user_id=?", cid, uid)
	if err != nil {
		return false, err
	}
	return officeLevel > heroCount, nil
}

// miscInsertHero 对齐 openHeroBox_* 的 sys_city_hero 固定属性插入（列按 0003 schema；
// mem_hero_blood/regenerateHeroAttri/updateCityHeroChange 缺表或属 M7，未接线）。
func (s *Service) miscInsertHero(ctx context.Context, uid, cid int, name string, sex, face,
	level, exp, affairs, bravery, wisdom, loyalty, command, heroType int) error {
	_, err := s.db.Exec(ctx,
		"insert into heroes (user_id,name,sex,face,city_id,state,level,exp,"+
			"affairs_base,bravery_base,wisdom_base,loyalty,command_base,hero_type) "+
			"values (?,?,?,?,?,0,?,?,?,?,?,?,?,?)",
		uid, name, sex, face, cid, level, exp, affairs, bravery, wisdom, loyalty, command, heroType)
	return err
}

// useGaojiTuiEnLing 对齐 GoodsFunc.php:2076。
// 返回 [0]=推恩后爵位、[1]=endtime-unix_timestamp()（剩余秒数，非绝对 endtime，与 PHP ret 顺序一致）。
// users.nobility 为 varchar，按 ParseFloat 数值化（失败按 0）。
func (s *Service) useGaojiTuiEnLing(ctx context.Context, uid, gid int) (int, int64, error) {
	_ = gid // legacy 形参未用，固定 gid=117
	ok, err := s.checkGoods(ctx, uid, 117)
	if err != nil {
		return 0, 0, err
	}
	if !ok {
		return 0, 0, errLegacy("你拥有的该道具数量为0，请去商城购买后再使用。")
	}
	nobility, err := s.cellFloat(ctx, "select nobility from users where id=?", uid)
	if err != nil {
		return 0, 0, err
	}
	if nobility < 5 {
		return 0, 0, errLegacy("大夫以上爵位才能使用高级推恩令。")
	}
	lefttime, err := s.db.FetchCellInt64(ctx,
		"select endtime-unix_timestamp() from user_buffers where user_id=? and buftype=16", uid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, 0, err
	}
	if lefttime != 0 { // 正在使用则时间延长（legacy !empty：lefttime=0/无行都走新插入分支）
		if _, err := s.db.Exec(ctx,
			"update user_buffers set endtime=endtime+864000 where user_id=? and buftype=16", uid); err != nil {
			return 0, 0, err
		}
		if err := s.ReduceGoods(ctx, uid, 117, 1, 0); err != nil {
			return 0, 0, err
		}
		nb, err := s.getBufNobility(ctx, uid, nobility)
		if err != nil {
			return 0, 0, err
		}
		left, err := s.db.FetchCellInt64(ctx,
			"select endtime-unix_timestamp() from user_buffers where user_id=? and buftype=16", uid)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return 0, 0, err
		}
		return nb, left, nil
	}
	if nobility >= 19 {
		return 0, 0, errLegacy("您的爵位已经达到或超过“关内侯”，无须再使用高级推恩令。")
	}
	if _, err := s.db.Exec(ctx,
		"insert into user_buffers (user_id,buftype,bufparam,endtime) values (?,16,5,unix_timestamp()+864000) "+
			"on duplicate key update endtime=endtime+864000", uid); err != nil {
		return 0, 0, err
	}
	if err := s.ReduceGoods(ctx, uid, 117, 1, 0); err != nil {
		return 0, 0, err
	}
	nb, err := s.getBufNobility(ctx, uid, nobility)
	if err != nil {
		return 0, 0, err
	}
	left, err := s.db.FetchCellInt64(ctx,
		"select endtime-unix_timestamp() from user_buffers where user_id=? and buftype=16", uid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, 0, err
	}
	return nb, left, nil
}

// useTuiEnLing 对齐 GoodsFunc.php:2110。返回 [0]=爵位、[1]=endtime-unix_timestamp()。
// completeTaskWithTaskid(306) 属 M8，未接线。
func (s *Service) useTuiEnLing(ctx context.Context, uid, gid int) (int, int64, error) {
	_ = gid // legacy 形参未用，固定 gid=124
	ok, err := s.checkGoods(ctx, uid, 124)
	if err != nil {
		return 0, 0, err
	}
	if !ok {
		return 0, 0, errLegacy("你拥有的该道具数量为0，请去商城购买后再使用。")
	}
	nobility, err := s.cellFloat(ctx, "select nobility from users where id=?", uid)
	if err != nil {
		return 0, 0, err
	}
	lefttime, err := s.db.FetchCellInt64(ctx,
		"select endtime-unix_timestamp() from user_buffers where user_id=? and buftype=18", uid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, 0, err
	}
	if lefttime != 0 { // 正在使用则时间延长
		if _, err := s.db.Exec(ctx,
			"update user_buffers set endtime=endtime+86400*3 where user_id=? and buftype=18", uid); err != nil {
			return 0, 0, err
		}
		if err := s.ReduceGoods(ctx, uid, 124, 1, 0); err != nil {
			return 0, 0, err
		}
		nb, err := s.getBufNobility(ctx, uid, nobility)
		if err != nil {
			return 0, 0, err
		}
		left, err := s.db.FetchCellInt64(ctx,
			"select endtime-unix_timestamp() from user_buffers where user_id=? and buftype=18", uid)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return 0, 0, err
		}
		return nb, left, nil
	}
	if nobility >= 18 {
		return 0, 0, errLegacy("您的爵位已经达到或超过“大庶长”，无须再使用推恩令。")
	}
	if _, err := s.db.Exec(ctx,
		"insert into user_buffers (user_id,buftype,bufparam,endtime) values (?,18,2,unix_timestamp()+86400*3) "+
			"on duplicate key update endtime=endtime+86400*3", uid); err != nil {
		return 0, 0, err
	}
	if err := s.ReduceGoods(ctx, uid, 124, 1, 0); err != nil {
		return 0, 0, err
	}
	nb, err := s.getBufNobility(ctx, uid, nobility)
	if err != nil {
		return 0, 0, err
	}
	left, err := s.db.FetchCellInt64(ctx,
		"select endtime-unix_timestamp() from user_buffers where user_id=? and buftype=18", uid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, 0, err
	}
	return nb, left, nil
}

// useSheMianWenShu 对齐 GoodsFunc.php:2143。
// 怪癖保留：update 影响行数为 0 时不报错，而是 return shemian_fail 文案。
func (s *Service) useSheMianWenShu(ctx context.Context, uid, gid int) (string, error) {
	_ = gid // legacy 形参未用，固定 gid=134
	ok, err := s.checkGoods(ctx, uid, 134)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errLegacy("你拥有的该道具数量为0，请去商城购买后再使用。")
	}
	honour, err := s.db.FetchCellInt64(ctx, "select honour from users where id=?", uid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if honour >= 0 { // 无行→0→同样报"不需要赦免"（legacy false>=0 亦为真）
		return "", errLegacy("你没有小于0的战场荣誉，不需要使用赦免文书。")
	}
	affected, err := s.db.Exec(ctx, "update users set honour=0 where honour<0 and id=?", uid)
	if err != nil {
		return "", err
	}
	if affected > 0 {
		if err := s.ReduceGoods(ctx, uid, 134, 1, 0); err != nil {
			return "", err
		}
		return "赦免文书使用成功，你的战场荣誉已经变为0。", nil
	}
	return "赦免文书使用失败", nil
}

// useShiShiWenShu 对齐 GoodsFunc.php:2171。
// 降级：BattleNet 远程服务无 → getBattleNetScore/restoreBattleNetScore 不可用，
// restore 恒失败 → 不扣道具，直接返回 shishi_fail（对照 PHP:2184-2187 分支）。
func (s *Service) useShiShiWenShu(ctx context.Context, uid, gid int) (string, error) {
	_ = gid // legacy 形参未用，固定 gid=158
	ok, err := s.checkGoods(ctx, uid, 158)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errLegacy("你拥有的该道具数量为0，请去商城购买后再使用。")
	}
	return "誓师文书使用失败", nil
}

// useQingZhanShu 对齐 GoodsFunc.php:2191。
// 怪癖保留：today_war_count=0 的检查先于 checkGoods。
// 失败分支 legacy 是 throw（区别于赦免文书的 return）。
func (s *Service) useQingZhanShu(ctx context.Context, uid, gid int) (string, error) {
	_ = gid // legacy 形参未用，固定 gid=138
	twc, err := s.db.FetchCellInt64(ctx, "select today_war_count from user_schedule where user_id=?", uid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if twc == 0 {
		return "", errLegacy("当前剧情战场参战次数为0,不需要重置")
	}
	ok, err := s.checkGoods(ctx, uid, 138)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errLegacy("not_enough_goods138")
	}
	affected, err := s.db.Exec(ctx, "update user_schedule set today_war_count=0 where user_id=?", uid)
	if err != nil {
		return "", err
	}
	if affected > 0 {
		if err := s.ReduceGoods(ctx, uid, 138, 1, 0); err != nil {
			return "", err
		}
		return "请战书使用成功，你的剧情战场参战次数已经变为0。", nil
	}
	return "", errLegacy("请战书使用失败")
}

// useDianMinLin 对齐 GoodsFunc.php:2314。
// 缺道具文案按 PHP 为字面量 "not_enough_goods57"（lang addPeople.no_goods 未被该分支引用，1:1 保留）。
// updateCityResourceAdd/completeTaskWithTaskid(308) 属 city/M8，未接线。
func (s *Service) useDianMinLin(ctx context.Context, uid, cid int) (string, error) {
	ok, err := s.checkGoods(ctx, uid, 57)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errLegacy("not_enough_goods57")
	}
	owner, err := s.db.FetchCellInt64(ctx, "select user_id from cities where id=?", cid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if owner == 0 || owner != int64(uid) {
		return "", errLegacy("城池不属于你")
	}
	row, err := s.db.FetchOne(ctx, "select people,people_max from city_resources where city_id=?", cid)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
		// legacy 无行：people/people_max 均 null，null>=null 为真 → city_full
		return "", errLegacy("当前人口已经超过城池人口上限，不能招徕更多流民了。")
	}
	peopleMax := model.Int64(row, "people_max")
	people := model.Int64(row, "people")
	if people >= peopleMax {
		return "", errLegacy("当前人口已经超过城池人口上限，不能招徕更多流民了。")
	}
	add := int64(math.Ceil(float64(peopleMax) * 0.2))
	if add < 100 {
		add = 100
	}
	if _, err := s.db.Exec(ctx, "update city_resources set people=people+? where city_id=?", add, cid); err != nil {
		return "", err
	}
	if err := s.ReduceGoods(ctx, uid, 57, 1, 0); err != nil {
		return "", err
	}
	return fmt.Sprintf("成功招徕百姓%d人", add), nil
}

// useTaiPingYaoShu 对齐 GoodsFunc.php:2335。add=ceil(people_max*0.1)，下限 100。
func (s *Service) useTaiPingYaoShu(ctx context.Context, uid, cid int) (string, error) {
	ok, err := s.checkGoods(ctx, uid, 139)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errLegacy("你没有道具“太平要术”，不能招徕流民。")
	}
	owner, err := s.db.FetchCellInt64(ctx, "select user_id from cities where id=?", cid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if owner == 0 || owner != int64(uid) {
		return "", errLegacy("城池不属于你")
	}
	row, err := s.db.FetchOne(ctx, "select people,people_max from city_resources where city_id=?", cid)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
		return "", errLegacy("当前人口已经超过城池人口上限，不能招徕更多流民了。")
	}
	peopleMax := model.Int64(row, "people_max")
	people := model.Int64(row, "people")
	if people >= peopleMax {
		return "", errLegacy("当前人口已经超过城池人口上限，不能招徕更多流民了。")
	}
	add := int64(math.Ceil(float64(peopleMax) * 0.1))
	if add < 100 {
		add = 100
	}
	if _, err := s.db.Exec(ctx, "update city_resources set people=people+? where city_id=?", add, cid); err != nil {
		return "", err
	}
	if err := s.ReduceGoods(ctx, uid, 139, 1, 0); err != nil {
		return "", err
	}
	return fmt.Sprintf("成功招徕百姓%d人", add), nil
}

// useAnMingGaoShi 对齐 GoodsFunc.php:2536。city_schedule 无行视为未使用过（legacy empty 语义）。
// completeTaskWithTaskid(309) 属 M8，未接线。
func (s *Service) useAnMingGaoShi(ctx context.Context, uid, cid int) (string, error) {
	ok, err := s.checkGoods(ctx, uid, 58)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errLegacy("not_enough_goods58")
	}
	info, err := s.db.FetchOne(ctx,
		"select last_anming, unix_timestamp() as nowtime from city_schedule where city_id=?", cid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	lasttime := model.Int64(info, "last_anming")
	nowtime := model.Int64(info, "nowtime")
	if lasttime != 0 && nowtime-lasttime < 259200 {
		return "", errLegacy(fmt.Sprintf("“安民告示”72小时内只能使用一次，请在%s后再使用。",
			makeTimeLeft(259200-(nowtime-lasttime))))
	}
	if _, err := s.db.Exec(ctx,
		"update city_resources set morale=100, complaint=0, morale_stable=100-tax, people_stable=people_max where city_id=?",
		cid); err != nil {
		return "", err
	}
	if _, err := s.db.Exec(ctx,
		"insert into city_schedule (city_id,last_anming) values (?,unix_timestamp()) on duplicate key update last_anming=unix_timestamp()",
		cid); err != nil {
		return "", err
	}
	if err := s.ReduceGoods(ctx, uid, 58, 1, 0); err != nil {
		return "", err
	}
	return "“安民告示”使用成功，当前城池民心升至100，民怨降为0。", nil
}

// useKaoGongJi 对齐 GoodsFunc.php:2558。buftype=12+(gid-60)，delay 86400。
// 文案 sprintf(KaoGongJi_valid_date, cfg_goods.name)。
func (s *Service) useKaoGongJi(ctx context.Context, uid, gid int) (int64, string, error) {
	ok, err := s.checkGoods(ctx, uid, gid)
	if err != nil {
		return 0, "", err
	}
	if !ok {
		return 0, "", errLegacy(fmt.Sprintf("not_enough_goods%d", gid))
	}
	bufType := 12 + (gid - 60)
	endtime, err := s.addUserBuffer(ctx, uid, bufType, 86400, 86400)
	if err != nil {
		return 0, "", err
	}
	if err := s.ReduceGoods(ctx, uid, gid, 1, 0); err != nil {
		return 0, "", err
	}
	name, err := s.db.FetchCellString(ctx, "select name from cfg_goods where gid=?", gid)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, "", err
		}
		name = ""
	}
	return endtime, fmt.Sprintf("“%s”有效时间截止到", name), nil
}

// useXunChaLin 对齐 GoodsFunc.php:2377。buftype=100、+86400*3；
// legacy update 带 bufparam=0，addUserBuffer 不含 bufparam → 原生 SQL。
func (s *Service) useXunChaLin(ctx context.Context, uid, cid int) (int64, error) {
	_ = cid // legacy 形参未用
	ok, err := s.checkGoods(ctx, uid, 142)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, errLegacy("not_enough_goods142")
	}
	delay := int64(86400 * 3)
	if _, err := s.db.Exec(ctx,
		"insert into user_buffers (user_id,buftype,bufparam,endtime) values (?,100,0,unix_timestamp()+?) "+
			"on duplicate key update endtime=endtime+?, bufparam=0", uid, delay, delay); err != nil {
		return 0, err
	}
	if err := s.ReduceGoods(ctx, uid, 142, 1, 0); err != nil {
		return 0, err
	}
	return s.bufferEnd(ctx, uid, 100)
}

// useAdvancedConstructionPlan 对齐 GoodsFunc.php:2356。
// 名城判定（读 PHP 确认方向）：city.type==5 先归 0（type=5 本身是"资源地满级标记"），
// 其余 type!=0 才报"不能在名城使用高级建筑图纸。"。city_buffers buftype=10001、+12*3600。
func (s *Service) useAdvancedConstructionPlan(ctx context.Context, uid, cid, gid int) (int64, error) {
	ok, err := s.checkGoods(ctx, uid, gid)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, errLegacy(fmt.Sprintf("not_enough_goods%d", gid))
	}
	cityType, err := s.db.FetchCellInt64(ctx, "select type from cities where id=?", cid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if cityType == 5 {
		cityType = 0
	}
	if cityType != 0 {
		return 0, errLegacy("不能在名城使用高级建筑图纸。")
	}
	delay := int64(12 * 3600)
	if _, err := s.db.Exec(ctx,
		"insert into city_buffers (city_id,buftype,bufparam,endtime) values (?,10001,0,unix_timestamp()+?) "+
			"on duplicate key update endtime=endtime+?", cid, delay, delay); err != nil {
		return 0, err
	}
	endtime, err := s.db.FetchCellInt64(ctx, "select endtime from city_buffers where city_id=? and buftype=10001", cid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if err := s.ReduceGoods(ctx, uid, gid, 1, 0); err != nil {
		return 0, err
	}
	return endtime, nil
}

// userXianDiZhaoShu 对齐 GoodsFunc.php:1910。buftype=gid(161501)、delay 86400*3。
// 降级：sys_union/sys_mail_*/sys_alarm/sendSysInform 缺表或外部服务 → 联盟邮件与全服通告省略。
func (s *Service) userXianDiZhaoShu(ctx context.Context, uid, gid int) (int64, error) {
	goodsname, err := s.db.FetchCellString(ctx, "select name from cfg_goods where gid=?", gid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	ok, err := s.checkGoods(ctx, uid, gid)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, errLegacy(fmt.Sprintf("你没有%s，不能使用。", goodsname))
	}
	delay := int64(86400 * 3)
	endtime, err := s.addUserBuffer(ctx, uid, gid, delay, delay)
	if err != nil {
		return 0, err
	}
	if err := s.ReduceGoods(ctx, uid, gid, 1, 0); err != nil {
		return 0, err
	}
	return endtime, nil
}

// useZhaoAnLing 对齐 GoodsFunc.php:4802。
// 原版 bug 保留：insert 用 86400×useCount，on duplicate 固定 +86400；
// 扣道具 gid 为字面量 10333（非形参 $gid）。
func (s *Service) useZhaoAnLing(ctx context.Context, uid, gid, useCount int) (int64, error) {
	goodCnt, err := s.goodsCount(ctx, uid, gid) // 计数读 $gid，但 insert buftype/reduce 均固定 10333
	if err != nil {
		return 0, err
	}
	if goodCnt == 0 || goodCnt < int64(useCount) {
		return 0, errLegacy("当前物品不足")
	}
	addTime := int64(86400 * useCount)
	endtime, err := s.addUserBuffer(ctx, uid, 10333, addTime, 86400)
	if err != nil {
		return 0, err
	}
	if err := s.ReduceGoods(ctx, uid, 10333, int64(useCount), 0); err != nil {
		return 0, err
	}
	return endtime, nil
}

// useAddUserHeroExpBook 对齐 GoodsFunc.php:1548。
// 降级：sys_user_level 表无 → levelLimit 恒 100。
// 怪癖保留：realCnt=goodCnt（非 useCount）；useCount>needCnt 时 exp 直接置 maxExp
// 且文案仍报单份 expadd；checkGoods 不足文案为 duihuan.not_enogh"当前物品不足"（以 PHP 为准）。
func (s *Service) useAddUserHeroExpBook(ctx context.Context, uid, cid, gid, useCount int) (string, error) {
	_ = cid // legacy 按 uid+herotype=1000 查君主将，cid 未用
	goodCnt, err := s.goodsCount(ctx, uid, gid)
	if err != nil {
		return "", err
	}
	if goodCnt == 0 || goodCnt < int64(useCount) {
		return "", errLegacy("当前物品不足")
	}
	hero, err := s.db.FetchOne(ctx, "select * from heroes where user_id=? and hero_type=1000 limit 1", uid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errLegacy("您当前没有君主将")
		}
		return "", err
	}
	levelLimit := 100 // sys_user_level 表无，恒 100
	if model.Int(hero, "level") >= levelLimit {
		return "", errLegacy("将领等级达到上限，不需要再增加经验。")
	}
	state := model.Int(hero, "state")
	if !(state == 0 || state == 1 || state == 7 || state == 8) { // isHeroInCity HeroFunc.php:1949
		return "", errLegacy("将领正在出征或者没有效忠于你。只能给在本城内效忠于你的将领使用。")
	}
	heroID := model.Int(hero, "id")
	heroCurExp := model.Int64(hero, "exp")
	maxExp, err := s.db.FetchCellInt64(ctx, "select total_exp from cfg_hero_levels where level=?", levelLimit)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
		maxExp = 0
	}
	leaveExp := maxExp - heroCurExp

	expadd := int64(0)
	switch gid {
	case 10957:
		expadd = 5000
	case 10958:
		expadd = 15000
	case 10959:
		expadd = 30000
	}
	needCnt := int64(math.Ceil(float64(leaveExp) / float64(expadd)))
	realCnt := goodCnt
	msgExpadd := expadd
	if int64(useCount) > needCnt {
		realCnt = needCnt
		if _, err := s.db.Exec(ctx, "update heroes set exp=? where id=?", maxExp, heroID); err != nil {
			return "", err
		}
	} else {
		msgExpadd = expadd * realCnt
		if _, err := s.db.Exec(ctx, "update heroes set exp=exp+? where id=?", msgExpadd, heroID); err != nil {
			return "", err
		}
	}
	if err := s.ReduceGoods(ctx, uid, gid, realCnt, 0); err != nil {
		return "", err
	}
	return fmt.Sprintf("使用成功，君主将经验增加%d", msgExpadd), nil
}

// useMenZhuMiZhao 对齐 GoodsFunc.php:1601。
// 降级：users 无 union_id/sys_union/sys_online 表 → checkGoods 通过后走"未加入联盟"文案
// （lang useMenZhuMiZhao.not_join_union，任务指定）。
func (s *Service) useMenZhuMiZhao(ctx context.Context, uid int) error {
	ok, err := s.checkGoods(ctx, uid, 14)
	if err != nil {
		return err
	}
	if !ok {
		return errLegacy("not_enough_goods14")
	}
	return errLegacy("你还没有加入联盟，不能使用“盟主密诏”")
}

// useMenzhulin 对齐 GoodsFunc.php:1629。
// 降级：users 无 union_id → checkGoods 通过后走"未加入联盟"（lang useMenzhulin.not_join_union）。
func (s *Service) useMenzhulin(ctx context.Context, uid, cid int) error {
	_ = cid // legacy 形参未用
	ok, err := s.checkGoods(ctx, uid, 15)
	if err != nil {
		return err
	}
	if !ok {
		return errLegacy("not_enough_goods15")
	}
	return errLegacy("你还没有加入联盟，不能使用盟主令")
}

// useaddyuanbao 对齐 GoodsFunc.php:1649。gid=-100：money+=goodCnt、reduceGoods、返回数额。
// empty($goodCnt)：0 与无行同样报"当前物品不足"。
func (s *Service) useaddyuanbao(ctx context.Context, uid int) (int64, error) {
	goodCnt, err := s.db.FetchCellInt64(ctx, "select `count` from user_goods where user_id=? and gid=-100", uid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if goodCnt == 0 {
		return 0, errLegacy("当前物品不足")
	}
	if _, err := s.db.Exec(ctx, "update users set money=money+? where id=?", goodCnt, uid); err != nil {
		return 0, err
	}
	if err := s.ReduceGoods(ctx, uid, -100, goodCnt, 0); err != nil {
		return 0, err
	}
	return goodCnt, nil
}

// useWuHun 对齐 GoodsFunc.php:2398。
// 降级：cfg_npc_hero/mem_hero_buffer/sys_city_hero_base_add 表无 →
// 仅 151027/151028/151029 晋级丹分支可实现（条件按 PHP：level<119/124/129 或 npcid<0 则拒绝，
// 即要求名将 npcid>=0；heroes 无 npc_id 列时 select * 取不到该键按 0 处理，npcid<0 恒不触发）；
// 其余 gid 走不可达的武魂加成本体 → errLegacy("此功能尚未开放。")（lang useGoods.func_not_in_use）。
// sendSysInform 全服通告属外部，未接线。
func (s *Service) useWuHun(ctx context.Context, uid, hid, gid int) (string, error) {
	ok, err := s.checkGoods(ctx, uid, gid)
	if err != nil {
		return "", err
	}
	if !ok {
		name, err := s.db.FetchCellString(ctx, "select name from cfg_goods where gid=?", gid)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
		return "", errLegacy(fmt.Sprintf("你没有道具“%s”。", name))
	}
	hero, err := s.db.FetchOne(ctx, "select * from heroes where user_id=? and id=? limit 1", uid, hid)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
		// legacy 无行：level/npcid 为 null，null<119 为真 → 走"等级不足"报错
	}
	if gid > 151026 && gid < 151030 {
		level := model.Int(hero, "level")
		npcid := model.Int(hero, "npc_id")
		var uplevel int
		switch gid {
		case 151027:
			if level < 119 || npcid < 0 {
				return "", errLegacy("只有名将将领达到120级才能使用初级晋级丹！")
			}
			uplevel = 125
		case 151028:
			if level < 124 || npcid < 0 {
				return "", errLegacy("只有名将将领达到125级才能使用中级晋级丹！")
			}
			uplevel = 130
		case 151029:
			if level < 129 || npcid < 0 {
				return "", errLegacy("只有名将将领达到130级才能使用高级晋级丹！")
			}
			uplevel = 135
		}
		if _, err := s.db.Exec(ctx, "update heroes set level=?,exp=695100000 where id=?", uplevel, hid); err != nil {
			return "", err
		}
		if err := s.ReduceGoods(ctx, uid, gid, 1, 0); err != nil {
			return "", err
		}
		return "恭喜你,该将领已经成功升级了！", nil
	}
	// 非晋级丹：mem_hero_buffer 表无 → 恒走 PHP else 分支（2430-2456）。
	// 可达检查：state!=0 → 仅空闲将领可用；随后 cfg_npc_hero 缺 → 属性计算不可达，降级。
	if model.Int(hero, "state") != 0 {
		return "", errLegacy("空闲状态的将领才能使用武魂。")
	}
	return "", errLegacy("此功能尚未开放。")
}

// miscXiuJia 对齐 GoodsFunc.php:3869 xiuJia($uid,$day)。
// 降级：reinforcequeue 表无 → 恒 0（无加速队列）；mem_world 表无 → 战乱检查与盟友驻军遣返省略；
// sys_user_state 表无 → vacstart/vacend 写入省略；
// 军队在外按 PHP 原样：troops 存在任意该用户行即拒绝（无 state 过滤，怪癖保留）。
func (s *Service) miscXiuJia(ctx context.Context, uid int, day int) error {
	out, err := s.db.Exists(ctx, "select 1 from troops where user_id=? limit 1", uid)
	if err != nil {
		return err
	}
	if out {
		return errLegacy("你有军队在外，不能休假")
	}
	upgrading, err := s.db.Exists(ctx, "select 1 from technics where user_id=? and state=1 limit 1", uid)
	if err != nil {
		return err
	}
	if upgrading {
		return errLegacy("你有科技在升级，不能休假")
	}
	cities, err := s.db.FetchRows(ctx, "select id from cities where user_id=?", uid)
	if err != nil {
		return err
	}
	ids := make([]any, 0, len(cities))
	for _, c := range cities {
		ids = append(ids, model.Int64(c, "id"))
	}
	if len(ids) > 0 { // legacy 无城时 in() 为空串（SQL 报错）；Go 按无任务放行
		ph := "(" + strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",") + ")"
		bBusy, err := s.db.Exists(ctx,
			"select 1 from buildings where city_id in "+ph+" and state<>0 limit 1", ids...)
		if err != nil {
			return err
		}
		if bBusy {
			return errLegacy("你有建筑在升级，不能休假")
		}
		dBusy, err := s.db.Exists(ctx, "select 1 from draft_queue where city_id in "+ph+" limit 1", ids...)
		if err != nil {
			return err
		}
		if dBusy {
			return errLegacy("你有兵营正在招募军队，不能休假")
		}
		// 城防制造队列（sys_city_reinforcequeue）表无 → 恒 0
		// 城池战乱（mem_world）表无 → 省略
	}
	// 冷却检查（先于休假开始）
	cooling, err := s.db.FetchOne(ctx,
		"select bufparam, endtime-unix_timestamp() as lefttime from user_buffers where user_id=? and buftype=23 limit 1", uid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if cooling != nil { // 正处于休假冷却时期
		return errLegacy(fmt.Sprintf("休假结束%d小时后才可以再次使用，还需%s！",
			model.Int(cooling, "bufparam"), makeTimeLeft(model.Int64(cooling, "lefttime"))))
	}
	// sys_user_state.vacend 未处理检查：表无 → 省略
	// 盟友驻军遣返 update：依赖 legacy troops 列（targetcid/pathtime 等）与 mem_world，属 M7 → 省略
	vactime := int64(day) * 86400
	if len(ids) > 0 {
		ph := "(" + strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",") + ")"
		if _, err := s.db.Exec(ctx, "update city_resources set vacation=1 where city_id in "+ph, ids...); err != nil {
			return err
		}
	}
	coolingTime := int64(math.Floor((float64(vactime) * 0.2) / 8))
	totalDuringTime := vactime + coolingTime
	// 913 休假冷却 buffer：on duplicate 为绝对 now+total（非叠加），与 addUserBuffer 不同 → 原生 SQL
	if _, err := s.db.Exec(ctx,
		"insert into user_buffers (user_id,buftype,endtime) values (?,913,unix_timestamp()+?) "+
			"on duplicate key update endtime=unix_timestamp()+?", uid, totalDuringTime, totalDuringTime); err != nil {
		return err
	}
	return nil
}

// useXiuJiaFu 对齐 GoodsFunc.php:3956。121→3 天、122→10 天。
// sessions 文件写入属 legacy 会话层 → 省略。
func (s *Service) useXiuJiaFu(ctx context.Context, uid, cid, gid int) (string, error) {
	_ = cid // legacy 形参未用
	ok, err := s.checkGoods(ctx, uid, gid)
	if err != nil {
		return "", err
	}
	if !ok {
		name, err := s.db.FetchCellString(ctx, "select name from cfg_goods where gid=?", gid)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
		return "", errLegacy(fmt.Sprintf("你没有%s，不能使用。", name))
	}
	day := 3
	if gid == 122 {
		day = 10
	}
	if err := s.miscXiuJia(ctx, uid, day); err != nil {
		return "", err
	}
	if err := s.ReduceGoods(ctx, uid, gid, 1, 0); err != nil {
		return "", err
	}
	now, err := s.db.Now(ctx)
	if err != nil {
		return "", err
	}
	endtime := now + int64(day)*86400
	return fmt.Sprintf("休假开始，你会自动掉线。休假将于 %s 结束！", makeEndTimeStr(endtime)), nil
}

// openHeroBoxTaskreward 对齐 GoodsFunc.php:3488（gid156 自荐状）。
// 降级：随机名依赖 mem_cfg_firstname/girlname/boyname 表无 → 按 PHP 空结果路径（generateName 返回 ”）
// 取 name=""；mem_hero_blood/regenerateHeroAttri/updateCityHeroChange 属缺表/M7 → 省略。
func (s *Service) openHeroBoxTaskreward(ctx context.Context, uid int) (string, error) {
	cid, err := s.lastCityID(ctx, uid)
	if err != nil {
		return "", err
	}
	ok, err := s.checkGoods(ctx, uid, 156)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errLegacy("你没有这个道具。")
	}
	has, err := s.miscCityHasHeroPosition(ctx, uid, cid)
	if err != nil {
		return "", err
	}
	if !has {
		return "", errLegacy("你的招贤馆内没有位置了。")
	}
	sex := int(miscMtRand(0, 1))
	name := "" // 缺表 → PHP 空结果路径
	face := 1001 + int(miscMtRand(0, 69))
	if sex == 0 {
		face = 1 + int(miscMtRand(0, 8))
	}
	if err := s.ReduceGoods(ctx, uid, 156, 1, 0); err != nil {
		return "", err
	}
	if err := s.miscInsertHero(ctx, uid, cid, name, sex, face, 1, 1, 70, 70, 70, 70, 0, 0); err != nil {
		return "", err
	}
	return "你得到了一名将领。", nil
}

// openHeroBoxActreward 对齐 GoodsFunc.php:3511（gid10215 热血勇士召唤令，名字为 PHP 硬编码）。
func (s *Service) openHeroBoxActreward(ctx context.Context, uid int) (string, error) {
	cid, err := s.lastCityID(ctx, uid)
	if err != nil {
		return "", err
	}
	ok, err := s.checkGoods(ctx, uid, 10215)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errLegacy("你没有这个道具。")
	}
	has, err := s.miscCityHasHeroPosition(ctx, uid, cid)
	if err != nil {
		return "", err
	}
	if !has {
		return "", errLegacy("你的招贤馆内没有位置了。")
	}
	sex := int(miscMtRand(0, 1))
	name := "热血勇士"
	face := 1001 + int(miscMtRand(0, 69))
	if sex == 0 {
		face = 1 + int(miscMtRand(0, 8))
	}
	if err := s.ReduceGoods(ctx, uid, 10215, 1, 0); err != nil {
		return "", err
	}
	if err := s.miscInsertHero(ctx, uid, cid, name, sex, face, 2, 100, 60, 80, 60, 70, 30, 142); err != nil {
		return "", err
	}
	return "你得到了一名将领。", nil
}

// openHeroBoxTongling 对齐 GoodsFunc.php:3535（gid11227 统领召唤令，sex 固定 1）。
func (s *Service) openHeroBoxTongling(ctx context.Context, uid int) (string, error) {
	cid, err := s.lastCityID(ctx, uid)
	if err != nil {
		return "", err
	}
	ok, err := s.checkGoods(ctx, uid, 11227)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errLegacy("你没有这个道具。")
	}
	has, err := s.miscCityHasHeroPosition(ctx, uid, cid)
	if err != nil {
		return "", err
	}
	if !has {
		return "", errLegacy("你的招贤馆内没有位置了。")
	}
	name := "禁军统领" // lang useGoods.act_hero_tongling
	face := 1001 + int(miscMtRand(0, 69))
	if err := s.ReduceGoods(ctx, uid, 11227, 1, 0); err != nil {
		return "", err
	}
	if err := s.miscInsertHero(ctx, uid, cid, name, 1, face, 2, 100, 90, 100, 90, 80, 60, 1001); err != nil {
		return "", err
	}
	return "你得到了一名将领。", nil
}

// openHeroBoxShiwei 对齐 GoodsFunc.php:3558（gid11228 侍卫召唤令）。
// 怪癖保留：act_hero_shiwei 与 act_hero_tongling 同为"禁军统领"（lang.php:2556-2557 原版如此）。
func (s *Service) openHeroBoxShiwei(ctx context.Context, uid int) (string, error) {
	cid, err := s.lastCityID(ctx, uid)
	if err != nil {
		return "", err
	}
	ok, err := s.checkGoods(ctx, uid, 11228)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errLegacy("你没有这个道具。")
	}
	has, err := s.miscCityHasHeroPosition(ctx, uid, cid)
	if err != nil {
		return "", err
	}
	if !has {
		return "", errLegacy("你的招贤馆内没有位置了。")
	}
	name := "禁军统领" // lang useGoods.act_hero_shiwei
	face := 1001 + int(miscMtRand(0, 69))
	if err := s.ReduceGoods(ctx, uid, 11228, 1, 0); err != nil {
		return "", err
	}
	if err := s.miscInsertHero(ctx, uid, cid, name, 1, face, 2, 100, 70, 90, 70, 80, 50, 1002); err != nil {
		return "", err
	}
	return "你得到了一名将领。", nil
}

// openHeroBoxJiajiang 对齐 GoodsFunc.php:3580（gid12051 热血家将）。
func (s *Service) openHeroBoxJiajiang(ctx context.Context, uid int) (string, error) {
	cid, err := s.lastCityID(ctx, uid)
	if err != nil {
		return "", err
	}
	ok, err := s.checkGoods(ctx, uid, 12051)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errLegacy("你没有这个道具。")
	}
	has, err := s.miscCityHasHeroPosition(ctx, uid, cid)
	if err != nil {
		return "", err
	}
	if !has {
		return "", errLegacy("你的招贤馆内没有位置了。")
	}
	name := "热血家将" // lang useGoods.act_hero_jiajiang
	face := 1001 + int(miscMtRand(0, 69))
	if err := s.ReduceGoods(ctx, uid, 12051, 1, 0); err != nil {
		return "", err
	}
	if err := s.miscInsertHero(ctx, uid, cid, name, 1, face, 2, 100, 70, 80, 70, 80, 70, 1003); err != nil {
		return "", err
	}
	return "你得到了一名将领。", nil
}

// addArmorShelf 对齐 GoodsFunc.php:4297。145→+5、146→+50（读 PHP 确认）。
// 降级：users 无 armor_column 列 → 无法读取/持久化，当前栏位按 0（不触发 500 上限与成就），
// 仅扣道具并返回增量后的目标值（dispatch 拼"你的装备栏增加到%d个"）。
func (s *Service) addArmorShelf(ctx context.Context, uid, gid int) (int, error) {
	count := int64(0) // armor_column 列缺失，恒 0
	if count >= 500 {
		return 0, errLegacy("你的装备栏已经最大，不能再增加")
	}
	goodsCnt, err := s.goodsCount(ctx, uid, gid)
	if err != nil {
		return 0, err
	}
	if goodsCnt <= 0 { // legacy empty($goods)||count<=0
		return 0, errLegacy(fmt.Sprintf("not_enough_goods%d", gid))
	}
	add := int64(0)
	if gid == 145 {
		add = 5
	}
	if gid == 146 {
		add = 50
	}
	target := count + add
	if target > 500 {
		target = 500
	}
	// 成就 36（500 格）属 M8，未接线
	// legacy 用 addGoods($uid,$gid,-1,0)（带符号，不夹 0）
	if err := s.AddGoods(ctx, uid, gid, -1, 0); err != nil {
		return 0, err
	}
	return int(target), nil
}

// useShenMiChuanYinFu 对齐 GoodsFunc.php:4332（mode 6 题目列表）。
// 降级：sys_question 表无 → checkGoods 失败报"你没有神秘传音符"；通过后取题失败
// 报"此功能尚未开放。"（lang useGoods.func_not_in_use），道具不扣（PHP 扣在取题之后，不可达）。
func (s *Service) useShenMiChuanYinFu(ctx context.Context, uid int) ([]map[string]any, error) {
	ok, err := s.checkGoods(ctx, uid, 147)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errLegacy("你没有神秘传音符")
	}
	return nil, errLegacy("此功能尚未开放。")
}

// giveMeHeroCard 对齐 GoodsFunc.php:4433。
// 降级：mem_state（state=197 服务器开关）表无 → yysType 恒 0 ≠ 60/55555555 → 该功能未开放。
func (s *Service) giveMeHeroCard(ctx context.Context, uid, gid int) (string, error) {
	_ = uid
	_ = gid
	return "", errLegacy("该功能未开放")
}

// generateHero4Card 对齐 GoodsFunc.php:4482。降级同 giveMeHeroCard（mem_state 表无）。
func (s *Service) generateHero4Card(ctx context.Context, uid, cid, gid int) (string, error) {
	_ = uid
	_ = cid
	_ = gid
	return "", errLegacy("该功能未开放")
}

// useArmyOrder 对齐 GoodsFunc.php:4623（251-254 军令）。降级同 giveMeHeroCard（mem_state 表无）。
func (s *Service) useArmyOrder(ctx context.Context, uid, cid, gid int) (string, error) {
	_ = uid
	_ = cid
	_ = gid
	return "", errLegacy("该功能未开放")
}

// miscDoCheckCityAndType 对齐 GoodsFunc.php:5251 doCheckCityAndType。
// 降级：cfg_soldier_special_city 表无 → 步罗城检查恒空跳过。
func (s *Service) miscDoCheckCityAndType(cityInfo map[string]any) error {
	if len(cityInfo) == 0 {
		return errLegacy("当前城池不存在")
	}
	t := model.Int(cityInfo, "type")
	sp := model.Int(cityInfo, "is_special")
	if t > 0 || sp == 1 || sp == 2 || sp == 99 || sp == 98 || sp == 97 {
		return errLegacy("当前城池无法使用该道具，请切换到普通城池进行使用。")
	}
	return nil
}

// changeCityMap 对齐 GoodsFunc.php:5168。
// 怪癖保留：扣道具用 addGoods($uid,$gid,-1,0706)（PHP 八进制 0706=454）；
// 皮肤时效 buff 存 city_buffers 时 buftype=gid、bufparam='705'（以 PHP 列序为准）。
// 文案 skinGood_not_enough 在 lang.php 未定义 → PHP 运行时为空消息，1:1 保留 errLegacy("")。
func (s *Service) changeCityMap(ctx context.Context, uid, cid, gid int) (string, error) {
	cityInfo, err := s.db.FetchOne(ctx, "select * from cities where id=? and user_id=?", cid, uid)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
		cityInfo = nil
	}
	if err := s.miscDoCheckCityAndType(cityInfo); err != nil {
		return "", err
	}
	count, err := s.goodsCount(ctx, uid, gid)
	if err != nil {
		return "", err
	}
	if count < 1 {
		return "", errLegacy("")
	}
	goodName, err := s.db.FetchCellString(ctx, "select name from cfg_goods where gid=?", gid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	isSpecial := 0
	switch gid {
	case 10932:
		isSpecial = 10
	case 10933:
		isSpecial = 11
	case 10934:
		isSpecial = 12
	case 10935:
		isSpecial = 13
	case 10936:
		isSpecial = 14
	case 10937:
		isSpecial = 15
	case 10996:
		isSpecial = 99
	case 11021:
		isSpecial = 16
	case 11022:
		isSpecial = 98
	case 11078:
		isSpecial = 97
	}

	// 扣除道具（addGoods 带符号，type=454）
	if err := s.AddGoods(ctx, uid, gid, -1, 454); err != nil {
		return "", err
	}
	bufInfo, err := s.db.FetchOne(ctx, "select * from city_buffers where city_id=? and bufparam='705' limit 1", cid)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
		bufInfo = nil
	}

	if gid == 10996 || gid == 11022 || gid == 11078 { // 永久皮肤
		if bufInfo != nil {
			if _, err := s.db.Exec(ctx, "delete from city_buffers where city_id=? and bufparam='705' limit 1", cid); err != nil {
				return "", err
			}
		}
		if _, err := s.db.Exec(ctx, "update cities set is_special=? where id=?", isSpecial, cid); err != nil {
			return "", err
		}
		return "使用成功，恭喜您获得永久的城池皮肤效果", nil
	}

	now, err := s.db.Now(ctx)
	if err != nil {
		return "", err
	}
	interval := int64(604800) // 7 天
	timeEnd := now + interval

	if _, err := s.db.Exec(ctx, "update cities set is_special=? where id=?", isSpecial, cid); err != nil {
		return "", err
	}
	if bufInfo == nil {
		if _, err := s.db.Exec(ctx,
			"insert into city_buffers (city_id,buftype,bufparam,endtime) values (?,?, '705',?)",
			cid, gid, timeEnd); err != nil {
			return "", err
		}
	} else if model.Int(bufInfo, "buftype") == gid {
		// legacy 尾条件 "$bufInfo[bufparam]='705'" 恒真（'705'='705'），等价 bufparam='705'
		if _, err := s.db.Exec(ctx,
			"update city_buffers set endtime=endtime+? where city_id=? and buftype=? and bufparam='705'",
			interval, cid, gid); err != nil {
			return "", err
		}
	} else {
		if _, err := s.db.Exec(ctx,
			"update city_buffers set endtime=?, buftype=? where city_id=? and buftype=? and bufparam='705'",
			timeEnd, gid, cid, model.Int(bufInfo, "buftype")); err != nil {
			return "", err
		}
	}
	endtimeStr, err := s.db.FetchCellString(ctx,
		"select from_unixtime(endtime) from city_buffers where city_id=? and buftype=? and bufparam='705' limit 1", cid, gid)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
		endtimeStr = ""
	}
	return fmt.Sprintf("使用成功，“%s”效果将持续到%s", goodName, endtimeStr), nil
}

// userWangZheZhiCheng 对齐 GoodsFunc.php:4811（gid10816 王者之城图纸）。
// 降级：sys_user_designation 表无 → 王者称号授予省略。
// not_enough_goods83 取 lang.php 后定义值（:2311"没有足够的此商品，请先购买"）。
func (s *Service) userWangZheZhiCheng(ctx context.Context, uid, gid, cid int) error {
	if gid != 10816 {
		return errLegacy("没有足够的此商品，请先购买")
	}
	ok, err := s.checkGoods(ctx, uid, gid)
	if err != nil {
		return err
	}
	if !ok {
		return errLegacy("没有足够的此商品，请先购买")
	}
	cityInfo, err := s.db.FetchOne(ctx,
		"select c.* from cities c join buildings b on c.id=b.city_id "+
			"where c.id=? and c.user_id=? and b.building_id=6 limit 1", cid, uid)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		cityInfo = nil
	}
	if err := s.miscDoCheckCityAndType(cityInfo); err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx, "update cities set is_special=2 where id=?", cid); err != nil {
		return err
	}
	if err := s.ReduceGoods(ctx, uid, gid, 1, 0); err != nil {
		return err
	}
	// 清理当前城池原来的时效性皮肤 buff
	if _, err := s.db.Exec(ctx, "delete from city_buffers where city_id=? and bufparam='705'", cid); err != nil {
		return err
	}
	return nil
}

// useWangZheBingFu 对齐 GoodsFunc.php:5126（gid10931 王者兵符）。
// 降级：cfg_soldier_special_city 表无 → 恒走 legacy"空配置"分支（首次开启），
// 特殊兵种写入省略，仅扣道具（addGoods 带符号 type=703）并返回 add_specialSoldier_succ。
// cities 无 province 列 → province 按 0（省略值，未使用）。
func (s *Service) useWangZheBingFu(ctx context.Context, uid, cid, gid int) (string, error) {
	cityInfo, err := s.db.FetchOne(ctx, "select * from cities where id=? and user_id=?", cid, uid)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
		cityInfo = nil
	}
	if len(cityInfo) == 0 {
		return "", errLegacy("当前城池不存在")
	}
	if model.Int(cityInfo, "is_special") != 2 {
		return "", errLegacy("当前城池无法使用兵符，请切换到王者之城进行使用。")
	}
	count, err := s.goodsCount(ctx, uid, gid)
	if err != nil {
		return "", err
	}
	if count < 1 {
		return "", errLegacy("你的王者兵符数量不够")
	}
	sid := miscMtRand(45, 50)
	_, err = s.db.FetchCellString(ctx, "select name from cfg_soldiers where sid=?", sid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	// isOwnInfo 恒空（cfg_soldier_special_city 表无）→ 开启分支：insert 省略，type=703
	if err := s.AddGoods(ctx, uid, gid, -1, 703); err != nil {
		return "", err
	}
	return "恭喜您成功开启特殊兵种招募功能", nil
}

// getOutTroops 对齐 GoodsFunc.php:4899（19061-63 南蛮/山越/匈奴战书）。
// 降级：mem_world/cfg_province 表无 → 19063 州府检查与产兵/行军/战报省略；
// 通过后扣 1 份战书（addGoods 带符号 type=1），返回 nil（成功文案由 dispatch 层拼装）。
func (s *Service) getOutTroops(ctx context.Context, uid, cid, gid int) error {
	ok, err := s.checkGoods(ctx, uid, gid)
	if err != nil {
		return err
	}
	if !ok {
		return errLegacy("没有足够的此商品，请先购买")
	}
	if gid != 19061 && gid != 19062 && gid != 19063 {
		return errLegacy("你异常了!!!")
	}
	own, err := s.db.Exists(ctx, "select 1 from cities where user_id=? and id=? limit 1", uid, cid)
	if err != nil {
		return err
	}
	if !own {
		return errLegacy("你异常了!!!")
	}
	// 19063 的 mem_world.province 检查：表无 → 省略（任务约定）
	// 蛮族行军/报警/战报（sys_troops legacy 列 + sendReport）：属 M7 → 省略
	if err := s.AddGoods(ctx, uid, gid, -1, 1); err != nil {
		return err
	}
	return nil
}
