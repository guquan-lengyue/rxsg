package armor

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"rxsg/backend/internal/model"
)

// synth.go 复刻 legacy server/game/EquipmentFunc.php synthStuff(1156) + calculateProbability(1296)
// + parseStuffRecipe(1477)（材料合成 = 铁匠铺「宝石合成」页签）。
//
// 参数序（legacy 逐位 array_shift）：cid, targe_gid, count, dj（巧手符 gid=202）, pearlProtect（宝珠保护符 gid=11172）。
//
// 校验顺序 1:1 保留：
//   count<=0 → equipment.synth_count；
//   用巧手符(dj==1) → gid=202 数量 ≥ count，否则 not_enough_goods202；
//   用保护符(pearlProtect==1) → gid=11172 数量 ≥ count，否则 not_enough_goods11172；
//   targe_gid ∈ {300,310,320,330,340,350,360,370}（1 级宝石）→ equipment.linit_level1。
//
// 成功率/怪癖 1:1 保留（详见下方注释）：
//   硬编码成功率数组 index=目标等级-1；
//   targe_gid==205 → 跳过成功率计算（恒 100%）；
//   [300,379] && dj==1 的巧手符特判（"将错就错，送福利"）：count<=limit 恒 100%，否则 limit+calc(count-limit)；
//   其它情况 prop=prop*(1+dj*10/100) 后 calc(count, prop)。
//
// 结算：addGoods(uid, targe_gid, synth_count, 99) → 材料扣减（保护符按 synth_count，否则按 count）→
//   巧手符扣 202×count → 保护符扣 11172×count → logUserAction(uid,23)。
// 返回 legacy ret 数组 [synth_count, need_gid, needPear]（顺序照搬）。
//
// 降级：材料读写复用本包既有 addGoods/goodsCount（utils.php 等价实现）；
//   技术等级读 city_technics（city_id, technic_id，tid=23）。

const (
	gidQiaoShou     = 202   // 巧手符
	gidPearlProtect = 11172 // 宝珠保护符
	tidSynthTechnic = 23    // 合成技巧
)

// synthRate 成功率表（EquipmentFunc.php:1216，逐字照搬）；下标 = 目标等级-1。
var synthRate = []int{80, 70, 60, 50, 40, 30, 20, 10, 10, 10, 10, 8, 5, 3, 1}

// synthFirstLevel 1 级宝石（不能合成，EquipmentFunc.php:1180 chujiArr）。
var synthFirstLevel = map[int]bool{300: true, 310: true, 320: true, 330: true, 340: true, 350: true, 360: true, 370: true}

// Synth 对齐 EquipmentFunc.php:1156 synthStuff。
func (s *Service) Synth(ctx context.Context, uid, cid, gid, count, dj, pearlProtect int) ([]any, error) {
	if count <= 0 {
		return nil, errf(msgSynthCount)
	}
	if dj == 1 { // 巧手符
		cnt, err := s.goodsCount(ctx, uid, gidQiaoShou)
		if err != nil {
			return nil, err
		}
		if cnt < int64(count) { // legacy empty()||count< 语义
			return nil, errf("not_enough_goods202")
		}
	}
	if pearlProtect == 1 { // 宝珠保护符
		cnt, err := s.goodsCount(ctx, uid, gidPearlProtect)
		if err != nil {
			return nil, err
		}
		if cnt < int64(count) {
			return nil, errf("not_enough_goods11172")
		}
	}
	if synthFirstLevel[gid] { // 1 级宝石不能合成
		return nil, errf(msgLinitLevel1)
	}

	synthCount := count // 最终合成个数
	if gid != 205 {     // 合成镶嵌宝石(205)成功率 100%，不用计算
		techLevel, err := cellInt(ctx, s.db,
			"select level from city_technics where city_id=? and technic_id=? limit 1", cid, tidSynthTechnic)
		if err != nil {
			return nil, err
		}
		targetLevel := 0
		switch {
		case gid >= 300 && gid <= 379: // 前 10 级
			targetLevel = gid%10 + 1
			if int64(targetLevel) > techLevel { // empty() 或等级不足
				return nil, errf(fmt.Sprintf(msgSyncStaffLimitFmt, targetLevel))
			}
		case gid >= 17500 && gid <= 17539: // 后 5 级
			targetLevel = gid%5 + 11
			if techLevel < 10 { // 后五级需要合成技巧 10 级
				return nil, errf(fmt.Sprintf(msgSyncStaffLimitFmt, 10))
			}
		}
		prop := 0.0
		if targetLevel >= 1 && targetLevel-1 < len(synthRate) {
			prop = float64(synthRate[targetLevel-1])
		}
		if gid >= 300 && gid <= 379 && dj == 1 {
			// 巧手符特判（"前 10 级巧手符功能不做改变，将错就错，送福利"）。
			level := gid%10 + 1
			limit := 19683 / math.Pow(3, float64(level-1))
			if float64(count) <= limit {
				prop = 100
			} else {
				lim := int(limit)
				synthCount = lim + calculateProbability(count-lim, prop)
			}
		} else {
			// 未用巧手符，或用巧手符且合成 11-15 级宝珠：正常 +10%/次。
			rateAdd := dj * 10
			prop = prop * (1 + float64(rateAdd)/100)
			synthCount = calculateProbability(count, prop)
		}
	}

	// 配方：cfg_recipe.recipe = "材料gid,数量,..."（parseStuffRecipe 解析）。
	recipe, err := cellStr(ctx, s.db, "select recipe from cfg_recipe where gid=? limit 1", gid)
	if err != nil {
		return nil, err
	}
	if recipe == "" { // 无配方（legacy 空串 explode 后 SQL 异常）→ 统一按材料不足处理
		return nil, errf(msgNoStuff)
	}
	gids, dict := parseStuffRecipe(recipe)
	needGid := needGidString(gids)

	ph := strings.Repeat("?,", len(gids))
	ph = ph[:len(ph)-1]
	args := make([]any, 0, len(gids)+1)
	args = append(args, uid)
	for _, g := range gids {
		args = append(args, g)
	}
	rows, err := s.db.FetchRows(ctx,
		fmt.Sprintf("select gid, `count` from user_goods where user_id=? and gid in (%s)", ph), args...)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 || len(rows) != len(dict) {
		return nil, errf(msgNoStuff)
	}
	for _, stuff := range rows {
		if model.Int(stuff, "count") < dict[model.Int(stuff, "gid")]*count {
			return nil, errf(msgNoStuff)
		}
	}

	// 合成产物入账。
	if err := s.addGoods(ctx, uid, gid, int64(synthCount), 99); err != nil {
		return nil, err
	}

	// 材料扣减：保护符按成功数扣，否则按尝试数扣。
	needPear := 0
	for _, stuff := range rows {
		g := model.Int(stuff, "gid")
		per := dict[g]
		need := per * count
		if pearlProtect == 1 { // 使用了保护符，合成失败的次数不扣除材料
			need = per * synthCount
		}
		if err := s.addGoods(ctx, uid, g, int64(-need), 99); err != nil {
			return nil, err
		}
		needPear = need
	}

	if dj == 1 { // 使用巧手符
		if err := s.addGoods(ctx, uid, gidQiaoShou, int64(-count), 0); err != nil {
			return nil, err
		}
	}
	if pearlProtect == 1 {
		if err := s.addGoods(ctx, uid, gidPearlProtect, int64(-count), 0); err != nil {
			return nil, err
		}
	}
	if err := s.logUserAction(ctx, uid, 23); err != nil { // 材料合成日志
		return nil, err
	}

	return []any{synthCount, needGid, needPear}, nil
}

// calculateProbability 对齐 EquipmentFunc.php:1296：count 次独立判定，每次 mt_rand(1,10000)<=prop*100。
func calculateProbability(count int, prop float64) int {
	ret := 0
	for i := 0; i < count; i++ {
		if float64(mtRand(1, 10000)) <= prop*100 {
			ret++
		}
	}
	return ret
}

// parseStuffRecipe 对齐 EquipmentFunc.php:1477：按 "gid,count,..." 解析为 gid 序列 + gid→需求量。
func parseStuffRecipe(recipe string) ([]int, map[int]int) {
	ary := strings.Split(recipe, ",")
	dict := map[int]int{}
	gids := make([]int, 0, len(ary)/2)
	for i := 0; i+1 < len(ary); i += 2 {
		g := atoi(ary[i])
		gids = append(gids, g)
		dict[g] = atoi(ary[i+1])
	}
	return gids, dict
}

// needGidString 对齐 synthStuff:1237-1244 的 need_gid 拼接（gid 以 ", " 连接）。
func needGidString(gids []int) string {
	parts := make([]string, 0, len(gids))
	for _, g := range gids {
		parts = append(parts, strconv.Itoa(g))
	}
	return strings.Join(parts, ", ")
}

// logUserAction 对齐 utils.php:1808（log_user_actions 流水）。
func (s *Service) logUserAction(ctx context.Context, uid, aid int) error {
	_, err := s.db.Exec(ctx,
		"insert into log_user_actions (user_id, aid, `time`) values (?,?,unix_timestamp())", uid, aid)
	return err
}
