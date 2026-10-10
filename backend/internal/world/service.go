package world

// service.go M11 世界地图批次：1:1 复刻 legacy server/game/WorldFunc.php 全部函数
// （doGetWorldInfo/getBlockData/getWorldCityInfo/getWorldFieldInfo/startWar/createCityFromLand/
//   checkIsInSili/addFavourites/getFavouritesList/deleteFavourites/setFavouritesComments/
//   getMaxCountByOfficePos/getGovernInfo/governOthers/getMapCity/checkCanInvade/markCity/clearMark/
//   getActionField/getUserFields）。
// 依赖的公共函数逐字对齐 legacy utils.php：addCityResources(193)/addCityPeople(261)/addCitySoldier(283)/
//   updateCityResourceAdd(926)/updateCityHeroChange(677)/resetCityGoodsAdd(1479)/getBufferNobility(1534)/
//   sendReport(133)/completeTask(703)/logUserAction(1808)/MakeEndTime(55)/MakeTimeLeft(63)/getAssingStartTime(1948)。
//
// 裁剪（遵循用户已确认清单，保留判断结构，按原版 false/0/空 语义）：
//   - unionAssit(988)：依赖 sys_union/sys_union_member（M8 整体排除联盟）→ 不实现该函数。
//   - getLuoyangCityInfo(192)、getWorldCityInfo 的 cid==215265 洛阳分支(176-184)、getMapCity 的 type==4 分支(794-804)：
//     M7/M8 已排除洛阳（log_luoyang_belong 无表）→ 按无洛阳处理（跳过）。
//   - markCity 的盟主发信循环(910-914)：新库无 sys_mail_* 邮件表 → 不发信（标记写入仍保留）。
//   - checkIsInSili 的 sendSysInform 全服公告(444)：跨服/公告裁剪 → no-op。
//   - 跨服/战场/排行/反沉迷：与前批次一致裁剪（mem_user_trickwar 计策模块未实现，user_trickwars 恒空）。
//
// ⚠ startWar(209) 原版语义为「宣战」（写 mem_user_inwar 并回传 getWorldCityInfo），**不涉及 troops 落库**；
//   本实现严格 1:1，任务描述中"startWar 出征 troops 落库"与原版不符，已在汇报中标注。

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"rxsg/backend/internal/db"
	"rxsg/backend/internal/game"
	"rxsg/backend/internal/httpx"
	"rxsg/backend/internal/lock"
	"rxsg/backend/internal/model"
)

// NPCUIDEnd 对齐 common.php:34 NPC_UID_END（<=897 视为 NPC）。
const npcUIDEnd = 897

type Service struct {
	db *db.DB
	lk *lock.Locker
}

func NewService(d *db.DB) *Service {
	return &Service{db: d, lk: lock.New(d.DB)}
}

// WithUserLock 包裹写操作，对齐 legacy lockUser/unlockUser 区间。
func (s *Service) WithUserLock(ctx context.Context, uid int, key string, fn func(context.Context) error) error {
	err := s.lk.WithUserLock(ctx, uid, key, fn)
	if err != nil {
		if err.Error() == "lock_busy" {
			return errLegacy("服务器忙，请稍后再进行操作。")
		}
		return err
	}
	return nil
}

// errLegacy 构造与 legacy throw new Exception 等价的错误响应（HTTP 400）。
// 原版"收藏成功/修改备注成功/标记成功/下政令成功"等成功提示同样经由 throw 返回，1:1 保留。
func errLegacy(msg string) error {
	return httpx.BadRequest("world_error", msg)
}

// ── 坐标/地格换算（utils.php:663 cid2wid / utils.php:462 wid2cid，逐行移植）──────

// cid2wid 对齐 utils.php:663：
//
//	$y = floor($cid/1000); $x = $cid%1000;
//	(floor($y/10))*10000 + (floor($x/10))*100 + ($y%10)*10 + ($x%10)
func cid2wid(cid int) int {
	y := cid / 1000
	x := cid % 1000
	return (y/10)*10000 + (x/10)*100 + (y%10)*10 + (x % 10)
}

// wid2cid 对齐 utils.php:462：
//
//	$y = floor($wid/10000)*10 + floor(($wid%100)/10);
//	$x = floor(($wid%10000)/100)*10 + floor($wid%10);
//	return $y*1000 + $x
func wid2cid(wid int) int {
	y := (wid/10000)*10 + (wid%100)/10
	x := ((wid%10000)/100)*10 + wid%10
	return y*1000 + x
}

// wid2cidExpr 是 wid2cid 的 SQL 形式（CheckCanInvade:829 内联表达式，逐字对应）。
const wid2cidExpr = "(floor(w.wid / 10000) * 10 + floor(((w.wid % 100) / 10)))*1000 + floor((w.wid % 10000) / 100) * 10 + floor(w.wid % 10)"

// ── 低层取值工具（对齐 legacy sql_fetch_one_cell/empty→0 语义）──────────────────

func (s *Service) cellInt(ctx context.Context, query string, args ...any) (int64, error) {
	v, err := s.db.FetchCellInt64(ctx, query, args...)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return v, err
}

func (s *Service) cellStr(ctx context.Context, query string, args ...any) (string, error) {
	v, err := s.db.FetchCellString(ctx, query, args...)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// ── 公共工具函数移植 ───────────────────────────────────────────────────────

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

// addCityPeople 对齐 utils.php:261。
func (s *Service) addCityPeople(ctx context.Context, cid int, count int64) error {
	_, err := s.db.Exec(ctx, "update city_resources set people=people+? where city_id=?", count, cid)
	return err
}

// addCitySoldier 对齐 utils.php:283（写入 city_soldiers + log_city_soldiers；后者 uid 恒 0、type=9——原版怪癖保留）。
func (s *Service) addCitySoldier(ctx context.Context, cid, sid int, count int64) error {
	if _, err := s.db.Exec(ctx,
		"insert into city_soldiers (city_id, soldier_id, `count`) values (?,?,?) on duplicate key update `count`=`count`+?",
		cid, sid, count, count); err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx,
		"insert into log_city_soldiers (cid, sid, uid, `count`, `type`) values (?,?,0,?,9) on duplicate key update `count`=`count`+?",
		cid, sid, count, count); err != nil {
		return err
	}
	return s.updateCityResourceAdd(ctx, cid)
}

// updateCityResourceAdd 对齐 utils.php:926：置附属城的 resource_changing=1。
func (s *Service) updateCityResourceAdd(ctx context.Context, cid int) error {
	ownercid, err := s.cellInt(ctx, "select ownercid from mem_world where wid=?", cid2wid(cid))
	if err != nil {
		return err
	}
	if ownercid == 0 {
		ownercid = int64(cid)
	}
	if ownercid == 0 {
		return nil
	}
	_, err = s.db.Exec(ctx, "update city_res_add set resource_changing=1 where city_id=?", ownercid)
	return err
}

// updateCityHeroChange 对齐 utils.php:677：重算 hero_fee（英雄成将俸禄）。
func (s *Service) updateCityHeroChange(ctx context.Context, uid, cid int) error {
	v, err := s.cellInt(ctx, `select coalesce(sum(level*20 +
		(greatest(affairs_base+affairs_add-90,0)+greatest(bravery_base+bravery_add-90,0)+greatest(wisdom_base+wisdom_add-90,0))*50),0)
		from heroes where hero_type<>1000 and city_id=? and user_id=? and state not in (5,6,9)`, cid, uid)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, "update city_resources set hero_fee=? where city_id=?", v, cid)
	return err
}

// resetCityGoodsAdd 对齐 utils.php:1479：按道具 buff（buftype 1~4）重置 goods_*_add。
func (s *Service) resetCityGoodsAdd(ctx context.Context, uid, cid int) error {
	now, err := s.db.Now(ctx)
	if err != nil {
		return err
	}
	rows, err := s.db.FetchRows(ctx,
		"select buftype from user_buffers where user_id=? and buftype<=4 and endtime>?", uid, now)
	if err != nil {
		return err
	}
	var foodAdd, woodAdd, rockAdd, ironAdd int
	for _, r := range rows {
		switch model.Int(r, "buftype") {
		case 1:
			foodAdd = 25
		case 2:
			woodAdd = 25
		case 3:
			rockAdd = 25
		case 4:
			ironAdd = 25
		}
	}
	_, err = s.db.Exec(ctx, `update city_res_add set goods_food_add=?,goods_wood_add=?,goods_rock_add=?,goods_iron_add=?,
		resource_changing=1 where city_id=?`, foodAdd, woodAdd, rockAdd, ironAdd, cid)
	return err
}

// getBufferNobility 对齐 utils.php:1534：推恩令（user_buffers buftype∈{16,18}）提升爵位。
func (s *Service) getBufferNobility(ctx context.Context, uid int, realNobility float64) (int, error) {
	bufparam, err := s.cellInt(ctx,
		"select bufparam from user_buffers where user_id=? and (buftype=16 or buftype=18) order by bufparam desc limit 1", uid)
	if err != nil {
		return 0, err
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

// makeEndTime 对齐 utils.php:55：用数据库 from_unixtime 按 "%Y年%m月%d日 %H:%i:%s" 格式化。
func (s *Service) makeEndTime(ctx context.Context, ts int64) (string, error) {
	return s.cellStr(ctx, "select from_unixtime(?, ?)", ts, "%Y年%m月%d日 %H:%i:%s")
}

// makeTimeLeft 对齐 utils.php:63：把剩余秒数格式化为中文。
func makeTimeLeft(timeleft int64) string {
	hour := timeleft / 3600
	minute := (timeleft - hour*3600) / 60
	second := timeleft % 60
	switch {
	case hour > 0:
		return fmt.Sprintf("%d小时%d分钟%d秒", hour, minute, second)
	case minute > 0:
		return fmt.Sprintf("%d分钟%d秒", minute, second)
	default:
		return fmt.Sprintf("%d秒", second)
	}
}

// sendReport 对齐 utils.php:133（sig: touid,type,title,origincid,happencid,content）。
// 注意 legacy sendReportDetail 忽略传入 $type，改用由 title 推导的 stype——原版行为保留。
func (s *Service) sendReport(ctx context.Context, touid, typ, title, origincid, happencid int, content string) error {
	origincity := ""
	if origincid > 0 {
		origincity, _ = s.cellStr(ctx, "select name from cities where id=?", origincid)
		if origincity == "" {
			origincity = s.worldTypeName(ctx, origincid)
		}
	}
	happencity := ""
	if origincid == happencid {
		happencity = origincity
	} else if happencid > 0 {
		happencity, _ = s.cellStr(ctx, "select name from cities where id=?", happencid)
		if happencity == "" {
			happencity = s.worldTypeName(ctx, happencid)
		}
	}
	stype := 3
	switch {
	case title <= 11:
		stype = 0
	case title >= 12 && title <= 14:
		stype = 1
	case title == 19:
		stype = 2
	}
	if _, err := s.db.Exec(ctx,
		"insert into reports (user_id, origincid, origincity, happencid, happencity, title, `type`, `time`, `read`, battleid, content) "+
			"values (?,?,?,?,?,?,?,unix_timestamp(),0,0,?)",
		touid, origincid, origincity, happencid, happencity, title, stype, content); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, "insert into alarms (user_id, report) values (?,1) on duplicate key update report=1", touid)
	return err
}

// worldTypeName 对齐 sendReport 的野地名回退：mem_world join cfg_world_type。
func (s *Service) worldTypeName(ctx context.Context, cid int) string {
	name, _ := s.cellStr(ctx,
		"select c.name from mem_world m left join cfg_world_type c on c.type=m.type where m.wid=?", cid2wid(cid))
	return name
}

// completeTask 对齐 utils.php:703（replace into sys_user_goal → user_goals）。
func (s *Service) completeTask(ctx context.Context, uid, gid int) error {
	_, err := s.db.Exec(ctx, "replace into user_goals (uid, gid) values (?,?)", uid, gid)
	return err
}

// logUserAction 对齐 utils.php:1808。
func (s *Service) logUserAction(ctx context.Context, uid, aid int) error {
	if _, err := s.db.Exec(ctx, "insert into log_user_actions (user_id, aid, `time`) values (?,?,unix_timestamp())", uid, aid); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, "insert into log_action_counts (user_id, aid, `count`) values (?,?,1) on duplicate key update `count`=`count`+1", uid, aid)
	return err
}

// ── 1. doGetWorldInfo（WorldFunc.php:6）────────────────────────────────────
// 返回该城全部武将（legacy: select * from sys_city_hero where cid=$cid）。
func (s *Service) DoGetWorldInfo(ctx context.Context, uid, cid int) ([]map[string]any, error) {
	return s.db.FetchRows(ctx, "select * from heroes where city_id=?", cid)
}

// ── 2. getBlockData（WorldFunc.php:11）─────────────────────────────────────
// 返回 [ [ [blockstart, group_concat串], ... ], marks ]。
// 原版会写 getBlockData_sql.log/getBlockData.log 调试文件——服务器本地副作用，新栈不写（保留 SQL 语义）。
func (s *Service) GetBlockData(ctx context.Context, uid int, blocks []int) ([]any, error) {
	if err := s.clearMark(ctx, uid); err != nil {
		return nil, err
	}
	ret := make([]any, 0, len(blocks))
	for _, block := range blocks {
		blockstart := block * 100
		blockend := blockstart + 100
		concat, err := s.cellStr(ctx,
			"select group_concat((wid - ?),':',type,':',ownercid,':',state,':',level,':',province,':',jun) "+
				"from mem_world where wid >= ? and wid < ?", blockstart, blockstart, blockend)
		if err != nil {
			return nil, err
		}
		ret = append(ret, []any{blockstart, concat})
	}
	unionid, err := s.cellInt(ctx, "select union_id from users where id=?", uid)
	if err != nil {
		return nil, err
	}
	marks, err := s.db.FetchRows(ctx, "select * from union_marks where unionid=?", unionid)
	if err != nil {
		return nil, err
	}
	if marks == nil {
		marks = []map[string]any{}
	}
	return []any{ret, marks}, nil
}

// ── 3. getWorldCityInfo（WorldFunc.php:46）─────────────────────────────────
func (s *Service) GetWorldCityInfo(ctx context.Context, uid int, cities []int) ([]map[string]any, error) {
	result := make([]map[string]any, 0)
	if len(cities) == 0 {
		return result, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(cities)), ",")
	args := make([]any, 0, len(cities))
	for _, c := range cities {
		args = append(args, c)
	}
	rows, err := s.db.FetchRows(ctx,
		"select c.id as cid,c.type as citytype,c.is_special as is_special,c.province as provinceId,c.name as cityname,"+
			"u.id as uid,u.nickname as username,u.passport,u.union_id,n.name as unionname,u.prestige,u.state as userstate "+
			"from cities c, users u left join unions n on u.union_id=n.id "+
			"where c.id in ("+placeholders+") and c.user_id=u.id", args...)
	if err != nil {
		return nil, err
	}
	user, err := s.db.FetchOne(ctx, "select * from users where id=?", uid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	meUID := model.Int(user, "id")
	meUnion := model.Int(user, "union_id")
	now, err := s.db.Now(ctx)
	if err != nil {
		return nil, err
	}
	for _, city := range rows {
		// 补 legacy 列（新库 users 无 flagchar/face/sex → 空/0）。
		city["flagchar"] = ""
		city["userface"] = 0
		city["usersex"] = 0
		flag, err := s.cityFlag(ctx, meUID, meUnion, now, city)
		if err != nil {
			return nil, err
		}
		city["flag"] = flag
		result = append(result, city)
	}
	return result, nil
}

// cityFlag 复刻 getWorldCityInfo:64-175 的小旗判定（保留全部分支与顺序）。
func (s *Service) cityFlag(ctx context.Context, meUID, meUnion int, now int64, city map[string]any) (int, error) {
	cityUID := model.Int(city, "uid")
	cityUnion := model.Int(city, "union_id")
	cityType := model.Int(city, "citytype")
	flag := 7
	if cityUID == meUID {
		flag = 0
	} else if cityUnion == meUnion && cityUnion > 0 {
		flag = 1
	} else {
		relation, err := s.db.FetchOne(ctx, "select * from union_relations where unionid=? and target=?", meUnion, cityUnion)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
		if relation != nil {
			switch model.Int(relation, "type") {
			case 0:
				flag = 2
			case 1:
				flag = 3
			case 2:
				if now-model.Int64(relation, "time") > 3600*8 {
					flag = 4
				} else {
					flag = 8
				}
				if flag == 8 {
					f, err := s.personalWarFlag(ctx, meUID, cityUID, flag)
					if err != nil {
						return 0, err
					}
					flag = f
				}
			default:
				f, err := s.npcOrPersonalFlag(ctx, meUID, cityUID, cityType, -1)
				if err != nil {
					return 0, err
				}
				flag = f
			}
		} else {
			f, err := s.npcOrPersonalFlag(ctx, meUID, cityUID, cityType, -1)
			if err != nil {
				return 0, err
			}
			flag = f
		}
	}
	return flag, nil
}

// npcOrPersonalFlag 复刻 getWorldCityInfo:111-139/141-170 的共用分支。
// cur 为当前 flag（-1 表示未设置，走完整分支）。
func (s *Service) npcOrPersonalFlag(ctx context.Context, meUID, cityUID, cityType, cur int) (int, error) {
	if cityUID < npcUIDEnd || (cityType > 0 && cityType != 5) {
		// NPC 城和特殊城可以直接占领
		return 5, nil
	}
	if ok, err := s.trickwarExists(ctx, meUID, cityUID); err != nil {
		return 0, err
	} else if ok {
		return 6, nil
	}
	inwar, err := s.db.FetchOne(ctx,
		"select * from user_inwars where endtime>unix_timestamp() and ((uid=? and targetuid=?) or (targetuid=? and uid=?))",
		meUID, cityUID, meUID, cityUID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if inwar != nil {
		if model.Int(inwar, "state") == 0 {
			return 8, nil
		}
		return 6, nil
	}
	// 没有小旗
	return 7, nil
}

// personalWarFlag 复刻 getWorldCityInfo:92-109 的"联盟宣战等待中"子分支。
func (s *Service) personalWarFlag(ctx context.Context, meUID, cityUID, cur int) (int, error) {
	if ok, err := s.trickwarExists(ctx, meUID, cityUID); err != nil {
		return 0, err
	} else if ok {
		return 6, nil
	}
	inwar, err := s.db.FetchOne(ctx,
		"select * from user_inwars where endtime>unix_timestamp() and ((uid=? and targetuid=?) or (targetuid=? and uid=?))",
		meUID, cityUID, meUID, cityUID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if inwar != nil {
		if model.Int(inwar, "state") == 0 {
			return 8, nil
		}
		return 6, nil
	}
	return cur, nil
}

func (s *Service) trickwarExists(ctx context.Context, meUID, cityUID int) (bool, error) {
	return s.db.Exists(ctx,
		"select 1 from user_trickwars where endtime>unix_timestamp() and ((uid=? and targetuid=?) or (uid=? and targetuid=?))",
		meUID, cityUID, cityUID, meUID)
}

// ── 4. getLuoyangCityInfo（WorldFunc.php:192）→ 裁剪（洛阳，M7/M8 排除）──────

// ── 5. getWorldFieldInfo（WorldFunc.php:202）───────────────────────────────
// 返回 [ row ]（单元素数组）。
func (s *Service) GetWorldFieldInfo(ctx context.Context, uid, wid int) ([]any, error) {
	row, err := s.db.FetchOne(ctx,
		"select w.wid,w.type,w.ownercid,w.province,w.level,c.id as cid,c.name as cityname,u.nickname as username,u.prestige,u.union_id,n.name as unionname "+
			"from mem_world w left join cities c on c.id=w.ownercid left join users u on u.id=c.user_id left join unions n on n.id=u.union_id "+
			"where w.wid=?", wid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if row == nil {
		row = map[string]any{}
	}
	return []any{row}, nil
}

// ── 6. startWar（WorldFunc.php:209）────────────────────────────────────────
// ⚠ 原版为「宣战」：写 mem_user_inwar(user_inwars) 并回传 getWorldCityInfo，不涉及 troops。
func (s *Service) StartWar(ctx context.Context, uid, targetuid, targetcid int) ([]map[string]any, error) {
	if ok, err := s.db.Exists(ctx,
		"select 1 from user_inwars where (uid=? and targetuid=?) or (targetuid=? and uid=?)",
		uid, targetuid, uid, targetuid); err != nil {
		return nil, err
	} else if ok {
		return nil, errLegacy("你们已经处于宣战状态。") // startWar.war_is_declared
	}
	user, err := s.db.FetchOne(ctx, "select nickname,state,lastcid from users where id=?", uid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if model.Int(user, "state") == 1 {
		return nil, errLegacy("你处于新手保护状态，无法宣战。") // startWar.new_protect
	}
	targetuser, err := s.db.FetchOne(ctx, "select nickname,state,lastcid,union_id from users where id=?", targetuid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if model.Int(targetuser, "state") == 1 {
		return nil, errLegacy("对方处于新手保护状态，无法宣战。") // startWar.target_new_protect
	}
	now, err := s.db.Now(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(ctx,
		"insert into user_inwars (uid,targetuid,state,endtime) values (?,?,0,unix_timestamp()+8*3600)",
		uid, targetuid); err != nil {
		return nil, err
	}
	username := model.Str(user, "nickname")
	targetusername := model.Str(targetuser, "nickname")
	startTxt, err := s.makeEndTime(ctx, now+8*3600)
	if err != nil {
		return nil, err
	}
	endTxt, err := s.makeEndTime(ctx, now+56*3600)
	if err != nil {
		return nil, err
	}
	caution := fmt.Sprintf(startWarSuccCaution, username, startTxt, endTxt)
	if err := s.sendReport(ctx, targetuid, 22, 22, model.Int(user, "lastcid"), targetcid, caution); err != nil {
		return nil, err
	}
	report := fmt.Sprintf(startWarSuccReport, targetusername, startTxt, endTxt)
	if err := s.sendReport(ctx, uid, 22, 22, model.Int(user, "lastcid"), targetcid, report); err != nil {
		return nil, err
	}
	// union_id>0 → addUnionEvent（联盟裁剪，跳过）。
	// USER_FOR_51 / PASSTYPE 平台事件（跨服）→ 裁剪。
	cidStr, err := s.cellStr(ctx, "select group_concat(id) from cities where user_id=?", targetuid)
	if err != nil {
		return nil, err
	}
	cities := make([]int, 0)
	for _, p := range strings.Split(cidStr, ",") {
		if n, e := strconv.Atoi(strings.TrimSpace(p)); e == nil && p != "" {
			cities = append(cities, n)
		}
	}
	return s.GetWorldCityInfo(ctx, uid, cities)
}

// ── 7. createCityFromLand（WorldFunc.php:267）──────────────────────────────
// 返回空数组（legacy return array()）。
func (s *Service) CreateCityFromLand(ctx context.Context, uid, targetwid int) ([]any, error) {
	targetcid := wid2cid(targetwid)
	worldInfo, err := s.db.FetchOne(ctx, "select * from mem_world where wid=?", targetwid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	lastcid, err := s.cellInt(ctx, "select lastcid from users where id=?", uid)
	if err != nil {
		return nil, err
	}
	if model.Int(worldInfo, "type") != 1 {
		return nil, errLegacy("只有平地才能筑城") // createCityFromLand.only_flatlands_can_build
	}
	if model.Int(worldInfo, "ownercid") != int(lastcid) {
		return nil, errLegacy("目标平地不是本城池的附属平地，不能在此处筑城") // target_flatlands_notYours
	}
	if model.Int(worldInfo, "state") != 0 {
		return nil, errLegacy("目标平地正在战乱中，不能筑城。") // target_flatlands_in_war
	}
	// 原版怪癖：state=4（驻扎）的部队；新栈 army 仅产生 state 0/1，此查询恒空，"新建城池优化"下不再需要驻军
	// （WorldFunc.php:287 no_army 校验本就被注释）。
	troops, err := s.db.FetchRows(ctx, "select * from troops where user_id=? and target_id=? and state=4", uid, targetcid)
	if err != nil {
		return nil, err
	}
	var gold, food, wood, rock, iron int64
	for range troops {
		// 原版从 sys_troops.resource 字段累加（"gold,food,wood,rock,iron"）；新 troops 表无 resource 列 → 恒 0。
	}
	cityres, err := s.db.FetchOne(ctx, "select * from city_resources where city_id=?", int(lastcid))
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	goldincity := model.Int64(cityres, "gold")
	foodincity := model.Int64(cityres, "food")
	woodincity := model.Int64(cityres, "wood")
	rockincity := model.Int64(cityres, "rock")
	ironincity := model.Int64(cityres, "iron")
	if goldincity < 10000 || foodincity < 10000 || woodincity < 10000 || rockincity < 10000 || ironincity < 10000 {
		return nil, errLegacy("本城池资源不足，需要每种资源及黄金各10000才能筑城") // no_enough_resource
	}
	nobilityStr, err := s.cellStr(ctx, "select nobility from users where id=?", uid)
	if err != nil {
		return nil, err
	}
	realNobility, _ := strconv.ParseFloat(nobilityStr, 64)
	nobility, err := s.getBufferNobility(ctx, uid, realNobility)
	if err != nil {
		return nil, err
	}
	maxCityCount, err := s.cellInt(ctx, "select city_count from cfg_nobility where id=?", nobility)
	if err != nil {
		return nil, err
	}
	currentCityCount, err := s.cellInt(ctx, "select count(*) from cities where user_id=?", uid)
	if err != nil {
		return nil, err
	}
	if currentCityCount >= maxCityCount {
		nextname, _ := s.cellStr(ctx, "select name from cfg_nobility where id=?", nobility+1)
		return nil, errLegacy(fmt.Sprintf("你的爵位不够，筑城失败。当你的爵位晋升为“%s”时，才能统治更多的城池。", nextname))
	}
	if err := s.addCityResources(ctx, int(lastcid), -10000, -10000, -10000, -10000, -10000); err != nil {
		return nil, err
	}
	const newcityName = "新城池" // worldfunc.newcity
	// 清除伤兵，逃兵，俘虏
	if _, err := s.db.Exec(ctx, "delete from city_wounded where city_id=?", targetcid); err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(ctx, "delete from city_lamsters where city_id=?", targetcid); err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(ctx, "delete from city_captives where city_id=?", targetcid); err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(ctx,
		"replace into cities (id,user_id,name,type,state,province) values (?,?,?,0,0,?)",
		targetcid, uid, newcityName, model.Str(worldInfo, "province")); err != nil {
		return nil, err
	}
	// 自动建设 1 级官府（新库官府 bid=1，对齐 0003/0010 建筑映射）
	if _, err := s.db.Exec(ctx, "delete from buildings where city_id=? and xy='120'", targetcid); err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(ctx,
		"replace into buildings (city_id,xy,building_id,level) values (?, '120', ?, 1)", targetcid, game.BidGoverment); err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(ctx,
		"replace into city_resources (city_id,people,food,wood,rock,iron,gold) values (?,0,?,?,?,?,?)",
		targetcid, food, wood, rock, iron, gold); err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(ctx, "replace into city_res_add (city_id) values (?)", targetcid); err != nil {
		return nil, err
	}
	// 修改所在地的属性
	if _, err := s.db.Exec(ctx, "update mem_world set ownercid=?, type=0 where wid=?", targetcid, cid2wid(targetcid)); err != nil {
		return nil, err
	}
	// 重新计算宝物加成
	if err := s.resetCityGoodsAdd(ctx, uid, targetcid); err != nil {
		return nil, err
	}
	// 军队入住（原版：第一支军队首领作城守）。新栈 state=4 恒空，分支实际不触发。
	if len(troops) > 0 {
		if err := s.houseTroops(ctx, uid, targetcid, troops); err != nil {
			return nil, err
		}
	}
	if err := s.updateCityResourceAdd(ctx, targetcid); err != nil {
		return nil, err
	}
	if err := s.updateCityHeroChange(ctx, uid, int(lastcid)); err != nil {
		return nil, err
	}
	if err := s.updateCityHeroChange(ctx, uid, targetcid); err != nil {
		return nil, err
	}
	// 完成建立新城任务
	if err := s.completeTask(ctx, uid, 169); err != nil {
		return nil, err
	}
	if err := s.addCityResources(ctx, targetcid, 5000, 5000, 5000, 5000, 5000); err != nil {
		return nil, err
	}
	if err := s.logUserAction(ctx, uid, 1); err != nil {
		return nil, err
	}
	// 创建君主将（hero_type=1000）。原版怪癖：select 中的 '$cid' 变量在 createCityFromLand 作用域未定义 → 空串，
	// city_id 落为 0（WorldFunc.php:422）；select sex/face 来自 sys_user，新库 users 无此两列 → 0。逐字保留。
	if ok, err := s.db.Exists(ctx, "select 1 from heroes where user_id=? and hero_type=1000", uid); err != nil {
		return nil, err
	} else if !ok {
		hid, err := s.db.Insert(ctx,
			"insert into heroes (user_id,name,sex,face,city_id,state,level,command_base,affairs_base,bravery_base,wisdom_base,loyalty,hero_type) "+
				"select id,nickname,0,0,0,0,1,50,1,1,1,100,1000 from users where id=?", uid)
		if err != nil {
			return nil, err
		}
		if _, err := s.db.Exec(ctx,
			"insert into hero_blood (hero_id,`force`,force_max,energy,energy_max) values (?,150,150,150,150)", hid); err != nil {
			return nil, err
		}
	}
	// 马来定制：state=197 为 60 时，在司隶筑城需告知服务器。
	yysType, err := s.cellInt(ctx, "select value from mem_state where state=197")
	if err != nil {
		return nil, err
	}
	if yysType == 60 {
		if err := s.CheckIsInSili(ctx, uid, targetcid); err != nil {
			return nil, err
		}
	}
	return []any{}, nil
}

// houseTroops 复刻 createCityFromLand:357-392 的"军队入住"（新 troops 用 JSON soldiers 表示兵力）。
func (s *Service) houseTroops(ctx context.Context, uid, targetcid int, troops []map[string]any) error {
	hasSetChief := false
	for _, troop := range troops {
		if hid := model.Int(troop, "hero_id"); hid > 0 {
			if !hasSetChief {
				hasSetChief = true
				if _, err := s.db.Exec(ctx, "update heroes set city_id=?, state=1 where id=?", targetcid, hid); err != nil {
					return err
				}
				if _, err := s.db.Exec(ctx, "update cities set chief_hero_id=? where id=?", hid, targetcid); err != nil {
					return err
				}
			} else {
				if _, err := s.db.Exec(ctx, "update heroes set city_id=?, state=0 where id=?", targetcid, hid); err != nil {
					return err
				}
			}
		}
		for sid, cnt := range decodeSoldiers(model.Str(troop, "soldiers")) {
			if cnt <= 0 {
				continue
			}
			if _, err := s.db.Exec(ctx,
				"insert into city_soldiers (city_id,soldier_id,`count`) values (?,?,?) on duplicate key update `count`=`count`+?",
				targetcid, sid, cnt, cnt); err != nil {
				return err
			}
			if _, err := s.db.Exec(ctx,
				"insert into log_city_soldiers (cid,sid,uid,`count`,`type`) values (?,?,?,?,10) on duplicate key update `count`=`count`+?",
				targetcid, sid, uid, cnt, cnt); err != nil {
				return err
			}
		}
		if _, err := s.db.Exec(ctx, "delete from troops where id=?", model.Int(troop, "id")); err != nil {
			return err
		}
		if err := s.updateCityResourceAdd(ctx, model.Int(troop, "city_id")); err != nil {
			return err
		}
	}
	return nil
}

// ── 8. checkIsInSili（WorldFunc.php:435）───────────────────────────────────
// 马来定制：若玩家在司隶（province=1）筑城则告知服务器。sendSysInform 公告裁剪 → no-op。
func (s *Service) CheckIsInSili(ctx context.Context, uid, cid int) error {
	wid := cid2wid(cid)
	province, err := s.cellStr(ctx, "select province from mem_world where wid=?", wid)
	if err != nil {
		return err
	}
	p, _ := strconv.Atoi(province)
	if p == 1 {
		// legacy 取 userName/cityName/X/Y 后 sendSysInform(0,1,0,300,1800,1,49151,msg)；
		// 公告/跨服广播裁剪 → no-op（仅保留 province 判定与触发条件）。
	}
	return nil
}

// ── 9. addFavourites（WorldFunc.php:447）───────────────────────────────────
func (s *Service) AddFavourites(ctx context.Context, uid, targetcid int) error {
	if ok, err := s.db.Exists(ctx, "select 1 from user_favourites where uid=? and cid=?", uid, targetcid); err != nil {
		return err
	} else if ok {
		return errLegacy("该目标已被收藏。 ") // addFavourites.already_in_fav
	}
	cnt, err := s.cellInt(ctx, "select count(*) from user_favourites where uid=?", uid)
	if err != nil {
		return err
	}
	if cnt >= 10 {
		return errLegacy("你的收藏目标已达到上限10个，删除其他目标后才能继续收藏。") // addFavourites.fav_is_full
	}
	wid := cid2wid(targetcid)
	worldInfo, err := s.db.FetchOne(ctx, "select * from mem_world where wid=?", wid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var name string
	if model.Int(worldInfo, "type") == 0 {
		name, _ = s.cellStr(ctx, "select name from cities where id=?", targetcid)
	} else {
		name, _ = s.cellStr(ctx, "select name from cfg_world_type where type=?", model.Int(worldInfo, "type"))
	}
	if _, err := s.db.Exec(ctx, "insert into user_favourites (uid,cid,name,comments) values (?,?,?,'')", uid, targetcid, name); err != nil {
		return err
	}
	return errLegacy("收藏成功！你可以在校场的出征界面查看收藏列表。") // addFavourites.succ
}

// ── 10. getFavouritesList（WorldFunc.php:473）──────────────────────────────
// 返回 [ 城池(cid,name), 收藏(id,cid,name,comments) ]。
func (s *Service) GetFavouritesList(ctx context.Context, uid int) ([]any, error) {
	cities, err := s.db.FetchRows(ctx, "select id as cid,name from cities where user_id=?", uid)
	if err != nil {
		return nil, err
	}
	favs, err := s.db.FetchRows(ctx, "select id,cid,name,comments from user_favourites where uid=?", uid)
	if err != nil {
		return nil, err
	}
	return []any{cities, favs}, nil
}

// ── 11. deleteFavourites（WorldFunc.php:480）───────────────────────────────
func (s *Service) DeleteFavourites(ctx context.Context, uid, id int) ([]any, error) {
	fav, err := s.db.FetchOne(ctx, "select * from user_favourites where id=?", id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if fav == nil || model.Int(fav, "uid") != uid {
		return nil, errLegacy("删除收藏目标出错。") // deleteFavourites.error_in_del_fav
	}
	if _, err := s.db.Exec(ctx, "delete from user_favourites where id=?", id); err != nil {
		return nil, err
	}
	return s.GetFavouritesList(ctx, uid)
}

// ── 12. setFavouritesComments（WorldFunc.php:491）──────────────────────────
func (s *Service) SetFavouritesComments(ctx context.Context, uid, id int, comments string) error {
	fav, err := s.db.FetchOne(ctx, "select * from user_favourites where id=?", id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if fav == nil || model.Int(fav, "uid") != uid {
		return errLegacy("收藏目标不存在。") // setFavouritesComments.already_exist
	}
	if _, err := s.db.Exec(ctx, "update user_favourites set comments=? where id=?", comments, id); err != nil {
		return err
	}
	return errLegacy("修改目标备注成功。") // setFavouritesComments.succ
}

// ── 13. getMaxCountByOfficePos（WorldFunc.php:507）─────────────────────────
func GetMaxCountByOfficePos(cityType, officepos int) int {
	maxCount := 0
	switch cityType {
	case 1:
		if officepos == 6 {
			maxCount = 1
		} else if officepos == 7 {
			maxCount = 2
		} else if officepos >= 8 {
			maxCount = 3
		}
	case 2:
		if officepos == 9 {
			maxCount = 4
		} else if officepos == 10 {
			maxCount = 5
		} else if officepos >= 11 {
			maxCount = 6
		}
	case 3:
		if officepos >= 12 {
			maxCount = 8
		}
	case 4:
		if officepos >= 13 {
			maxCount = 10
		}
	}
	return maxCount
}

// ── 14. getGovernInfo（WorldFunc.php:527）──────────────────────────────────
// 返回 [ officename, maxCount, todayCount ]。
func (s *Service) GetGovernInfo(ctx context.Context, uid, cid int) ([]any, error) {
	row, err := s.db.FetchOne(ctx, "select govern_count,last_govern_time from city_schedule where city_id=?", cid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	officepos, err := s.cellInt(ctx, "select officepos from users where id=?", uid)
	if err != nil {
		return nil, err
	}
	officename, err := s.cellStr(ctx, "select name from cfg_office_pos where id=?", officepos)
	if err != nil {
		return nil, err
	}
	cityType, err := s.cellInt(ctx, "select type from cities where id=?", cid)
	if err != nil {
		return nil, err
	}
	maxCount := GetMaxCountByOfficePos(int(cityType), int(officepos))
	ret := []any{officename, maxCount}
	if row == nil {
		return append(ret, 0), nil
	}
	count := model.Int64(row, "govern_count")
	now, err := s.db.Now(ctx)
	if err != nil {
		return nil, err
	}
	if dayBucket(now) > dayBucket(model.Int64(row, "last_govern_time")) {
		return append(ret, 0), nil
	}
	return append(ret, count), nil
}

// dayBucket 对齐 legacy floor(($now + 8*3600) / 86400)（东八区日界）。
func dayBucket(ts int64) int64 {
	return (ts + 8*3600) / 86400
}

// ── 15. governOthers（WorldFunc.php:560）───────────────────────────────────
// 政令类型 0 收税 1 抽丁 2 征粮 3 收编 4 裁军。
func (s *Service) GovernOthers(ctx context.Context, uid, typ, tcid, tuid, cid int, cityname string) error {
	if ok, err := s.db.Exists(ctx, "select 1 from cities where user_id=? and id=?", uid, cid); err != nil {
		return err
	} else if !ok {
		return errLegacy("你异常了!!!") // sendCommand.command_exception
	}
	if ok, err := s.db.Exists(ctx, "select 1 from cities where user_id=? and id=?", tuid, tcid); err != nil {
		return err
	} else if !ok {
		return errLegacy("你异常了!!!")
	}
	// 普通城不能下达政令
	cityType, err := s.cellInt(ctx, "select type from cities where id=?", cid)
	if err != nil {
		return err
	}
	if cityType == 5 {
		cityType = 0
	}
	if cityType == 0 {
		return errLegacy("你所在城池不是名城，不能下达政令。") // governOthers.city_cannot_govern
	}
	// 官府没有达到 10 级，不能下达（新库官府 bid=1）
	governLevel, err := s.cellInt(ctx, "select level from buildings where city_id=? and building_id=?", cid, game.BidGoverment)
	if err != nil {
		return err
	}
	if governLevel < 10 {
		return errLegacy("名城官府等级达到10级，才能下达政令。") // not_enouth_government_level
	}
	// 目标城池级别大于自己，也不能下达
	targetcitytype, err := s.cellInt(ctx, "select type from cities where id=?", tcid)
	if err != nil {
		return err
	}
	if targetcitytype == 5 {
		targetcitytype = 0
	}
	if cityType <= targetcitytype {
		return errLegacy("该城池不受你管辖，不能下达政令。") // not_enough_level
	}
	// 自己盟友的城池不能政令
	userUnionId, err := s.cellInt(ctx, "select union_id from users where id=?", uid)
	if err != nil {
		return err
	}
	if userUnionId != 0 {
		if ok, err := s.db.Exists(ctx, "select 1 from users where id=? and union_id=?", tuid, userUnionId); err != nil {
			return err
		} else if ok {
			return errLegacy("目标城池不能为本盟的城池！") // luoyang.target_can_not_union
		}
	}
	// 查看封禁、休假状态
	if targetcitytype == 0 {
		st, err := s.db.FetchOne(ctx,
			"select forbiend,vacend,unix_timestamp() as nowtime from user_states where uid=? and (forbiend>unix_timestamp() or vacend>unix_timestamp())", tuid)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if st != nil {
			if model.Int64(st, "forbiend") > model.Int64(st, "nowtime") {
				return errLegacy("该城池处于免战状态，不能下达政令。") // target_not_in_war
			} else if model.Int64(st, "vacend") > model.Int64(st, "nowtime") {
				return errLegacy("该城池处于免战状态，不能下达政令。")
			}
		}
	}
	// 原版怪癖：$cityTypeNameField = "big_city_"+$cityType → PHP 数值化后等于 $cityType 本身（L626）。
	cityTypeName, err := s.cellStr(ctx, "select value from cfg_name where name=?", strconv.Itoa(int(cityType)))
	if err != nil {
		return err
	}
	officepos, err := s.cellInt(ctx, "select officepos from users where id=?", uid)
	if err != nil {
		return err
	}
	officename, err := s.cellStr(ctx, "select name from cfg_office_pos where id=?", officepos)
	if err != nil {
		return err
	}
	wid := cid2wid(cid)
	twid := cid2wid(tcid)
	world, err := s.db.FetchOne(ctx, "select province,jun from mem_world where wid=?", wid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	tworld, err := s.db.FetchOne(ctx, "select province,jun from mem_world where wid=?", twid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	province := model.Str(world, "province")
	jun := model.Str(world, "jun")
	tprovince := model.Str(tworld, "province")
	tjun := model.Str(tworld, "jun")
	x := cid % 1000
	y := cid / 1000
	maxCount := GetMaxCountByOfficePos(int(cityType), int(officepos))
	if cityType == 1 {
		pos := (cid/10000)*100 + ((cid % 1000) / 10)
		tpos := (tcid/10000)*100 + ((tcid % 1000) / 10)
		if pos != tpos {
			return errLegacy("该城池不受你管辖，不能下达政令。")
		}
	} else if cityType == 2 {
		if jun != tjun || province != tprovince {
			return errLegacy("该城池不受你管辖，不能下达政令。")
		}
	} else if cityType == 3 {
		if province != tprovince {
			return errLegacy("该城池不受你管辖，不能下达政令。")
		}
	}
	now, err := s.db.Now(ctx)
	if err != nil {
		return err
	}
	lastBeGovernTime, err := s.cellInt(ctx, "select last_be_govern_time from city_schedule where city_id=?", tcid)
	if err != nil {
		return err
	}
	if lastBeGovernTime != 0 {
		// 一天以内被下达过政令则不能下达
		if !(dayBucket(now) > dayBucket(lastBeGovernTime)) {
			return errLegacy("该城池今天已经被征收过了，不能重复下达政令。") // target_has_been_govern
		}
	}
	timeandcount, err := s.db.FetchOne(ctx, "select govern_count,last_govern_time from city_schedule where city_id=?", cid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	todayFirst := false
	todayCount := int64(0)
	if timeandcount == nil {
		todayFirst = true
	} else {
		if dayBucket(now) > dayBucket(model.Int64(timeandcount, "last_govern_time")) {
			todayFirst = true
		} else {
			todayCount = model.Int64(timeandcount, "govern_count")
			if todayCount >= int64(maxCount) {
				return errLegacy(fmt.Sprintf("%s在%s每天可以下达%s次政令，你今天已经下令%s次，不能再次下令了。",
					officename, cityTypeName, itoa(int64(maxCount)), itoa(int64(maxCount))))
			}
		}
	}
	todayCount++
	msg := ""
	switch typ {
	case 0: // 征税
		totalCount, err := s.cellInt(ctx, "select gold from city_resources where city_id=?", tcid)
		if err != nil {
			return err
		}
		addCount := totalCount / 10
		if err := s.addCityResources(ctx, tcid, 0, 0, 0, 0, -addCount); err != nil {
			return err
		}
		report := fmt.Sprintf("%s城（%s,%s）向你征收税赋，你损失黄金%.0f。", cityname, itoa(int64(x)), itoa(int64(y)), float64(addCount))
		if err := s.sendReport(ctx, tuid, 3, 26, cid, tcid, report); err != nil {
			return err
		}
		if err := s.addCityResources(ctx, cid, 0, 0, 0, 0, addCount); err != nil {
			return err
		}
		msg = fmt.Sprintf("收税成功，获得黄金%.0f。", float64(addCount))
	case 1: // 抽丁
		totalCount, err := s.cellInt(ctx, "select people from city_resources where city_id=?", tcid)
		if err != nil {
			return err
		}
		addCount := totalCount / 5
		if err := s.addCityPeople(ctx, tcid, -addCount); err != nil {
			return err
		}
		if err := s.addCityPeople(ctx, cid, addCount); err != nil {
			return err
		}
		report := fmt.Sprintf("%s城（%s,%s）向强抽壮丁，你损失人口%.0f。", cityname, itoa(int64(x)), itoa(int64(y)), float64(addCount))
		if err := s.sendReport(ctx, tuid, 3, 28, cid, tcid, report); err != nil {
			return err
		}
		msg = fmt.Sprintf("抽丁成功，获得人口%.0f。", float64(addCount))
	case 2: // 征粮
		totalCount, err := s.cellInt(ctx, "select food from city_resources where city_id=?", tcid)
		if err != nil {
			return err
		}
		addCount := totalCount / 10
		if err := s.addCityResources(ctx, tcid, 0, 0, 0, -addCount, 0); err != nil {
			return err
		}
		report := fmt.Sprintf("%s城（%s,%s）向你征收粮食，你损失粮食%.0f。", cityname, itoa(int64(x)), itoa(int64(y)), float64(addCount))
		if err := s.sendReport(ctx, tuid, 3, 42, cid, tcid, report); err != nil {
			return err
		}
		if err := s.addCityResources(ctx, cid, 0, 0, 0, addCount, 0); err != nil {
			return err
		}
		msg = fmt.Sprintf("征粮成功，增加粮食%.0f。", float64(addCount))
	case 3: // 收编
		row, err := s.db.FetchOne(ctx,
			"select a.soldier_id as sid,a.count from city_soldiers a,cfg_soldiers b where a.city_id=? and a.soldier_id=b.sid and b.fromcity=1 order by count desc limit 1", tcid)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		sid := 1
		totalCount := int64(0)
		if row != nil {
			sid = model.Int(row, "sid")
			totalCount = model.Int64(row, "count")
		}
		sname, _ := s.cellStr(ctx, "select name from cfg_soldiers where sid=?", sid)
		addCount := totalCount / 50
		if err := s.addCitySoldier(ctx, tcid, sid, -addCount); err != nil {
			return err
		}
		report := fmt.Sprintf("%s城（%s,%s））强行收编你的军队，你损失%s%.0f。", cityname, itoa(int64(x)), itoa(int64(y)), sname, float64(addCount))
		if err := s.sendReport(ctx, tuid, 3, 43, cid, tcid, report); err != nil {
			return err
		}
		meaddcount := addCount / 2
		if err := s.addCitySoldier(ctx, cid, sid, meaddcount); err != nil {
			return err
		}
		msg = fmt.Sprintf("收编成功，增加%s%.0f。", sname, float64(meaddcount))
	case 4: // 裁军
		row, err := s.db.FetchOne(ctx, "select soldier_id as sid,count from city_soldiers where city_id=? order by count desc limit 1", tcid)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		sid := 1
		totalCount := int64(0)
		if row != nil {
			sid = model.Int(row, "sid")
			totalCount = model.Int64(row, "count")
		}
		sname, _ := s.cellStr(ctx, "select name from cfg_soldiers where sid=?", sid)
		addCount := totalCount / 20
		if err := s.addCitySoldier(ctx, tcid, sid, -addCount); err != nil {
			return err
		}
		report := fmt.Sprintf("%s城（%s,%s）勒令你裁军，你损失%s%.0f。", cityname, itoa(int64(x)), itoa(int64(y)), sname, float64(addCount))
		if err := s.sendReport(ctx, tuid, 3, 44, cid, tcid, report); err != nil {
			return err
		}
		msg = fmt.Sprintf("裁军成功，对方减少%s%.0f。", sname, float64(addCount))
	}
	if _, err := s.db.Exec(ctx,
		"insert into city_schedule (city_id,last_be_govern_time) values(?,unix_timestamp()) on duplicate key update last_be_govern_time=unix_timestamp()", tcid); err != nil {
		return err
	}
	if todayFirst {
		if _, err := s.db.Exec(ctx,
			"insert into city_schedule (city_id,govern_count,last_be_govern_time) values(?,?,unix_timestamp()) "+
				"on duplicate key update last_govern_time=unix_timestamp(), govern_count=?", cid, todayCount, todayCount); err != nil {
			return err
		}
	} else {
		if _, err := s.db.Exec(ctx,
			"insert into city_schedule (city_id,govern_count) values(?,?) on duplicate key update govern_count=?", cid, todayCount, todayCount); err != nil {
			return err
		}
	}
	return errLegacy(msg)
}

// ── 16. getMapCity（WorldFunc.php:787）─────────────────────────────────────
func (s *Service) GetMapCity(ctx context.Context, uid int) ([]map[string]any, error) {
	rows, err := s.db.FetchRows(ctx,
		"select c.name,c.id as cid,c.type,u.nickname as ownername,un.name as union_name "+
			"from cities c left join users u on c.user_id=u.id left join unions un on u.union_id=un.id where c.type>1 and c.type<5")
	if err != nil {
		return nil, err
	}
	// 洛阳分支（type==4，getMapCity:794-804）裁剪：M7/M8 已排除洛阳。
	return rows, nil
}

// ── 17. checkCanInvade（WorldFunc.php:810）─────────────────────────────────
// 返回 [true] 或 [false, type, total, invaded]。
func (s *Service) CheckCanInvade(ctx context.Context, uid, cid int) ([]any, error) {
	typ, err := s.cellInt(ctx, "select type from cities where id=?", cid)
	if err != nil {
		return nil, err
	}
	if typ == 5 {
		typ = 0
	}
	if typ < 2 {
		return []any{true}, nil
	}
	unionid, err := s.cellInt(ctx, "select union_id from users where id=?", uid)
	if err != nil {
		return nil, err
	}
	wid := cid2wid(cid)
	place, err := s.db.FetchOne(ctx, "select * from mem_world where wid=? limit 1", wid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	province, _ := strconv.Atoi(model.Str(place, "province"))
	jun, _ := strconv.Atoi(model.Str(place, "jun"))
	var total, invaded int64
	switch typ {
	case 2:
		total, err = s.cellInt(ctx, "select count(*) from cities c left join mem_world w on c.id="+wid2cidExpr+
			" where c.type=1 and w.province=? and w.jun=?", province, jun)
		if err != nil {
			return nil, err
		}
		invaded, err = s.cellInt(ctx, "select count(*) from cities c left join mem_world w on c.id="+wid2cidExpr+
			" left join users u on u.id=c.user_id where c.type=1 and w.province=? and w.jun=? and u.union_id=?", province, jun, unionid)
		if err != nil {
			return nil, err
		}
	case 3:
		total, err = s.cellInt(ctx, "select count(*) from cities c left join mem_world w on c.id="+wid2cidExpr+
			" where c.type=2 and w.province=?", province)
		if err != nil {
			return nil, err
		}
		invaded, err = s.cellInt(ctx, "select count(*) from cities c left join mem_world w on c.id="+wid2cidExpr+
			" left join users u on u.id=c.user_id where c.type=2 and w.province=? and u.union_id=?", province, unionid)
		if err != nil {
			return nil, err
		}
	case 4:
		total, err = s.cellInt(ctx, "select count(*) from cities c where c.type=3")
		if err != nil {
			return nil, err
		}
		invaded, err = s.cellInt(ctx, "select count(*) from cities c left join users u on u.id=c.user_id where c.type=3 and u.union_id=?", unionid)
		if err != nil {
			return nil, err
		}
	}
	// 原版 percent = $invaded/$total；total=0 时 PHP7 为 NAN/INF（保留：0/0→0，>0/0→∞）。
	var percent float64
	if total == 0 {
		if invaded > 0 {
			percent = math.Inf(1)
		}
	} else {
		percent = float64(invaded) / float64(total)
	}
	if percent > 0.33333333 {
		return []any{true}, nil
	}
	return []any{false, int(typ), total, invaded}, nil
}

// ── 18. markCity（WorldFunc.php:853）───────────────────────────────────────
func (s *Service) MarkCity(ctx context.Context, uid, cid int) error {
	city, err := s.db.FetchOne(ctx, "select * from cities where id=? limit 1", cid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if city == nil || model.Int(city, "type") == 0 || model.Int(city, "type") == 5 {
		return errLegacy("只能对名城标记。") // MarkCity.only_for_famous_city
	}
	unionid, err := s.cellInt(ctx, "select union_id from users where id=?", uid)
	if err != nil {
		return err
	}
	if unionid == 0 {
		return errLegacy("你还没有联盟。") // MarkCity.No_Union
	}
	union, err := s.db.FetchOne(ctx, "select * from unions where id=? limit 1", unionid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if union == nil {
		return errLegacy("你还没有联盟。")
	}
	unionpos, err := s.cellInt(ctx, "select union_pos from users where id=?", uid)
	if err != nil {
		return err
	}
	// 原版怪癖：`$unionpos <= 0 || $unionpos <= 3` 等价于 `$unionpos <= 3`（L871）。
	if unionpos <= 3 {
		return errLegacy("你没有权限进行该项操作。") // MarkCity.No_Permission
	}
	owneruid, err := s.cellInt(ctx, "select user_id from cities where id=?", cid)
	if err != nil {
		return err
	}
	if owneruid > 1000 {
		targetunionid, err := s.cellInt(ctx, "select union_id from users where id=?", owneruid)
		if err != nil {
			return err
		}
		if ok, err := s.db.Exists(ctx, "select 1 from union_relations where unionid=? and target=? and type=2", unionid, targetunionid); err != nil {
			return err
		} else if !ok {
			return errLegacy("目标城池所属联盟并未与本盟开战。") // MarkCity.not_in_war
		}
	}
	// 删一下过期的标记
	if err := s.clearMark(ctx, uid); err != nil {
		return err
	}
	if ok, err := s.db.Exists(ctx, "select 1 from union_marks where unionid=? and cid=?", unionid, cid); err != nil {
		return err
	} else if ok {
		return errLegacy("该城已被标记。") // MarkCity.have_marked
	}
	count, err := s.cellInt(ctx, "select count(*) from union_marks where unionid=?", unionid)
	if err != nil {
		return err
	}
	if count >= 20 {
		first, err := s.cellInt(ctx, "select min(endtime) from union_marks where unionid=?", unionid)
		if err != nil {
			return err
		}
		now, err := s.db.Now(ctx)
		if err != nil {
			return err
		}
		return errLegacy(fmt.Sprintf("标记城池的数量上限为20个，本盟对城池的标记已经满足上限，继续标记需要等待其他标记解除。距最近一个标记解除的时间为：%s",
			makeTimeLeft(first-now)))
	}
	wid := cid2wid(cid)
	if _, err := s.db.Exec(ctx,
		"insert into union_marks(`unionid`,`cid`,`endtime`,`type`,`wid`) values(?,?,unix_timestamp()+48*3600,1,?)",
		unionid, cid, wid); err != nil {
		return err
	}
	// 盟主发信通知本方盟内所有成员（sys_mail_content/sys_mail_box 无表 → 不发信）。
	return errLegacy("**标记成功") // MarkCity.mark_succ
}

// ── 19. clearMark（WorldFunc.php:919）──────────────────────────────────────
func (s *Service) clearMark(ctx context.Context, uid int) error {
	unionid, err := s.cellInt(ctx, "select union_id from users where id=?", uid)
	if err != nil {
		return err
	}
	marks, err := s.db.FetchRows(ctx, "select * from union_marks where unionid=?", unionid)
	if err != nil {
		return err
	}
	if len(marks) == 0 {
		return nil
	}
	for _, mark := range marks {
		cid := model.Int(mark, "cid")
		owneruid, err := s.cellInt(ctx, "select user_id from cities where id=?", cid)
		if err != nil {
			return err
		}
		if owneruid > 1000 {
			targetunionid, err := s.cellInt(ctx, "select union_id from users where id=?", owneruid)
			if err != nil {
				return err
			}
			if ok, err := s.db.Exists(ctx, "select 1 from union_relations where unionid=? and target=? and type=2", unionid, targetunionid); err != nil {
				return err
			} else if !ok {
				if _, err := s.db.Exec(ctx, "update union_marks set endtime=0 where id=?", model.Int(mark, "id")); err != nil {
					return err
				}
			}
		}
	}
	_, err = s.db.Exec(ctx, "delete from union_marks where endtime < unix_timestamp()")
	return err
}

// ── 20. getActionField（WorldFunc.php:938）─────────────────────────────────
// 返回 [type, [wid,...]]。
func (s *Service) GetActionField(ctx context.Context, uid, typ int) ([]any, error) {
	results, err := s.db.FetchRows(ctx, "select cid,count,starttime from cfg_special_act")
	if err != nil {
		return nil, err
	}
	tempArr := make([]int, 0)
	if len(results) == 0 {
		tempArr = append(tempArr, -1)
	} else {
		curtime, err := s.db.Now(ctx)
		if err != nil {
			return nil, err
		}
		curdaystartSec, err := s.cellInt(ctx, "select unix_timestamp(curdate())")
		if err != nil {
			return nil, err
		}
		for _, result := range results {
			cid := model.Int(result, "cid")
			count := model.Int(result, "count")
			parts := strings.Split(model.Str(result, "starttime"), ":")
			hh, mm := 0, 0
			if len(parts) > 0 {
				hh, _ = strconv.Atoi(parts[0])
			}
			if len(parts) > 1 {
				mm, _ = strconv.Atoi(parts[1])
			}
			starttimeSec := int64(hh)*3600 + int64(mm)*60 + curdaystartSec
			wid := cid2wid(cid)
			if cid < 1000 || count == 0 || curtime < starttimeSec {
				wid = -1
			}
			tempArr = append(tempArr, wid)
		}
	}
	return []any{typ, tempArr}, nil
}

// ── 21. getUserFields（WorldFunc.php:980）──────────────────────────────────
// 返回 [ rows ]。
func (s *Service) GetUserFields(ctx context.Context, uid int) ([]any, error) {
	rows, err := s.db.FetchRows(ctx,
		"select m.wid from mem_world m,cities s where m.ownercid=s.id and m.type>0 and s.user_id=?", uid)
	if err != nil {
		return nil, err
	}
	return []any{rows}, nil
}

// itoa 整数格式化。
func itoa(v int64) string { return strconv.FormatInt(v, 10) }

// decodeSoldiers 解析 troops.soldiers（新库 JSON map[sid]count）。
func decodeSoldiers(raw string) map[int]int64 {
	out := map[int]int64{}
	if raw == "" {
		return out
	}
	var m map[string]int64
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return out
	}
	for k, v := range m {
		if sid, err := strconv.Atoi(k); err == nil {
			out[sid] = v
		}
	}
	return out
}

// startWar 文案（lang.php:642/643，逐字）。
const startWarSuccReport = "你对%s宣战。<br/>宣战8小时后正式进入战争状态。<br/>战争期间双方可以互相掠夺、占领对方的城池。<br/>战争持续48小时后自动结束。<br/>战争开始时间：%s。<br/>战争结束时间：%s。<br/>你可以使用道具，提升将领和军队的作战能力，使他们能更有效的消灭敌人。<br/>“虎符”增加将领的统率，“武曲星符”增加将领的攻击，“智多星符”增加将领的防御，<br/>“青囊书”增加军队伤兵的恢复数量，“陷阵战鼓”增加军队攻击力，“八卦阵图”增加军队防御力。<br/>战后别忘记到“校场”的“伤兵营”恢复伤兵。"

const startWarSuccCaution = "%s对你宣战。<br/>宣战8小时后正式进入战争状态。<br/>战争期间双方可以互相掠夺、占领对方的城池。<br/>战争持续48小时后自动结束。<br/>战争开始时间：%s。<br/>战争结束时间：%s。<br/>你可以使用道具，提升将领和军队的作战能力，使他们能更有效的消灭敌人。<br/>。“虎符”增加将领的统率，“武曲星符”增加将领的攻击，“智多星符”增加将领的防御，<br/>“青囊书”增加军队伤兵的恢复数量，“陷阵战鼓”增加军队攻击力，“八卦阵图”增加军队防御力。<br/>战后别忘记到“校场”的“伤兵营”恢复伤兵。<br/>使用“免战牌”在一段时间内避免被攻击，使用“迁城令”远离你的敌人。如果敌人太强大，你可以选择不出战，避免军队损失。或者联系你的盟友来协助你防守。"

// ── 24. unionAssit（WorldFunc.php:988）→ 裁剪（联盟，M8 整体排除）──────────
