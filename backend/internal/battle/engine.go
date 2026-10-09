package battle

// engine.go —— return_new_place 的 1:1 移植（OLdBattleCron.php:23-530）：
// 首回合初始化 fightsoldier（速度排序）→ 逐单位移动/选敌/伤害/反击 → 回写 + 战报 + 结束。
//
// 保真说明：
//   - 伤害公式 shanghai = count*ap*ap/(ap+target_dp)（L309）逐字保留；打城墙 = floor(a_r_num*ap*peoplenum/100)（L313）。
//   - PHP foreach 迭代的是数组副本：$value 为迭代开始时的快照，$fightsoldier[$key] 为实时值。
//     Go 以「cu 指针=实时、v=迭代副本」同时维护两者，避免边界分支（触发型城防被清空后仍反击）的读值差异。
//   - 原版怪癖保留：array_search 返回键 0 被 `!=false` 判为未命中（L272-273）；防守方反击死亡分支
//     `$fanji_siwang=$fanji_siwang` 自赋值（L421）；响应式城防（type2 did!=3）自爆 rand(0,count)（L293）。

import (
	"context"
	"math"
)

// soldierCfg 是 cfg_soldiers 引擎所需列。
type soldierCfg struct {
	Type       float64
	HP         float64
	AP         float64
	DP         float64
	Range      float64
	Speed      float64
	PeopleNeed float64
}

func (s *Service) loadSoldierCfg(ctx context.Context) (map[int]soldierCfg, error) {
	rows, err := s.db.FetchRows(ctx, "select sid,`type`,hp,ap,dp,`range`,speed,people_need from cfg_soldiers")
	if err != nil {
		return nil, err
	}
	out := make(map[int]soldierCfg, len(rows))
	for _, r := range rows {
		out[int(modelInt64(r, "sid"))] = soldierCfg{
			Type:       modelFloat(r, "type"),
			HP:         modelFloat(r, "hp"),
			AP:         modelFloat(r, "ap"),
			DP:         modelFloat(r, "dp"),
			Range:      modelFloat(r, "range"),
			Speed:      modelFloat(r, "speed"),
			PeopleNeed: modelFloat(r, "people_need"),
		}
	}
	return out, nil
}

// defenceCfg 是 cfg_defence 一行（did → hp/ap/dp/range）。
type defenceCfg struct {
	HP, AP, DP, Range float64
}

func (s *Service) loadDefenceCfg(ctx context.Context) (map[int]defenceCfg, error) {
	rows, err := s.db.FetchRows(ctx, "select * from cfg_defence order by did")
	if err != nil {
		return nil, err
	}
	out := make(map[int]defenceCfg, len(rows))
	for _, r := range rows {
		out[int(modelInt64(r, "did"))] = defenceCfg{
			HP:    modelFloat(r, "hp"),
			AP:    modelFloat(r, "ap"),
			DP:    modelFloat(r, "dp"),
			Range: modelFloat(r, "g_range"),
		}
	}
	return out, nil
}

// initFightSoldier 对齐 OLdBattleCron.php:44-163：构建并按速度降序排好 fightsoldier。
func (s *Service) initFightSoldier(ctx context.Context, b *Battle) ([]*Unit, error) {
	cfg, err := s.loadSoldierCfg(ctx)
	if err != nil {
		return nil, err
	}
	attackStartCID := b.AttackCID
	resistStartCID := b.ResistCID
	if b.Type > 3 {
		attackStartCID = b.AttackStartCID
		resistStartCID = b.ResistStartCID
		if b.Type == 5 {
			resistStartCID = 215265
		}
	}

	// ========== 进攻方
	ahero, err := s.heroBattleAdd(ctx, b.AttackHID, attackStartCID, 1)
	if err != nil {
		return nil, err
	}
	attackCount := getSoldierCounts(b.AttackSoldiers)
	aRaw := getSoldierArray(b.AttackSoldiers, b.AttackPos)
	commandSoldiers := 0.0
	if attackCount != 0 {
		commandSoldiers = (ahero.command * 100) / attackCount
	}
	if commandSoldiers > 1 {
		commandSoldiers = 1
	}
	attackUnits := make([]*Unit, 0, len(aRaw))
	for _, v := range aRaw {
		sc := cfg[int(v.SID)]
		u := &Unit{SID: v.SID, Count: v.Count, Range: v.Range, Typ: 1}
		aSidtype := sc.Type
		if aSidtype != 7 && aSidtype != 8 && aSidtype != 45 && aSidtype != 48 {
			u.Speed = sc.Speed * (1 + ahero.xingjunspeed + ahero.herospeed*commandSoldiers/100)
		} else {
			u.Speed = sc.Speed * (1 + ahero.jiayuspeed + ahero.herospeed*commandSoldiers/100)
		}
		if v.SID != 6 && v.SID != 10 && v.SID != 12 && v.SID != 49 && v.SID != 46 {
			u.GF = sc.Range * (1 + ahero.heroshoot*commandSoldiers/100)
		} else {
			u.GF = sc.Range*(1+ahero.shoot) + (ahero.heroshoot * commandSoldiers)
		}
		u.HP = sc.HP*(1+ahero.blood*commandSoldiers) + ahero.heroblood*commandSoldiers
		u.AP = sc.AP*((1+ahero.gongji)+ahero.attack*commandSoldiers/100) + ahero.heroattack*commandSoldiers
		u.DP = sc.DP*((1+ahero.fangyu)+ahero.defence*commandSoldiers/100) + ahero.herodefence*commandSoldiers
		u.Stype = aSidtype
		attackUnits = append(attackUnits, u)
	}

	// ========== 防守方
	rhero, err := s.heroBattleAdd(ctx, b.ResistHID, resistStartCID, b.Type)
	if err != nil {
		return nil, err
	}
	resistCount := getSoldierCounts(b.ResistSoldiers)
	rRaw := getSoldierArray(b.ResistSoldiers, b.ResistPos)
	rCommandSoldiers := 0.0
	if resistCount != 0 {
		rCommandSoldiers = (rhero.command * 100) / resistCount
	}
	if rCommandSoldiers > 1 {
		rCommandSoldiers = 1
	}
	if !s.isPlayer(ctx, b.ResistUID) {
		rCommandSoldiers = 1
	}
	resistUnits := make([]*Unit, 0, len(rRaw)+6)
	for _, v := range rRaw {
		sc := cfg[int(v.SID)]
		u := &Unit{SID: v.SID, Count: v.Count, Range: v.Range, Typ: 1}
		aSidtype := sc.Type
		if aSidtype != 7 && aSidtype != 8 && aSidtype != 45 && aSidtype != 48 {
			u.Speed = sc.Speed * (1 + rhero.xingjunspeed + rhero.herospeed*rCommandSoldiers/100)
		} else {
			u.Speed = sc.Speed * (1 + rhero.jiayuspeed + rhero.herospeed*rCommandSoldiers/100)
		}
		if v.SID != 6 && v.SID != 10 && v.SID != 12 && v.SID != 49 && v.SID != 46 {
			u.GF = sc.Range + (rhero.heroshoot * rCommandSoldiers)
		} else {
			u.GF = sc.Range*(1+rhero.shoot) + (rhero.heroshoot * rCommandSoldiers)
		}
		u.HP = sc.HP*(1+rhero.blood*rCommandSoldiers) + rhero.heroblood*rCommandSoldiers
		u.AP = sc.AP*((1+rhero.gongji)+rhero.attack*rCommandSoldiers/100) + rhero.heroattack*rCommandSoldiers
		u.DP = sc.DP*((1+rhero.fangyu)+rhero.defence*rCommandSoldiers/100) + rhero.herodefence*rCommandSoldiers
		u.Stype = aSidtype
		resistUnits = append(resistUnits, u)
	}

	// ========== 城墙 + 城防（L105-134）
	if b.WallHP != 0 {
		dCfg, err := s.loadDefenceCfg(ctx)
		if err != nil {
			return nil, err
		}
		wall := &Unit{SID: 18, Typ: 3, Count: b.WallHP, Range: 100, Speed: 0, GF: 0, HP: b.WallHP, AP: 0, DP: 0}
		resistUnits = append(resistUnits, wall)
		// resistdefence 首回合格式为 array2troop(def) 输出的 "N,did,cnt,cnt"；
		// 原版 L120 把第 2 值（=cnt）读入 sid 字段（怪癖，保留），did 为键。
		order, defMap := defence2Array(b.ResistDefence)
		for _, did := range order {
			ent := defMap[did]
			d := dCfg[did]
			resistUnits = append(resistUnits, &Unit{
				SID: ent.Cnt, Typ: 2, Did: float64(did), Range: 100, Count: ent.Cnt, Speed: 0,
				GF: d.Range, HP: d.HP, AP: d.AP, DP: d.DP,
			})
		}
	}

	// ========== 合并（攻方在前，守方在后），按速度冒泡降序（L136-161）
	endsoldier := make([]*Unit, 0, len(attackUnits)+len(resistUnits))
	for _, u := range attackUnits {
		u.Attack = 1
		endsoldier = append(endsoldier, u)
	}
	for _, u := range resistUnits {
		u.Attack = 0
		endsoldier = append(endsoldier, u)
	}
	n := len(endsoldier)
	for i := 0; i < n; i++ {
		for j := n - 1; j > i; j-- {
			if endsoldier[j].Speed > endsoldier[j-1].Speed {
				endsoldier[j], endsoldier[j-1] = endsoldier[j-1], endsoldier[j]
			}
		}
	}
	return endsoldier, nil
}

// returnNewPlace 对齐 OLdBattleCron.php:23：单回合结算。ended 表示本回合触发战斗结束。
func (s *Service) returnNewPlace(ctx context.Context, b *Battle, now int64) (bool, error) {
	st, err := b.loadEngine()
	if err != nil {
		return false, err
	}
	if st == nil {
		fs, err := s.initFightSoldier(ctx, b)
		if err != nil {
			return false, err
		}
		attackNPC := s.IsNPC(ctx, b.AttackUID)
		resistNPC := s.IsNPC(ctx, b.ResistUID)
		st = &engineState{Fight: fs, AttackNPC: attackNPC, ResistNPC: resistNPC}
	}

	tactics, err := s.loadTactics(ctx, b.ID)
	if err != nil {
		return false, err
	}
	cfg, err := s.loadSoldierCfg(ctx)
	if err != nil {
		return false, err
	}
	cfgType := func(sid float64) float64 {
		if c, ok := cfg[int(sid)]; ok {
			return c.Type
		}
		return 0
	}

	fs := st.Fight
	report := ""

	for key := 0; key < len(fs); key++ {
		cu := fs[key]
		if cu == nil || cu.Typ == 3 {
			continue
		}
		v := *cu // 迭代副本（PHP foreach $value）
		sid := v.SID
		if v.Typ != 1 {
			sid = v.Did
		}
		stype := v.Stype
		speed := v.Speed
		pos := v.Range
		rng := getRangBetween(fs, b.FieldRange, key)

		istarget := 0.0
		shanghai := 999.0
		siwang := 0.0
		targetSid := 0.0
		targetType := 0.0
		targetStart := 0.0
		targetEnd := 0.0
		ableFanji := 0.0
		fanjiShanghai := 0.0
		targetStartD := 0.0
		fanjiSiwang := 0.0
		fanjiEnd := 0.0

		isCity := 2.0
		if v.Typ == 1 {
			isCity = 1
		}
		isAttack := 2.0
		var tac tactic
		if v.Attack == 1 {
			isAttack = 1
			tac = tactics.get(1, int(stype))
			if tac.Action == 1 { // 前进
				newrange := pos - speed
				if speed < rng {
					cu.Range = newrange
				} else {
					speed = rng
					cu.Range = pos - rng
				}
			}
			if tac.Action == 3 { // 后退
				newrange := pos + speed
				if newrange > b.FieldRange {
					speed = b.FieldRange - pos
					cu.Range = b.FieldRange
				} else {
					cu.Range = newrange
				}
			}
		} else {
			tac = tactics.get(0, int(stype))
			if tac.Action == 1 {
				newrange := pos + speed
				if newrange < pos+rng {
					cu.Range = newrange
				} else {
					speed = rng
					cu.Range = pos + rng
				}
			}
			if tac.Action == 3 {
				newrange := pos - speed
				if newrange > 0 {
					cu.Range = newrange
				} else {
					speed = pos
					cu.Range = 0
				}
			}
		}

		target := tac.Target
		able := getAbleTarget(fs, key, cfgType)
		if len(able) > 0 {
			// 原版 array_search 松散比较：命中键 0 被 `!=false` 判为未命中（L272-273）。
			targetKey, found := arraySearchLoose(able, target)
			hit := found && targetKey != 0
			if !hit {
				targetKey = able[phpRand(1, len(able))-1].idx
			} else if target == 15 {
				if k, ok := getJTsid(fs); ok {
					targetKey = k
				}
			}
			tk := fs[targetKey]
			if tk == nil {
				// 理论上 abletarget 不含 nil；防御性跳过。
				continue
			}
			targetPos := tk.Range
			targetCount := tk.Count
			targetHp := tk.HP
			targetDp := tk.DP
			istarget = 1
			targetSid = tk.SID
			if target == 15 && hit {
				targetSid = 3
			}
			targetType = tk.Typ
			targetStart = targetCount

			// ── 攻击（L292-334）
			if v.Typ == 2 && v.Did != 3 { // 触发型城防：自爆
				fanjiSiwang = float64(phpRand(0, int(v.Count)))
				cu.Count = v.Count - fanjiSiwang
				if v.AP == 0 {
					siwang = fanjiSiwang
				} else {
					shanghai = fanjiSiwang * v.AP * v.AP / (v.AP + targetDp)
					siwang = math.Floor(shanghai / targetHp)
				}
				targetEnd = targetCount - siwang
			} else {
				aRNum := cu.Count // L308 读实时
				shanghai = aRNum * v.AP * v.AP / (v.AP + targetDp)
				if targetSid == 3 && sid == 6 && targetType == 2 {
					shanghai = math.Floor(shanghai / 50)
				}
				if targetType == 3 { // 打城墙
					peoplenum := cfg[int(sid)].PeopleNeed
					shanghai = math.Floor(aRNum * v.AP * peoplenum / 100)
					siwang = shanghai
					targetEnd = targetCount - siwang
					if targetEnd <= 0 {
						b.WallHP = 0
						for k1, v1 := range fs {
							if v1 != nil && v1.Typ == 2 {
								fs[k1] = nil
							}
						}
					} else {
						b.WallHP = targetEnd
					}
				} else {
					shanghai = math.Floor(shanghai)
					siwang = math.Floor(shanghai / targetHp)
					targetEnd = tk.Count - siwang
					tk.Count = targetEnd
				}
			}

			// ── 反击（L336-436）
			if targetEnd > 0 {
				if tk.Typ == 1 || (tk.Typ == 2 && tk.Did == 3) {
					tk.Count = targetEnd
					fanjiGF := tk.GF
					fanjiAttack := tk.Attack
					if fanjiAttack == 1 { // 目标为攻方
						if cu.Did == 3 && cu.Typ == 2 { // 当前单位是箭塔
							fanjiStype := tk.Stype
							if fanjiStype == 6 || fanjiStype == 10 || fanjiStype == 12 {
								if targetPos-fanjiGF < cu.Range {
									ableFanji = 1
									fanjiShanghai = tk.AP * tk.AP / (tk.AP + v.DP)
									fanjiShanghai = math.Floor(fanjiShanghai)
									targetStartD = cu.Count
									fanjiSiwang = math.Floor(fanjiShanghai / v.HP)
									if v.Count-fanjiSiwang > 0 {
										fanjiEnd = cu.Count - fanjiSiwang
										cu.Count = fanjiEnd
										if cu.Count <= 0 {
											cu.Count = 0
											fanjiEnd = 0
										}
									} else {
										fanjiSiwang = v.Count
										fanjiEnd = 0
										cu.Count = 0
										fs[key] = nil
									}
								}
							}
						} else {
							if targetPos-fanjiGF < cu.Range {
								ableFanji = 1
								fanjiShanghai = targetStart * tk.AP * tk.AP / (tk.AP + v.DP)
								fanjiShanghai = math.Floor(fanjiShanghai)
								targetStartD = cu.Count
								hp := v.HP
								if hp <= 0 {
									hp = 1
								}
								fanjiSiwang = math.Floor(fanjiShanghai / hp)
								if v.Count-fanjiSiwang > 0 {
									fanjiEnd = cu.Count - fanjiSiwang
									cu.Count = fanjiEnd
									if cu.Count <= 0 {
										cu.Count = 0
										fanjiEnd = 0
									}
								} else {
									fanjiSiwang = v.Count
									fanjiEnd = 0
									cu.Count = 0
									fs[key] = nil
								}
							}
						}
					} else { // 目标为守方
						if targetPos+fanjiGF > cu.Range {
							ableFanji = 1
							fanjiShanghai = targetStart * tk.AP * tk.AP / (tk.AP + v.DP)
							fanjiShanghai = math.Floor(fanjiShanghai)
							targetStartD = cu.Count
							fanjiSiwang = math.Floor(fanjiShanghai / v.HP)
							if v.Count-fanjiSiwang > 0 {
								fanjiEnd = cu.Count - fanjiSiwang
								cu.Count = fanjiEnd
								if cu.Count <= 0 {
									cu.Count = 0
									fanjiEnd = 0
								}
							} else {
								// 原版怪癖 L421：`$fanji_siwang = $fanji_siwang;` 自赋值（未取 $value['count']）。
								fanjiEnd = 0
								cu.Count = 0
								fs[key] = nil
							}
						}
					}
				} else {
					tk.Count = fanjiEnd // 被触发型城防干掉的
				}
			} else {
				siwang = targetStart
				targetEnd = 0
				fs[targetKey] = nil
			}
		}

		// 战报（L440）：17 字段 `.000000` 拼接。
		report += phpStr(isAttack) + ".000000," + phpStr(isCity) + ".000000," + phpStr(sid) + ".000000," +
			phpStr(tac.Action) + ".000000," + phpStr(speed) + ".000000," + phpStr(istarget) + ".000000," +
			phpStr(shanghai) + ".000000," + phpStr(targetType) + ".000000," + phpStr(targetSid) + ".000000," +
			phpStr(targetStart) + ".000000," + phpStr(siwang) + ".000000," + phpStr(targetEnd) + ".000000," +
			phpStr(ableFanji) + ".000000," + phpStr(fanjiShanghai) + ".000000," + phpStr(targetStartD) + ".000000," +
			phpStr(fanjiSiwang) + ".000000," + phpStr(fanjiEnd) + ".000000;"
	}

	// array_values（L442）
	alive := make([]*Unit, 0, len(fs))
	for _, u := range fs {
		if u != nil {
			alive = append(alive, u)
		}
	}
	st.Fight = alive

	// 回写字符串 + 结束判定（L443-485）
	attackmem, resistmem, defencemem := 0, 0, 0
	attackstr, resiststr, attackstrpos, resiststrpos, defencestrpos := "", "", "", "", ""
	for _, u := range alive {
		if u.Attack == 1 {
			if u.Count > 0 {
				attackmem++
				attackstr += phpStr(u.SID) + "," + phpStr(u.Count) + ","
				attackstrpos += phpStr(u.SID) + "," + phpStr(u.Range) + ","
			}
		}
		if u.Attack == 0 {
			if u.Count > 0 && u.Typ == 1 {
				resistmem++
				resiststr += phpStr(u.SID) + "," + phpStr(u.Count) + ","
				resiststrpos += phpStr(u.SID) + "," + phpStr(u.Range) + ","
			}
			if u.Count > 0 && u.Typ == 2 {
				defencemem++
				defencestrpos += phpStr(u.Did) + "," + phpStr(u.Range) + "," + phpStr(u.Count) + ","
			}
		}
	}

	battleEnd := -1
	if resistmem == 0 && b.WallHP == 0 {
		battleEnd = 0
	}
	if attackmem == 0 {
		battleEnd = 1
	}
	if attackmem == 0 && resistmem == 0 {
		battleEnd = 3
	}
	if b.Round > 40 {
		battleEnd = 2
	}

	attackstrpos = phpStr(float64(attackmem)) + "," + attackstrpos
	resiststrpos = phpStr(float64(resistmem)) + "," + resiststrpos
	attackstr = phpStr(float64(attackmem)) + "," + attackstr
	resiststr = phpStr(float64(resistmem)) + "," + resiststr
	defencestrpos = phpStr(float64(defencemem)) + "," + defencestrpos

	b.ResistPos = resiststrpos
	b.AttackPos = attackstrpos
	b.AttackSoldiers = attackstr
	b.ResistSoldiers = resiststr
	b.ResistDefence = defencestrpos
	b.Nexttime = now + roundSeconds
	if err := b.saveEngine(st); err != nil {
		return false, err
	}
	if _, err := s.db.Exec(ctx,
		`update battles set resistpos=?, wallhp=?, attackpos=?, attacksoldiers=?, resistsoldiers=?,
		   nexttime=?, round=round+1, resistdefence=?, engine_json=? where id=?`,
		b.ResistPos, b.WallHP, b.AttackPos, b.AttackSoldiers, b.ResistSoldiers,
		b.Nexttime, b.ResistDefence, b.EngineJSON, b.ID); err != nil {
		return false, err
	}
	if _, err := s.db.Exec(ctx,
		"insert into battle_rounds (battleid, round, report) values (?,?,?)",
		b.ID, b.Round, report); err != nil {
		return false, err
	}

	if battleEnd >= 0 {
		if err := s.finishBattle(ctx, b, battleEnd); err != nil {
			return false, err
		}
		return true, nil
	}
	b.Round++
	return false, nil
}
