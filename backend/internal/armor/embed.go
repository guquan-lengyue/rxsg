package armor

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"rxsg/backend/internal/model"
)

// embed.go 复刻 legacy EquipmentFunc.php 打孔/镶嵌链：
//   initHoles(619) / parseHoleRule(670) / getHole(694) / isSucc(1387) / probability(1396) /
//   openHole(768) / getDismantleCount(742) / doEmbed(964) / embedLimit(942) / assembleEmbedHoles(1444)。
// 降级：completeTaskWithTaskid(103802)/logUserAction 属 M8 未接线。
// 怪癖保留：
//   - initHoles reduce_gold=value*500/10（整除，无 ceil/floor）；
//   - getHole(1,max) 恒返回 "0"（isSucc=rand(1,100)∈[1,max] 恒 true）；
//   - openHole gid=201 拆珠时 useType=0 耗 1 份化石粉不返还宝珠（legacy 原样）；
//   - doEmbed 坐骑 pos 映射：400-500 → (gid-400)/20；502-582 → (gid-400)/20-5；
//     1400-1500 → (gid-1400)/20；12178-12182 → gid-12178；
//   - assembleEmbedHoles 聚魂珠（10830-10839）只能 pos=4；pos=4 只能聚魂珠（非坐骑时）；
//   - doEmbed 装备行缺失返回 ret=[0,文案] 而非 throw；道具检查
//     empty($goods)||count<=0&&part!=12 优先级怪癖（行缺失恒 no_embed_pearl）；
//   - 坐骑等级门槛/embedLimit 按 $goods['gid']（行缺失→not_zuoji_goods/直通）；
//   - openHole 化石粉空珠(0)拆珠→getDismantleCount throw wrong_pearl_gid；
//     不足文案 not_enough_goods{gid}#{left}。

// InitHoles 对齐 EquipmentFunc.php:619（激活打孔）。
func (s *Service) InitHoles(ctx context.Context, uid, cid, sid int) (int, error) {
	armor, err := s.fetchOneOrNil(ctx, `select u.*, c.part, c.type, c.value
		from user_armors u left join cfg_armor c on c.id=u.armorid
		where u.user_id=? and u.sid=? limit 1`, uid, sid)
	if err != nil {
		return 0, err
	}
	if armor == nil {
		return 0, errf(msgNoSuchArmor)
	}
	part := model.Int(armor, "part")
	if model.Str(armor, "embed_holes") != "" {
		if part != partMount {
			return 0, errf(msgAlreadyActive)
		}
		return 0, errf(msgAlreadyActiveHorse)
	}

	reduceGold := model.Int(armor, "value") * 500 / 10
	cityGold, err := cellInt(ctx, s.db, "select gold from city_resources where city_id=? limit 1", cid)
	if err != nil {
		return 0, err
	}
	if int64(reduceGold) > cityGold {
		return 0, errf(msgNotEnoughGoldAct)
	}

	holes := ""
	pearls := "0,0,0,0,0"
	if part != partMount {
		rule, err := cellStr(ctx, s.db, "select rule from cfg_armor_hole_rule where type=? limit 1", model.Int(armor, "type"))
		if err != nil {
			return 0, err
		}
		holes = parseHoleRule(rule)
	} else {
		holes = "0,0,0,0,0"
	}

	if _, err := s.db.Exec(ctx, "update user_armors set embed_holes=?, embed_pearls=? where sid=?", holes, pearls, sid); err != nil {
		return 0, err
	}
	if _, err := s.db.Exec(ctx, "update city_resources set gold=gold-? where city_id=?", reduceGold, cid); err != nil {
		return 0, err
	}
	return reduceGold, nil
}

// parseHoleRule 对齐 EquipmentFunc.php:670。
// "0"→"1"（初级打孔器）、"N/A"→"3"（不开）、"-1"→"2"（高级）、"-2"→"4"（特级）、数字→getHole(1,n) 恒 "0"。
func parseHoleRule(rule string) string {
	ary := strings.Split(rule, ",")
	ret := make([]string, 0, len(ary))
	for i := 0; i < len(ary); i++ {
		switch ary[i] {
		case "0":
			ret = append(ret, "1")
		case "N/A":
			ret = append(ret, "3")
		case "-1":
			ret = append(ret, "2")
		case "-2":
			ret = append(ret, "4")
		default:
			n, _ := strconv.Atoi(ary[i])
			ret = append(ret, getHole(1, n))
		}
	}
	return strings.Join(ret, ",")
}

// getHole 对齐 EquipmentFunc.php:694（isSucc(1,max) 恒 true → 恒返回 "0"）。
func getHole(min, max int) string {
	// isSucc: probability(min,max) = rand(1,100)∈[min,max]；min=1,max>=1 恒 true
	return "0"
}

// OpenHole 对齐 EquipmentFunc.php:768（化石粉拆珠 / 五彩化石粉 / 开孔）。
func (s *Service) OpenHole(ctx context.Context, uid, sid, gid, pos, useType, count int) (map[string]any, error) {
	armor, err := s.fetchOneOrNil(ctx, `select u.*, c.part
		from user_armors u left join cfg_armor c on c.id=u.armorid
		where u.user_id=? and u.sid=? limit 1`, uid, sid)
	if err != nil {
		return nil, err
	}
	if armor == nil {
		return nil, errf(msgNoSuchArmor)
	}

	if gid == gidHuaShiFen { // 化石粉拆珠
		pearlArr := strings.Split(model.Str(armor, "embed_pearls"), ",")
		if pos < 0 || pos >= len(pearlArr) {
			return nil, errf(msgWaiguaInvalid)
		}
		pearl := atoi(pearlArr[pos])
		needCount, err := getDismantleCount(pearl) // 空珠(0)→legacy throw wrong_pearl_gid
		if err != nil {
			return nil, err
		}
		if (useType == 0 && count != 1) || (useType == 1 && count != needCount) || (useType != 0 && useType != 1) {
			return nil, errf(msgDataException)
		}
		need := needCount
		if useType != 1 {
			need = 1
		}
		have, err := s.goodsCount(ctx, uid, gid)
		if err != nil {
			return nil, err
		}
		if have < int64(need) { // legacy empty($goods)||$goods['count']<$needCount
			return nil, errf(fmt.Sprintf("not_enough_goods%d#%d", gid, int64(need)-have))
		}
		pearlArr[pos] = "0"
		newPearls := strings.Join(pearlArr, ",")
		if _, err := s.db.Exec(ctx, "update user_armors set embed_pearls=? where user_id=? and sid=?", newPearls, uid, sid); err != nil {
			return nil, err
		}
		if useType == 1 {
			if err := s.addGoods(ctx, uid, pearl, 1, 0); err != nil {
				return nil, err
			}
		}
		if err := s.addGoods(ctx, uid, gid, -int64(need), 0); err != nil {
			return nil, err
		}
		armor["embed_pearls"] = newPearls
		return armor, nil
	}

	if gid == gidCaiHua { // 五彩化石粉
		pearlArr := strings.Split(model.Str(armor, "embed_pearls"), ",")
		if pos < 0 || pos >= len(pearlArr) {
			return nil, errf(msgWaiguaInvalid)
		}
		if useType != 0 && useType != 1 {
			return nil, errf(msgDataException)
		}
		have, err := s.goodsCount(ctx, uid, gid)
		if err != nil {
			return nil, err
		}
		if have < 1 { // legacy empty($goods)||intval($goods['count'])<1
			return nil, errf(fmt.Sprintf("not_enough_goods%d#1", gid))
		}
		pearl := atoi(pearlArr[pos]) // 先取原珠（置 0 前）
		pearlArr[pos] = "0"
		newPearls := strings.Join(pearlArr, ",")
		if _, err := s.db.Exec(ctx, "update user_armors set embed_pearls=? where user_id=? and sid=?", newPearls, uid, sid); err != nil {
			return nil, err
		}
		if useType == 1 {
			if err := s.addGoods(ctx, uid, pearl, 1, 0); err != nil {
				return nil, err
			}
		}
		if err := s.addGoods(ctx, uid, gid, -1, 0); err != nil {
			return nil, err
		}
		armor["embed_pearls"] = newPearls
		return armor, nil
	}

	// 开孔（holes[pos]=0）
	ok, err := s.checkGoods(ctx, uid, gid)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errf(fmt.Sprintf("not_enough_goods%d", gid))
	}
	holesArr := strings.Split(model.Str(armor, "embed_holes"), ",")
	if pos < 0 || pos >= len(holesArr) {
		return nil, errf(msgWaiguaInvalid)
	}
	holesArr[pos] = "0"
	newHoles := strings.Join(holesArr, ",")
	if _, err := s.db.Exec(ctx, "update user_armors set embed_holes=? where user_id=? and sid=?", newHoles, uid, sid); err != nil {
		return nil, err
	}
	if err := s.addGoods(ctx, uid, gid, -1, 0); err != nil {
		return nil, err
	}
	armor["embed_holes"] = newHoles
	return armor, nil
}

// getDismantleCount 对齐 EquipmentFunc.php:742；非宝珠 gid → legacy throw wrong_pearl_gid。
func getDismantleCount(gid int) (int, error) {
	if gid >= 300 && gid <= 379 {
		needArr := []int{1, 2, 3, 4, 6, 8, 10, 14, 20, 60}
		level := gid%10 + 1
		return needArr[level-1], nil
	}
	if gid >= 17500 && gid <= 17539 {
		needArr := []int{120, 240, 500, 1000, 2000}
		level := gid%5 + 11
		return needArr[level-11], nil
	}
	if gid >= 10830 && gid <= 10839 {
		needArr := []int{15, 30, 60, 120, 240, 500, 1000, 2000, 4000, 8000}
		level := gid%10 + 1
		return needArr[level-1], nil
	}
	return 0, errf(msgWrongPearlGID)
}

// EmbedResult doEmbed 返回（legacy ret=[1,0,pearls,rows]；行缺失 ret=[0,文案]）。
type EmbedResult struct {
	Started int    `json:"started"` // 1=确实尝试镶嵌；0=装备行缺失（怪癖：返回而非 throw）
	Msg     string `json:"msg"`     // Started=0 时的文案
	Pearls  string `json:"pearls"`  // 结算后 embed_pearls
}

// DoEmbed 对齐 EquipmentFunc.php:964（镶嵌宝珠）。
func (s *Service) DoEmbed(ctx context.Context, uid, sid, pos, gid, isZuoji int) (*EmbedResult, error) {
	armor, err := s.fetchOneOrNil(ctx, `select u.*, c.part, c.type, c.hero_level
		from user_armors u left join cfg_armor c on c.id=u.armorid
		where u.user_id=? and u.sid=? limit 1`, uid, sid)
	if err != nil {
		return nil, err
	}
	if armor == nil { // 怪癖：legacy ret=[0,文案] 而非 throw
		return &EmbedResult{Started: 0, Msg: msgNoSuchArmor}, nil
	}
	part := model.Int(armor, "part")
	if (part == partMount && isZuoji != 1) || (part != partMount && isZuoji == 1) {
		return nil, errf(msgDataException)
	}
	if model.Str(armor, "embed_holes") == "" {
		return nil, errf(msgNoPos)
	}
	holesArr := strings.Split(model.Str(armor, "embed_holes"), ",")
	if pos < 0 || pos >= len(holesArr) || atoi(holesArr[pos]) != 0 {
		return nil, errf(msgNoPos)
	}
	if model.Int(armor, "hid") != 0 {
		return nil, errf(msgArmorInHero)
	}

	// 战神马装（12178-12182）只能给紫色（type>=5）坐骑
	if gid >= 12178 && gid <= 12182 {
		if part != partMount {
			return nil, errf(msgDataException)
		}
		if model.Int(armor, "type") < 5 {
			return nil, errf(msgCanNotExist)
		}
	}

	// 道具行（legacy select * from sys_goods where uid and gid；行缺失→$goods['gid']=null）
	goodsGID, goodsExists, err := s.goodsRowGID(ctx, uid, gid)
	if err != nil {
		return nil, err
	}
	_ = goodsGID // 行存在时恒等于 gid（where 条件）

	if isZuoji == 1 {
		if part != partMount {
			return nil, errf(msgNotZuoji)
		}
		// pos 映射校验
		expectedPos := (gid - 400) / 20
		if pos != expectedPos {
			if gid >= 502 && gid <= 582 {
				if pos != (gid-400)/20-5 {
					return nil, errf(msgInvalidPos)
				}
			} else if gid >= 1400 && gid <= 1500 {
				if pos != (gid-1400)/20 {
					return nil, errf(msgInvalidPos)
				}
			} else if gid >= 12178 && gid <= 12182 {
				if pos != gid-12178 {
					return nil, errf(msgInvalidPos)
				}
			} else {
				return nil, errf(msgInvalidPos)
			}
		}
		// 等级门槛（legacy 按 $goods['gid']：行缺失→null→不落入任何区间→else throw not_zuoji_goods）
		heroLevel := model.Int(armor, "hero_level")
		if !goodsExists {
			return nil, errf(msgNotZuojiGoods)
		}
		if (gid >= 400 && gid <= 500) || (gid >= 502 && gid <= 582) || (gid >= 1400 && gid <= 1500) {
			if ((gid-400)%20+1)*10 > heroLevel {
				return nil, errf(msgZuojiGoodsLevel0)
			}
		} else if gid >= 12178 && gid <= 12182 {
			if model.Int(armor, "type") < 5 {
				return nil, errf(msgCanNotExist)
			}
		} else {
			return nil, errf(msgNotZuojiGoods)
		}
		// 专属坐骑校验（legacy 条件含重复 $gid==448 怪癖，无行为差异）
		if gid == 409 || gid == 429 || gid == 449 || gid == 469 || gid == 489 {
			armorid := model.Int(armor, "armorid")
			if armorid != 53016 && armorid != 53040 && armorid != 53052 {
				return nil, errf(msgNotZuojiGoods2)
			}
		}
		if gid == 410 || gid == 430 || gid == 450 || gid == 470 || gid == 490 {
			armorid := model.Int(armor, "armorid")
			if armorid != 12011 && armorid != 53040 && armorid != 53052 {
				return nil, errf(msgNotZuojiGoods2)
			}
		}
		if gid == 1401 || gid == 1421 || gid == 1441 || gid == 1461 || gid == 1481 {
			armorid := model.Int(armor, "armorid")
			if armorid != 12015 && armorid != 53040 && armorid != 53052 {
				return nil, errf(msgNotZuojiGoods2)
			}
		}
		if gid == 408 || gid == 428 || gid == 448 || gid == 468 {
			armorid := model.Int(armor, "armorid")
			if armorid != 53040 && armorid != 53052 {
				return nil, errf(msgNotZuojiGoods2)
			}
		}
	} else {
		if part == partMount {
			return nil, errf(msgIsZuoji)
		}
		// embedLimit 按 $goods['gid']：行缺失→null→所有区间判断 false→静默直通
		if goodsExists {
			if err := s.embedLimit(armor, gid); err != nil {
				return nil, err
			}
		}
	}

	// 道具检查（怪癖：empty($goods)||count<=0&&part!=12 —— 运算符优先级使"行缺失"
	// 无论 part 均先命中第一条 → no_embed_pearl；行存在且 count<=0 才按 part 分流）
	if !goodsExists {
		return nil, errf(msgNoEmbedPearl)
	}
	cnt, err := s.goodsCount(ctx, uid, gid)
	if err != nil {
		return nil, err
	}
	if cnt <= 0 {
		if part != partMount {
			return nil, errf(msgNoEmbedPearl)
		}
		return nil, errf(msgNoEmbedZuojiArmor)
	}

	pearlArr := strings.Split(model.Str(armor, "embed_pearls"), ",")
	if pos < 0 || pos >= len(pearlArr) {
		return nil, errf(msgWaiguaInvalid)
	}
	if atoi(pearlArr[pos]) != 0 {
		return nil, errf(msgWaiguaInvalid)
	}

	// assembleEmbedHoles 校验（非坐骑时聚魂珠孔位限制）
	if isZuoji != 1 {
		if pos == 4 {
			if gid < 10830 || gid > 10839 {
				return nil, errf(msgGoodPositionErr)
			}
		} else {
			if gid >= 10830 && gid <= 10839 {
				return nil, errf(msgGoodPositionErr)
			}
		}
	}

	pearlArr[pos] = strconv.Itoa(gid)
	newPearls := strings.Join(pearlArr, ",")
	oldPearls := model.Str(armor, "embed_pearls")
	res := &EmbedResult{Started: 1, Pearls: newPearls}
	if newPearls != oldPearls { // legacy：pearls!=old_pearls 才写库扣道具
		if _, err := s.db.Exec(ctx, "update user_armors set embed_pearls=? where user_id=? and sid=?", newPearls, uid, sid); err != nil {
			return nil, err
		}
		if err := s.addGoods(ctx, uid, gid, -1, 13); err != nil {
			return nil, err
		}
		// completeTaskWithTaskid(103804)（gid==325 活动任务）属 M8/M9 未接线
	}
	return res, nil
}

// goodsRowGID 读取 user_goods 行（legacy sys_goods 语义）：返回行内 gid 与行存在性。
func (s *Service) goodsRowGID(ctx context.Context, uid, gid int) (int, bool, error) {
	v, err := s.db.FetchCellInt64(ctx, "select gid from user_goods where user_id=? and gid=? limit 1", uid, gid)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return int(v), true, nil
}

// embedLimit 对齐 EquipmentFunc.php:942（调用方保证 goods 行存在）。
func (s *Service) embedLimit(armor map[string]any, gid int) error {
	strongLevel := model.Int(armor, "strong_level")
	level := 0
	if gid >= 300 && gid <= 379 {
		level = (gid-300)%10 + 1
	} else if gid >= 17500 && gid <= 17539 {
		level = gid%5 + 11
	} else if gid >= 10830 && gid <= 10839 {
		level = (gid-10800)%10 + 1
	}
	if level > 0 && strongLevel < level {
		return errf(msgEmbedLimit)
	}
	return nil
}
