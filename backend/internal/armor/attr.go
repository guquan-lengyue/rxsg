package armor

import (
	"context"
	"math"
	"strings"

	"rxsg/backend/internal/model"
)

// attr.go 复刻 legacy HeroFunc.php regenerateHeroAttri(1397) 全链：
//   sortArmorAttr(1315) / sortmyArmorAttr(1353) / warmBloodGoldSpearSpecial(1388) /
//   hasHeroBuffer(1830) / checkHeroLevel(9,80)（降级恒 false）。
// 降级（缺表）：
//   - mem_hero_buffer 无表 → hasHeroBuffer 恒 false（虎符/马鞭/buftype3/4 ×1.25 全跳过）；
//   - sys_user_book/cfg_book 无表 → 技能书加成恒 0；
//   - sys_user_level 无表 → checkHeroLevel(9,80) 恒 false（速度+10 不触发）；
//   - sys_armor_addon 空表 → 末日之刃特效跳过；
//   - mem_hero_blood 无表 → force_max/energy_max 写回省略。

// typeVal 有序属性对（type→value），保留 PHP 数组插入序（map 迭代无序，必须用 slice）。
type typeVal struct {
	t, v int
}

// sortArmorAttr 对齐 HeroFunc.php:1315。
// 校验：首元素×2+1==len；按 value 降序冒泡（稳定：仅 < 时交换）；结果按序去重（后写覆盖 → 同 type 取最后即最小值）。
func sortArmorAttr(attribute string) []typeVal {
	attrs := strings.Split(attribute, ",")
	n := len(attrs)
	if n == 0 || atoi(attrs[0])*2+1 != n {
		return nil
	}
	count := atoi(attrs[0])
	types := make([]int, 0, count)
	values := make([]int, 0, count)
	for i := 1; i < n; i += 2 {
		types = append(types, atoi(attrs[i]))
		values = append(values, atoi(attrs[i+1]))
	}
	// 冒泡（稳定：仅 < 时交换）
	for i := 0; i < count; i++ {
		for j := count - 1; j >= i+1; j-- {
			if values[j-1] < values[j] {
				types[j-1], types[j] = types[j], types[j-1]
				values[j-1], values[j] = values[j], values[j-1]
			}
		}
	}
	// 结果 map（同 type 后写覆盖 → 取最小值）
	seen := map[int]int{}
	order := make([]int, 0, count)
	for i := 0; i < count; i++ {
		t := types[i]
		if _, ok := seen[t]; !ok {
			order = append(order, t)
		}
		seen[t] = values[i]
	}
	out := make([]typeVal, 0, len(order))
	for _, t := range order {
		out = append(out, typeVal{t: t, v: seen[t]})
	}
	return out
}

// sortmyArmorAttr 对齐 HeroFunc.php:1353（armorid>53028 特殊装备，不排序，键为原 index）。
func sortmyArmorAttr(attribute string) []typeVal {
	attrs := strings.Split(attribute, ",")
	n := len(attrs)
	if n == 0 || atoi(attrs[0])*2+1 != n {
		return nil
	}
	out := make([]typeVal, 0, n/2)
	for i := 1; i < n; i += 2 {
		out = append(out, typeVal{t: i, v: atoi(attrs[i+1])})
	}
	return out
}

// applyTypeAdd 按 type 累加对应属性（1统/2政/3勇/4智/5体/6精/8攻/9防/11速）。
func applyTypeAdd(t, v int, cmd, aff, bra, wis, frc, eng, spd, atk, def *int) {
	switch t {
	case 1:
		*cmd += v
	case 2:
		*aff += v
	case 3:
		*bra += v
	case 4:
		*wis += v
	case 5:
		*frc += v
	case 6:
		*eng += v
	case 8:
		*atk += v
	case 9:
		*def += v
	case 11:
		*spd += v
	}
}

// regenerateHeroAttri 对齐 HeroFunc.php:1397（属性重算主链）。
// 返回 *EquipResult 供穿戴/卸下接口复用。
func (s *Service) regenerateHeroAttri(ctx context.Context, uid, hid int) (*EquipResult, error) {
	hero, err := s.fetchOneOrNil(ctx, "select * from heroes where id=? and user_id=? limit 1", hid, uid)
	if err != nil {
		return nil, err
	}
	if hero == nil {
		return nil, errf(msgNoHero)
	}
	// 清空 hero_attributes
	if _, err := s.db.Exec(ctx, "delete from hero_attributes where hid=?", hid); err != nil {
		return nil, err
	}

	armors, err := s.db.FetchRows(ctx, `select u.*, h.spart, h.armorid as h_armorid, c.attribute as c_attribute, c.tieid
		from user_armors u left join hero_armors h on h.hid=u.hid and h.sid=u.sid
		left join cfg_armor c on c.id=h.armorid
		where u.user_id=? and u.hid=? and u.hp>0`, uid, hid)
	if err != nil {
		return nil, err
	}

	level := model.Int(hero, "level")
	command := level + model.Int(hero, "command_base")
	affairs := model.Int(hero, "affairs_base") + model.Int(hero, "affairs_add")
	bravery := model.Int(hero, "bravery_base") + model.Int(hero, "bravery_add")
	wisdom := model.Int(hero, "wisdom_base") + model.Int(hero, "wisdom_add")

	commandAdd, forceAdd, energyAdd := 0, 0, 0
	affairsAdd, braveryAdd, wisdomAdd := 0, 0, 0
	speedAdd, attackAdd, defenceAdd := 0, 0, 0
	// rangeAdd 未使用（legacy 亦未写回）

	for _, armor := range armors {
		// ① 熔炼加成（cfg_armor_level_attr.attr 按 combine_level）
		if cl := model.Int(armor, "combine_level"); cl > 0 {
			ca, err := cellStr(ctx, s.db, "select attr from cfg_armor_level_attr where level=? limit 1", cl)
			if err != nil {
				return nil, err
			}
			if ca != "" {
				parts := strings.Split(ca, ",")
				if len(parts) > 0 && atoi(parts[0])*2+1 == len(parts) {
					for i := 1; i < len(parts); i += 2 {
						applyTypeAdd(atoi(parts[i]), atoi(parts[i+1]),
							&commandAdd, &affairsAdd, &braveryAdd, &wisdomAdd,
							&forceAdd, &energyAdd, &speedAdd, &attackAdd, &defenceAdd)
					}
				}
			}
		}
		// ② 装备基础 attribute 串（← cfg_armor.attribute，join c.* 覆盖）
		attrStr := model.Str(armor, "c_attribute")
		parts := strings.Split(attrStr, ",")
		if atoi(parts[0])*2+1 != len(parts) {
			continue // legacy：attribute 串非法 → 跳过该件后续（强化/镶嵌/神化）
		}
		for i := 1; i < len(parts); i += 2 {
			applyTypeAdd(atoi(parts[i]), atoi(parts[i+1]),
				&commandAdd, &affairsAdd, &braveryAdd, &wisdomAdd,
				&forceAdd, &energyAdd, &speedAdd, &attackAdd, &defenceAdd)
		}
		// ③ 强化加成
		armorid := model.Int(armor, "armorid")
		var sattr []typeVal
		if armorid > 53028 {
			sattr = sortmyArmorAttr(attrStr)
		} else {
			sattr = sortArmorAttr(attrStr)
		}
		strongLevel := model.Int(armor, "strong_level")
		for i := 1; i <= strongLevel; i++ {
			sv := 0
			if i <= 15 {
				sv = levelvalue[i]
			}
			for sv > 0 {
				for _, tv := range sattr {
					if sv <= 0 {
						break
					}
					applyTypeAdd(tv.t, 1,
						&commandAdd, &affairsAdd, &braveryAdd, &wisdomAdd,
						&forceAdd, &energyAdd, &speedAdd, &attackAdd, &defenceAdd)
					sv--
				}
			}
		}
		// ④ 镶嵌加成（cfg_goods.attr 按 embed_pearls 逐对）
		embed := model.Str(armor, "embed_pearls")
		if embed != "" {
			for _, gs := range strings.Split(embed, ",") {
				gid := atoi(gs)
				if gid == 0 {
					continue
				}
				ea, err := cellStr(ctx, s.db, "select attr from cfg_goods where gid=? limit 1", gid)
				if err != nil {
					return nil, err
				}
				if ea == "" {
					continue
				}
				ep := strings.Split(ea, ",")
				for i := 0; i < len(ep); i += 2 {
					if i+1 >= len(ep) {
						break
					}
					applyTypeAdd(atoi(ep[i]), atoi(ep[i+1]),
						&commandAdd, &affairsAdd, &braveryAdd, &wisdomAdd,
						&forceAdd, &energyAdd, &speedAdd, &attackAdd, &defenceAdd)
				}
			}
		}
		// ⑤ 套装神化（sys_user_tie_deify_attribute 仅取第 1 行；空表 → 跳过）
		sid := model.Int(armor, "sid")
		deify, err := s.fetchOneOrNil(ctx, `select tda.attid, tda.value, ca.type
			from user_tie_deify_attribute tda left join cfg_attribute ca on ca.attid=tda.attid
			where tda.sid=? limit 1`, sid)
		if err != nil {
			return nil, err
		}
		if deify != nil {
			attid := model.Int(deify, "attid")
			val := model.Int(deify, "value")
			if _, err := s.db.Exec(ctx, `insert into hero_attributes (hid, attid, value) values (?,?,?)
				on duplicate key update value=value+?`, hid, attid, val, val); err != nil {
				return nil, err
			}
			applyTypeAdd(model.Int(deify, "type"), val,
				&commandAdd, &affairsAdd, &braveryAdd, &wisdomAdd,
				&forceAdd, &energyAdd, &speedAdd, &attackAdd, &defenceAdd)
		}
	}

	// ⑥ 套装属性（cfg_tie_attribute precond<=穿戴件数）
	tieids, err := s.db.FetchRows(ctx, `select a.tieid from hero_armors ha
		left join cfg_armor a on a.id=ha.armorid where ha.hid=? group by a.tieid`, hid)
	if err != nil {
		return nil, err
	}
	for _, tr := range tieids {
		tieid := model.Int(tr, "tieid")
		if tieid == 0 {
			continue
		}
		cnt, err := cellInt(ctx, s.db, `select count(*) from hero_armors ha
			left join cfg_armor a on a.id=ha.armorid
			left join user_armors ua on ua.sid=ha.sid
			where ha.hid=? and a.tieid=? and ua.hp>0 and ua.user_id=?`, hid, tieid, uid)
		if err != nil {
			return nil, err
		}
		attrs, err := s.db.FetchRows(ctx, `select ta.attid, ta.value, ca.type
			from cfg_tie_attribute ta left join cfg_attribute ca on ca.attid=ta.attid
			where ta.precond<=? and ta.tieid=?`, int(cnt), tieid)
		if err != nil {
			return nil, err
		}
		for _, ar := range attrs {
			val := model.Int(ar, "value")
			attid := model.Int(ar, "attid")
			if _, err := s.db.Exec(ctx, `insert into hero_attributes (hid, attid, value) values (?,?,?)
				on duplicate key update value=value+?`, hid, attid, val, val); err != nil {
				return nil, err
			}
			applyTypeAdd(model.Int(ar, "type"), val,
				&commandAdd, &affairsAdd, &braveryAdd, &wisdomAdd,
				&forceAdd, &energyAdd, &speedAdd, &attackAdd, &defenceAdd)
		}
	}

	// ⑦ cfg_armor_attribute（按穿戴中装备的 armorid 关联）
	newAttrs, err := s.db.FetchRows(ctx, `select aa.attid, aa.value, ca.type
		from cfg_armor_attribute aa left join user_armors ua on ua.armorid=aa.armorid
		left join cfg_attribute ca on ca.attid=aa.attid
		where ua.user_id=? and ua.hid=? and ua.hp>0`, uid, hid)
	if err != nil {
		return nil, err
	}
	for _, ar := range newAttrs {
		val := model.Int(ar, "value")
		attid := model.Int(ar, "attid")
		if _, err := s.db.Exec(ctx, `insert into hero_attributes (hid, attid, value) values (?,?,?)
			on duplicate key update value=value+?`, hid, attid, val, val); err != nil {
			return nil, err
		}
		applyTypeAdd(model.Int(ar, "type"), val,
			&commandAdd, &affairsAdd, &braveryAdd, &wisdomAdd,
			&forceAdd, &energyAdd, &speedAdd, &attackAdd, &defenceAdd)
	}

	// ⑧ 百分比属性（10001统/10002政/10003勇/10004智/10005力/10006精/10008攻×bravery×10/10009防×wisdom×10）
	pctVals := map[int]int{}
	for _, attid := range []int{10001, 10002, 10003, 10004, 10005, 10006, 10008, 10009} {
		v, err := cellInt(ctx, s.db, "select value from hero_attributes where attid=? and hid=? limit 1", attid, hid)
		if err != nil {
			return nil, err
		}
		pctVals[attid] = int(v)
	}
	commandAdd += int(math.Floor(float64(command) * float64(pctVals[10001]) / 100))
	affairsAdd += int(math.Floor(float64(affairs) * float64(pctVals[10002]) / 100))
	braveryAdd += int(math.Floor(float64(bravery) * float64(pctVals[10003]) / 100))
	wisdomAdd += int(math.Floor(float64(wisdom) * float64(pctVals[10004]) / 100))
	attackAdd += int(math.Floor(float64(bravery) * 10 * float64(pctVals[10008]) / 100))
	defenceAdd += int(math.Floor(float64(wisdom) * 10 * float64(pctVals[10009]) / 100))

	// ⑨ 虎符（hasHeroBuffer 恒 false → 跳过）
	// ⑩ buftype3/4 ×1.25（mem_hero_buffer 无表 → 跳过）
	// ⑪ 技能书（sys_user_book 无表 → 跳过）
	// ⑫ 马鞭（hasHeroBuffer 恒 false → 跳过）
	// ⑬ checkHeroLevel(9,80) 恒 false → 速度+10 不触发

	// ⑭ forcemax / energymax
	forcemax := 100 + level/5 + bravery/3
	energymax := 100 + level/5 + wisdom/3
	forcePct := pctVals[10005]
	energyPct := pctVals[10006]
	forceAdd += int(math.Floor(float64(forcemax) * float64(forcePct) / 100))
	energyAdd += int(math.Floor(float64(energymax) * float64(energyPct) / 100))
	forcemax += int(math.Floor(float64(braveryAdd+bravery%3)/3)) + forceAdd
	energymax += int(math.Floor(float64(wisdomAdd+wisdom%3)/3)) + energyAdd

	// ⑮ 热血金枪特效（≥2 件 12018 active_special=1 → speed+1）
	speedAdd += s.warmBloodGoldSpearSpecial(ctx, hid)

	// ⑯ 末日之刃（sys_armor_addon 空表 → 跳过）

	// ⑰ 写回 heroes
	if _, err := s.db.Exec(ctx, `update heroes set command_add_on=?, affairs_add_on=?, bravery_add_on=?, wisdom_add_on=?,
		force_max_add_on=?, energy_max_add_on=?, speed_add_on=?, attack_add_on=?, defence_add_on=?
		where id=?`,
		commandAdd, affairsAdd, braveryAdd, wisdomAdd,
		forceAdd, energyAdd, speedAdd, attackAdd, defenceAdd, hid); err != nil {
		return nil, err
	}

	// ⑱ hero_blood（mem_hero_blood 无表 → 省略 force_max/energy_max 写回）

	// 组装返回
	armorsOut, err := s.GetHeroArmor(ctx, hid)
	if err != nil {
		return nil, err
	}
	return &EquipResult{
		HID:          hid,
		CommandAddOn: commandAdd,
		AffairsAddOn: affairsAdd,
		BraveryAddOn: braveryAdd,
		WisdomAddOn:  wisdomAdd,
		SpeedAddOn:   speedAdd,
		AttackAddOn:  attackAdd,
		DefenceAddOn: defenceAdd,
		Armors:       armorsOut,
	}, nil
}

// warmBloodGoldSpearSpecial 对齐 HeroFunc.php:1388（≥2 件 12018 active_special=1 → speed+1）。
func (s *Service) warmBloodGoldSpearSpecial(ctx context.Context, hid int) int {
	cnt, err := cellInt(ctx, s.db, `select count(*) from user_armors a, hero_armors b
		where a.sid=b.sid and b.hid=? and b.armorid=12018 and a.active_special=1`, hid)
	if err != nil {
		return 0
	}
	if cnt >= 2 {
		return 1
	}
	return 0
}
