package battle

// result.go —— 战斗结束结算，1:1 对照 getBattleResult（OLdBattleCron.php:790-960）
// 与结束分支（OLdBattleCron.php:487-529 / 1022-1615）。
//
// 已实现：伤兵/俘虏率、青囊加成（user_buffers 9/165/10083）、将领体力与装备耐久、将领经验、
//   君主声望、掠夺(add_resource 负重口径)、野地占领(owner_uid)、部队回程/驻守、战报(reports 表)。
//
// 声明性裁剪（新库无对应系统，与 M8 社交/战场排除一致）：
//   - 联盟战报(sys_union_report)、战场(battlefieldid/sys_battle_*)、洛阳(sys_luoyang_*) 不实现。
//   - 史诗任务物品(get_m_usertask_goods)、任务完成(completeTask)、告示(sys_inform)、
//     讨将(catchHero)/武将突围(throwUsersHeroField)、城池迁移、金币掉落(getOpenDefaultGoodsResult) 不实现。
//   - mem_world 野地上限(checkFeildCount)：新库无野地上限配置，占领恒成功（见 occupyField）。

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"strconv"
)

// 部队状态（与 army 包一致：0出征 1返程 4驻守）。
const (
	troopReturn   = 1
	troopGarrison = 4
)

// gains 是战报「战斗收获」区所需数值（对齐 $prestige3/$exp1/$a_wounded）。
type gains struct {
	Prestige float64
	Exp      float64
	Wounded  float64 // 伤兵比例（百分比 = a_wounded*100）
}

// finishBattle 对齐战斗结束处理：结算战果、写 sys_battle.state/result、战报。
func (s *Service) finishBattle(ctx context.Context, b *Battle, end int) error {
	result := 3
	switch end {
	case 0:
		result = 0
	case 1:
		result = 1
	default:
		result = 2
	}
	if _, err := s.db.Exec(ctx, "update battles set state=1, result=? where id=?", result, b.ID); err != nil {
		return err
	}

	mtroop, err := s.bakTroop(ctx, b.ID, b.AttackUID)
	if err != nil {
		return err
	}
	ytroop, err := s.bakTroop(ctx, b.ID, b.ResistUID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	g, err := s.settleBattleResult(ctx, b, end, mtroop, ytroop)
	if err != nil {
		return err
	}
	if err := s.applyOutcome(ctx, b, end, mtroop, ytroop); err != nil {
		return err
	}
	if err := s.writeBattleReport(ctx, b, end, mtroop, ytroop, g); err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx, "delete from bak_troops where battleid=?", b.ID); err != nil {
		return err
	}
	return nil
}

func (s *Service) bakTroop(ctx context.Context, battleID, uid int) (map[string]any, error) {
	return s.db.FetchOne(ctx, "select * from bak_troops where battleid=? and uid=? limit 1", battleID, uid)
}

// settleBattleResult 对齐 getBattleResult：伤兵/俘虏/体力/装备/经验/声望。
func (s *Service) settleBattleResult(ctx context.Context, b *Battle, end int, mtroop, ytroop map[string]any) (*gains, error) {
	if mtroop == nil {
		return &gains{}, nil
	}
	// 更新 troops.soldiers = 最新存活（新库 troops.soldiers 为 army 的 JSON 口径，需转换）。
	if _, err := s.db.Exec(ctx, "update troops set soldiers=? where id=?", phpToJSONSoldiers(b.AttackSoldiers), modelInt64(mtroop, "id")); err != nil {
		return nil, err
	}
	if ytroop != nil {
		if _, err := s.db.Exec(ctx, "update troops set soldiers=? where id=?", phpToJSONSoldiers(b.ResistSoldiers), modelInt64(ytroop, "id")); err != nil {
			return nil, err
		}
	}

	actArray := troop2Array(modelStr(mtroop, "soldiers"))
	defArray := troop2Array(modelStr(ytroop, "soldiers"))
	overAct := troop2Array(b.AttackSoldiers)
	overDef := troop2Array(b.ResistSoldiers)
	aArray := getArraySub(actArray, overAct)
	dArray := getArraySub(defArray, overDef)

	uid, resistuid := b.AttackUID, b.ResistUID
	cid, resistcid := b.AttackCID, b.ResistCID
	hid, resisthid := b.AttackHID, b.ResistHID

	dValue := sol2Value(dArray)
	aValue := sol2Value(aArray)
	acount := array2count(actArray)
	dcount := array2count(defArray)
	adiecount := array2count(aArray)
	ddiecount := array2count(dArray)

	user1, err := s.userRow(ctx, uid)
	if err != nil {
		return nil, err
	}
	user2, err := s.userRow(ctx, resistuid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	aWounded := 0.19
	if modelInt64(user1, "state") == 1 {
		aWounded = 0.8
	}
	dWounded := 0.19
	if modelInt64(user2, "state") == 1 {
		dWounded = 0.8
	}
	aCapture := 0.15
	if modelInt64(user1, "state") == 1 {
		aCapture = 0.32
	}

	// 青囊加成（mem_user_buffer buftype 9/165/10083 → user_buffers）。
	rows, err := s.db.FetchRows(ctx,
		"select user_id, buftype from user_buffers where buftype in (9,165,10083) and user_id in (?,?) and endtime>unix_timestamp()",
		uid, resistuid)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if int(modelInt64(r, "user_id")) == uid {
			if modelInt64(r, "buftype") == 9 {
				aWounded += 0.3
				aCapture += 0.12
			} else {
				aWounded += 0.6
				aCapture += 0.18
			}
			if aWounded > 0.8 {
				aWounded = 0.8
			}
			if aCapture > 0.5 {
				aCapture = 0.5
			}
		} else if int(modelInt64(r, "user_id")) == resistuid {
			if modelInt64(r, "buftype") == 9 {
				dWounded += 0.3
			} else {
				dWounded += 0.6
			}
			if dWounded > 0.8 {
				dWounded = 0.8
			}
		}
	}
	_ = aCapture

	// 攻方/守方伤兵入城（type<4）。
	if s.isPlayer(ctx, uid) && b.Type < 4 {
		w := newOMap()
		for _, sid := range aArray.keys {
			if cnt := math.Floor(aArray.get(sid) * aWounded); cnt > 0 {
				w.set(sid, cnt)
			}
		}
		if w.len() > 0 {
			if err := s.addCityWounded(ctx, cid, w); err != nil {
				return nil, err
			}
		}
	}
	if s.isPlayer(ctx, resistuid) && b.Type < 4 {
		w := newOMap()
		for _, sid := range dArray.keys {
			if cnt := math.Floor(dArray.get(sid) * dWounded); cnt > 0 {
				w.set(sid, cnt)
			}
		}
		if w.len() > 0 {
			if err := s.addCityWounded(ctx, resistcid, w); err != nil {
				return nil, err
			}
		}
	}

	// 将领体力 / 装备耐久（L922-933）。
	aLost := 0.0
	if adiecount > 0 && acount > 0 {
		aLost = math.Floor((adiecount * 100) / acount)
	}
	dLost := 0.0
	if ddiecount > 0 && dcount > 0 {
		dLost = math.Floor((ddiecount * 100) / dcount)
	}
	if aLost > 0 && s.isPlayer(ctx, uid) && hid > 0 {
		if _, err := s.db.Exec(ctx, "update hero_blood set force=greatest(0,force-?) where hero_id=?", aLost, hid); err != nil {
			return nil, err
		}
		if _, err := s.db.Exec(ctx, "update user_armors set hp=greatest(0,hp-?) where user_id=? and hid=?", math.Round(aLost), uid, hid); err != nil {
			return nil, err
		}
	}
	if dLost > 0 && s.isPlayer(ctx, resistuid) && resisthid > 0 {
		if _, err := s.db.Exec(ctx, "update hero_blood set force=greatest(0,force-?) where hero_id=?", dLost, resisthid); err != nil {
			return nil, err
		}
		if _, err := s.db.Exec(ctx, "update user_armors set hp=greatest(0,hp-?) where user_id=? and hid=?", math.Round(dLost), resistuid, resisthid); err != nil {
			return nil, err
		}
	}

	// 将领经验（L934-960）。
	exp1 := s.heroBattleExp(ctx, uid, hid, dValue)
	s.heroBattleExp(ctx, resistuid, resisthid, aValue)

	// 君主声望（L961-987）。
	prestige1 := modelFloat(user1, "prestige")
	if prestige1 <= 0 {
		prestige1 = 1
	}
	apeople := sol2people(aArray)
	dpeople := sol2people(dArray)
	prestige3 := 0.0
	if !s.isPlayer(ctx, resistuid) {
		prestige3 = math.Floor((dpeople - apeople) * math.Min(1, dpeople/prestige1))
		if prestige3+prestige1 < 0 {
			prestige3 = 0
		}
	} else {
		prestige2 := modelFloat(user2, "prestige")
		if prestige2 <= 0 {
			prestige2 = 1
		}
		prestige3 = math.Floor(((dpeople - apeople) * (1 + math.Min(prestige1/prestige2, 2))) / 10)
		prestige4 := -prestige3
		if prestige3+prestige1 < 0 {
			prestige3 = 0
		}
		if prestige4+prestige2 < 0 {
			prestige4 = 0
		}
		if err := s.addUserPrestige(ctx, resistuid, prestige4); err != nil {
			return nil, err
		}
	}
	if err := s.addUserPrestige(ctx, uid, prestige3); err != nil {
		return nil, err
	}

	// 战功声望（L1616-1620）。
	if s.isPlayer(ctx, resistuid) {
		if s.isPlayer(ctx, uid) {
			if _, err := s.db.Exec(ctx, "update users set war_attack_prestige=war_attack_prestige+? where id=?", dpeople, uid); err != nil {
				return nil, err
			}
		}
		if _, err := s.db.Exec(ctx, "update users set war_defence_prestige=war_defence_prestige+? where id=?", apeople, resistuid); err != nil {
			return nil, err
		}
	}
	_ = end
	return &gains{Prestige: prestige3, Exp: exp1, Wounded: aWounded * 100}, nil
}

// heroBattleExp 对齐 L934-960：按击杀价值 floor(value/100) 与升级所需取随机经验，写 heroes.exp。
// 新库简化：addHeroExp 的升级/突破流程由 hero 模块惰性处理，这里只累加 exp（herotype==1000 君主将不得经验）。
func (s *Service) heroBattleExp(ctx context.Context, uid, hid int, value float64) float64 {
	if !s.isPlayer(ctx, uid) || hid <= 0 {
		return 0
	}
	level, err := s.db.FetchCellInt64(ctx, "select level from heroes where id=?", hid)
	if err != nil {
		return 0
	}
	need, err := s.db.FetchCellInt64(ctx, "select upgrade_exp from cfg_hero_levels where level=?", level)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0
	}
	if level == 1 {
		need = 1000
	}
	mine := math.Floor(value / 100)
	var exp float64
	if mine > float64(need) {
		exp = float64(phpMtRand(int(need), int(mine)))
	} else {
		exp = float64(phpMtRand(int(mine), int(need)))
	}
	htype, _ := s.db.FetchCellInt64(ctx, "select hero_type from heroes where id=?", hid)
	if htype == 1000 {
		exp = 0
	}
	if exp > 0 {
		_, _ = s.db.Exec(ctx, "update heroes set exp=exp+? where id=?", exp, hid)
	}
	return exp
}

// addUserPrestige 对齐 cfg_js.php:1386：warprestige 与 prestige 同时 +GREATEST(0,value)。
func (s *Service) addUserPrestige(ctx context.Context, uid int, value float64) error {
	if uid <= 0 {
		return nil
	}
	add := math.Max(0, value)
	_, err := s.db.Exec(ctx,
		"update users set warprestige=warprestige+?, prestige=prestige+? where id=?", add, add, uid)
	return err
}

// addCityWounded 对齐 cfg_js.php:1372 addCityWounded（+ 分支）。
func (s *Service) addCityWounded(ctx context.Context, cid int, soldiers *oMap) error {
	for _, sid := range soldiers.keys {
		if _, err := s.db.Exec(ctx,
			"insert into city_wounded (city_id, soldier_id, count) values (?,?,?) "+
				"on duplicate key update count=greatest(0,count+values(count))",
			cid, sid, soldiers.get(sid)); err != nil {
			return err
		}
	}
	return nil
}

// applyOutcome 对齐结束分支：部队回程/驻守、掠夺资源、野地占领。
func (s *Service) applyOutcome(ctx context.Context, b *Battle, end int, mtroop, ytroop map[string]any) error {
	if mtroop == nil {
		return nil
	}
	_ = ytroop
	troopID := modelInt64(mtroop, "id")
	now, err := s.db.Now(ctx)
	if err != nil {
		return err
	}
	troop, err := s.db.FetchOne(ctx, "select * from troops where id=?", troopID)
	if errors.Is(err, sql.ErrNoRows) || troop == nil {
		return nil
	}
	targetType := int(modelInt64(troop, "target_type"))
	targetID := int(modelInt64(troop, "target_id"))
	task := int(modelInt64(troop, "task"))

	switch end {
	case 0: // 攻方胜
		if task == 4 && targetType == 1 { // 占领野地 → 驻守
			return s.occupyField(ctx, troop, targetID)
		}
		if task == 3 { // 掠夺
			if err := s.plunder(ctx, int(modelInt64(mtroop, "cid")), targetType, targetID, b); err != nil {
				return err
			}
		}
		return s.sendTroopBack(ctx, int(troopID), now)
	case 1: // 攻方败
		if _, err := s.db.Exec(ctx, "delete from troops where id=?", troopID); err != nil {
			return err
		}
		if b.AttackHID > 0 {
			if _, err := s.db.Exec(ctx, "update heroes set state=0 where id=?", b.AttackHID); err != nil {
				return err
			}
		}
		return nil
	default: // 平局/未知
		return s.sendTroopBack(ctx, int(troopID), now)
	}
}

// sendTroopBack 让进攻部队返程（state=1，回程时间 = 已行进时间；对齐 army.Recall 口径）。
func (s *Service) sendTroopBack(ctx context.Context, troopID int, now int64) error {
	startAt, err := s.db.FetchCellInt64(ctx, "select start_at from troops where id=?", troopID)
	if err != nil {
		return err
	}
	back := now
	if startAt > 0 && now > startAt {
		back = now + (now - startAt)
	}
	_, err = s.db.Exec(ctx,
		"update troops set state=?, start_at=?, back_at=?, arrive_at=0 where id=?", troopReturn, now, back, troopID)
	return err
}

// occupyField 新库野地占领：设 owner_uid=进攻方，部队转为驻守。
// 声明：legacy checkFeildCount（野地上限）在新库无配置，恒视为可占领。
func (s *Service) occupyField(ctx context.Context, troop map[string]any, fieldID int) error {
	uid := modelInt64(troop, "user_id")
	if _, err := s.db.Exec(ctx, "update fields set owner_uid=? where id=?", uid, fieldID); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx,
		"update troops set state=?, battleid=0, arrive_at=0 where id=?", troopGarrison, modelInt64(troop, "id"))
	return err
}

// plunder 掠夺：add_resource 负重口径（allcarry = getCrray(over_actArray, 科技11)）。
func (s *Service) plunder(ctx context.Context, cid, targetType, targetID int, b *Battle) error {
	rob := ""
	switch targetType {
	case 1:
		row, err := s.db.FetchOne(ctx,
			"select loot_gold, loot_food, loot_wood, loot_rock, loot_iron from fields where id=? limit 1", targetID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		rob = phpStr(modelFloat(row, "loot_gold")) + "," + phpStr(modelFloat(row, "loot_food")) + "," +
			phpStr(modelFloat(row, "loot_wood")) + "," + phpStr(modelFloat(row, "loot_rock")) + "," +
			phpStr(modelFloat(row, "loot_iron"))
	default:
		g, ok, err := s.getCityResource(ctx, targetID, 1)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		rob = g
	}
	carryTech, err := s.techLevel(ctx, cid, 11)
	if err != nil {
		return err
	}
	allcarry := getCrray(troop2Array(b.AttackSoldiers), float64(carryTech))
	_, robStr := addResource("0,0,0,0,0,", rob, allcarry)
	robArray, ok := checkResource(robStr)
	if !ok {
		return nil
	}
	if _, err := s.db.Exec(ctx,
		"update city_resources set gold=gold+?, food=food+?, wood=wood+?, rock=rock+?, iron=iron+? where city_id=?",
		robArray.gold, robArray.food, robArray.wood, robArray.rock, robArray.iron, cid); err != nil {
		return err
	}
	if targetType == 2 {
		if _, err := s.db.Exec(ctx,
			"update city_resources set gold=greatest(0,gold-?), food=greatest(0,food-?), wood=greatest(0,wood-?), rock=greatest(0,rock-?), iron=greatest(0,iron-?) where city_id=?",
			robArray.gold, robArray.food, robArray.wood, robArray.rock, robArray.iron, targetID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) userRow(ctx context.Context, uid int) (map[string]any, error) {
	return s.db.FetchOne(ctx, "select * from users where id=? limit 1", uid)
}

// sol2people 对齐 cfg_js.php:1410：Σ cnt*usepeople[sid]。
func sol2people(o *oMap) float64 {
	t := 0.0
	for _, sid := range o.keys {
		t += o.get(sid) * usePeopleAt(sid)
	}
	return t
}

// writeBattleReport 组装战报并写入 reports（legacy sendReport），复用 lang.go 模板。
func (s *Service) writeBattleReport(ctx context.Context, b *Battle, end int, mtroop, ytroop map[string]any, g *gains) error {
	if mtroop == nil {
		return nil
	}
	task := int(modelInt64(mtroop, "task"))
	myw, youw := "胜利", "失败"
	if end == 0 {
		myw, youw = "失败", "胜利"
	} else if end == 2 || end == 3 {
		myw, youw = "平局", "平局"
	}
	myName, _ := s.db.FetchCellString(ctx, "select nickname from users where id=?", b.AttackUID)
	youName, _ := s.db.FetchCellString(ctx, "select nickname from users where id=?", b.ResistUID)
	mHero, _ := s.getContentBattleHero(ctx, b.AttackHID)
	yHero, _ := s.getContentBattleHero(ctx, b.ResistHID)

	content := sprintf(tplBtitles,
		strconv.Itoa(b.Round), myw, myName, myw, youName, youw, mHero+yHero)

	// 双方军情（armynumss：兵种/数量/损失）。
	myArmy := armyList(mtroop, b.AttackSoldiers)
	youArmy := armyList(ytroop, b.ResistSoldiers)
	content += sprintf(tplBadtroopss, "我方军情", myArmy)
	content += sprintf(tplBadtroopss, "敌方军情", youArmy)

	// 战斗收获（声望/经验/伤兵比例）。
	if g == nil {
		g = &gains{}
	}
	content += sprintf(tplDetect, "战斗收获")
	content += sprintf(tplGet, phpStr(g.Prestige), phpStr(g.Exp), phpStr(g.Wounded))
	content += tplTitleEnd

	// 结束语。
	if end == 1 {
		content += "<br/> 我军全军覆没，没有军队返回！"
	} else {
		content += "<br/>军队正在返回。"
	}

	title := task
	stype := 3
	if title <= 11 {
		stype = 0
	} else if title >= 12 && title <= 14 {
		stype = 1
	} else if title == 19 {
		stype = 2
	}
	res, err := s.db.Exec(ctx,
		"insert into reports (user_id, origincid, origincity, happencid, happencity, title, `type`, `time`, `read`, battleid, content) "+
			"values (?,?,?,?,?,?,?,unix_timestamp(),0,?,?)",
		b.AttackUID, b.AttackCID, "", b.ResistCID, "", title, stype, b.ID, content)
	if err != nil {
		return err
	}
	_ = res
	_, err = s.db.Exec(ctx, "insert into alarms (user_id, report) values (?,1) on duplicate key update report=1", b.AttackUID)
	return err
}

// armyList 生成 armynumss 行串：兵种名/出征数/损失数。
func armyList(troop map[string]any, over string) string {
	orig := troop2Array(modelStr(troop, "soldiers"))
	left := troop2Array(over)
	var sb string
	for _, sid := range orig.keys {
		lost := orig.get(sid) - left.get(sid)
		if lost < 0 {
			lost = 0
		}
		sb += sprintf(tplArmyNumss, patrolReportSoldier[sid], phpStr(orig.get(sid)), phpStr(lost))
	}
	return sb
}
