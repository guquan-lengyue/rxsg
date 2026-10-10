package armor

import (
	"context"
	"fmt"
	"strings"

	"rxsg/backend/internal/model"
)

// barn.go 1:1 复刻 legacy 马厩/坐骑批次：
//   BarnFunc.php loadBarnGoods(5) / loadZuojiArmor(19) / doUnlade(30) / isFitPos(64) +
//   ArmorFunc.php doUpgradeArmor(1190) / checkAndReduceUpgradeGood(1286) / getUpgradeResultMsg(1340) +
//   EquipmentFunc.php loadEmbedPearlByArmor(859) / loadUnActiveHorseArmor(12)。
//
// 已在既有 armor 实现、本次【不重复实现】的坐骑链路（仅说明映射）：
//   - 强化坐骑   EquipmentFunc.php doStrong(178)      → Service.DoStrong（strong.go，is_zuoji=1）
//   - 装备/镶嵌   EquipmentFunc.php doEmbed(964)       → Service.DoEmbed（embed.go，is_zuoji=1）
//   - 驯服(初始化槽) EquipmentFunc.php initHoles(619)   → Service.InitHoles（embed.go，part=12）
//   - 熔炼（坐骑拒）EquipmentFunc.php combineArmor     → Service.CombineArmor（combine.go）
//
// 降级说明：
//   - loadUnActiveHorseArmor:12 返回 7 段（装备列表 + addSpecialArr/getArmorNewAttribute/
//     getArmorEmbedGoods/getTieInfo/getTieArmorAttribute/getDeifyAttribute/getFusionAttribute 六段属性聚合）。
//     后六段依赖的 6 个属性聚合辅助函数【本轮未移植】（散落在 HeroFunc.php，且多数读取的 M5 降级表为空）
//     → 本实现仅返回装备列表（含 cfg_armor.attribute 原串），其余段以空数组占位，前端按 attribute 自行计算。
//   - 公告/日志：completeTask/logUserAction 属 M8/M9 未接线（与既有 armor 包口径一致）。
//
// 怪癖保留：
//   - doUpgradeArmor 的 12007 分支 upgradeToArmorIdAdd 无值 → 成功时 targetArmorid 不变（armorid+0）。
//   - doUpgradeArmor 失败删除按 `where sid=?`（无 uid 条件，原版如此）。
//   - doUpgradeArmor tieid∉{11002,12003,10008,12006,12007}（含 0）→ checkAndReduceUpgradeGood 落 default
//     抛"当前装备无法进行升级！"。

// ── 文案（server/game/lang.php 逐字）─────────────────────────────────────────
const (
	// xilian.param_error（lang.php:2653）
	msgXilianParamError = "参数异常"
	// waigua.invalid（lang.php:2237）
	msgBarnWaiguaInvalid = "数据异常"
	// equipArmor.arm_not_exist（lang.php:1758）
	msgEquipArmNotExist = "该件装备不存在"
	// hero.xidian_unvalid（lang.php:2235）
	msgUpgradeXidianUnvalid = "使用外挂将导致账号数据异常，后果自负！"
	// upgradeArmor.*（lang.php:2662-2668）
	msgArmorCannotUpgrade       = "当前装备无法进行升级！"
	msgUpgradeMaterialNotEnough = "您的升级材料不足，无法升级装备！"
	msgUpgradeProtectNotEnough  = "您的升级保护符不足，无法升级装备！"
	msgUpgradeSucc              = "升级成功！恭喜您获得%s*1"
	msgUpgradeFail1             = "升级失败！装备无损失！"
	msgUpgradeFail2             = "升级失败！装备已损失，使用装备保护符可以确保装备在升级失败时不损失！"
)

// barnXilianGids（BarnFunc.php:10）洗练符 3 选 1。
var barnXilianGids = []int{12079, 12080, 12081}

// ── loadBarnGoods（BarnFunc.php:5）─────────────────────────────────────────

// LoadBarnGoods 对齐 BarnFunc.php:5 loadBarnGoods：
// 校验 xilianIndex∈[0,2]（否则抛"参数异常"），返回马厩强化/升阶道具行（cfg_goods⨝user_goods.count）。
func (s *Service) LoadBarnGoods(ctx context.Context, uid, xilianIndex int) ([]map[string]any, error) {
	if xilianIndex > 2 || xilianIndex < 0 {
		return nil, errf(msgXilianParamError)
	}
	xilianGid := barnXilianGids[xilianIndex]
	rows, err := s.db.FetchRows(ctx, `select b.*,
		(select `+"`count`"+` from user_goods where user_id=? and gid=b.gid) as `+"`count`"+`
		from cfg_goods b
		where b.gid in (212,213,214,11171,?,12157,12158,12159,12160,12161)`,
		uid, xilianGid)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// ── loadZuojiArmor（BarnFunc.php:19）──────────────────────────────────────

// LoadZuojiArmor 对齐 BarnFunc.php:19 loadZuojiArmor：
// 按槽位号 zuoji_type 返回用户拥有的坐骑装备（user_goods⨝cfg_goods，count>0）。
// legacy 额外读取 `select level from cfg_armor where id=$armorid` 但【从不使用】（原版冗余，保留语义：读而不判）。
func (s *Service) LoadZuojiArmor(ctx context.Context, uid, zuojiType, armorid int) ([]map[string]any, error) {
	// legacy：$level 读取后未参与任何判断（原版冗余，保留该读取语义）。
	if _, err := cellInt(ctx, s.db, "select level from cfg_armor where id=? limit 1", armorid); err != nil {
		return nil, err
	}
	rows, err := s.db.FetchRows(ctx, `select * from user_goods g
		left join cfg_goods f on f.gid=g.gid
		where g.user_id=? and f.zuoji_type=? and g.count>0`, uid, zuojiType)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// ── doUnlade（BarnFunc.php:30）────────────────────────────────────────────

// UnladeResult doUnlade 返回（legacy ret=[0,pos,pearls]）。
type UnladeResult struct {
	Pos    int    `json:"pos"`
	Pearls string `json:"pearls"`
}

// DoUnlade 对齐 BarnFunc.php:30 doUnlade：卸下坐骑某个槽位的装备（返还 goods）。
func (s *Service) DoUnlade(ctx context.Context, uid, sid, gid, pos int) (*UnladeResult, error) {
	if pos < 0 || pos > 4 {
		return nil, errf(msgInvalidPos)
	}
	if !isFitPos(gid, pos) {
		return nil, errf(msgInvalidPos)
	}
	armor, err := s.fetchOneOrNil(ctx, "select * from user_armors where user_id=? and sid=? limit 1", uid, sid)
	if err != nil {
		return nil, err
	}
	if armor == nil {
		return nil, errf(msgNoSuchArmor)
	}
	ary := strings.Split(model.Str(armor, "embed_pearls"), ",")
	if pos >= len(ary) || atoi(ary[pos]) != gid {
		return nil, errf(msgBarnWaiguaInvalid)
	}
	pearls := assembleEmbedHoles(ary, pos, 0)
	if _, err := s.db.Exec(ctx, "update user_armors set embed_pearls=? where sid=? and user_id=?", pearls, sid, uid); err != nil {
		return nil, err
	}
	if err := s.addGoods(ctx, uid, gid, 1, 0); err != nil {
		return nil, err
	}
	return &UnladeResult{Pos: pos, Pearls: pearls}, nil
}

// isFitPos 对齐 BarnFunc.php:64 isFitPos（gid→槽位 pos 映射）。
func isFitPos(gid, pos int) bool {
	if gid >= 502 && gid <= 582 {
		return pos == (gid-400)/20-5
	}
	if gid >= 1400 && gid <= 1500 {
		return pos == (gid-1400)/20
	}
	if gid >= 12178 && gid <= 12182 {
		return pos == gid-12178
	}
	return pos == (gid-400)/20
}

// assembleEmbedHoles 对齐 EquipmentFunc.php:1444（value=0 时短路，仅置位并回写）。
func assembleEmbedHoles(ary []string, repPos, value int) string {
	if repPos >= 0 && repPos < len(ary) {
		ary[repPos] = fmt.Sprintf("%d", value)
	}
	out := make([]string, len(ary))
	for i, v := range ary {
		if v == "" {
			out[i] = "0"
		} else {
			out[i] = v
		}
	}
	return strings.Join(out, ",")
}

// ── doUpgradeArmor（ArmorFunc.php:1190）───────────────────────────────────

// UpgradeArmorResult doUpgradeArmor 返回（legacy ret=[msg,goodStr,isProtected,isSucc,armorInfo]）。
type UpgradeArmorResult struct {
	Msg         string    `json:"msg"`
	GoodStr     string    `json:"good_str"`
	IsProtected bool      `json:"is_protected"`
	IsSucc      bool      `json:"is_succ"`
	Armor       *BagArmor `json:"armor"`
}

// upgradeRateArr（ArmorFunc.php:1207）tieid→成功率（百分比）。
var upgradeRateArr = map[int]int{11002: 30, 12003: 20, 10008: 15, 12006: 12, 12007: 10}

// upgradeToArmorIdAdd（ArmorFunc.php:1214）tieid→升级后 armorid 增量。
var upgradeToArmorIdAdd = map[int]int{11002: 42810, 12003: -37000, 10008: 4116, 12006: 12}

// DoUpgradeArmor 对齐 ArmorFunc.php:1190 doUpgradeArmor（装备升阶，坐骑页签"升级"复用）。
func (s *Service) DoUpgradeArmor(ctx context.Context, uid, sid int, isProtected bool) (*UpgradeArmorResult, error) {
	if sid < 0 {
		return nil, errf(msgUpgradeXidianUnvalid)
	}
	armorInfo, err := s.fetchOneOrNil(ctx, `select c.tieid, s.* from cfg_armor c, user_armors s
		where c.id=s.armorid and s.user_id=? and s.sid=? and s.hid=0 limit 1`, uid, sid)
	if err != nil {
		return nil, err
	}
	if armorInfo == nil {
		return nil, errf(msgEquipArmNotExist)
	}
	tieid := model.Int(armorInfo, "tieid")
	protInt := 0
	if isProtected {
		protInt = 1
	}
	goodStr, err := s.checkAndReduceUpgradeGood(ctx, uid, tieid, protInt)
	if err != nil {
		return nil, err
	}

	isSucc := false
	targetArmorid := 0
	if mtRand(1, 100) <= upgradeRateArr[tieid] {
		isSucc = true
		targetArmorid = model.Int(armorInfo, "armorid") + upgradeToArmorIdAdd[tieid]
	} else {
		isCut := true
		if tieid == 11002 { // 名将装备 20% 概率保留
			if mtRand(1, 100) <= 20 {
				isCut = false
			}
		}
		if isProtected {
			isCut = false
		}
		if isCut {
			// legacy：delete ... where sid='$sid' limit 1（无 uid 条件，原版如此）
			if _, err := s.db.Exec(ctx, "delete from user_armors where sid=? limit 1", sid); err != nil {
				return nil, err
			}
		}
	}

	if isSucc {
		if _, err := s.db.Exec(ctx, "update user_armors set armorid=? where sid=? limit 1", targetArmorid, sid); err != nil {
			return nil, err
		}
		switch tieid { // 增加一个孔 + 增加装备耐久
		case 12003: // 冰封装备→冰魂
			if _, err := s.db.Exec(ctx, "update user_armors set embed_holes='0,0,0,0,4', hp_max=300, hp=3000 where sid=? limit 1", sid); err != nil {
				return nil, err
			}
		case 10008: // 神武装备→神龙
			if _, err := s.db.Exec(ctx, "update user_armors set embed_holes='0,0,0,0,4', hp_max=200, hp=2000 where sid=? limit 1", sid); err != nil {
				return nil, err
			}
		}
	}

	msg, err := s.getUpgradeResultMsg(ctx, isSucc, isProtected, targetArmorid)
	if err != nil {
		return nil, err
	}
	armor, err := s.getOneArmorBySid(ctx, uid, sid)
	if err != nil {
		return nil, err
	}
	return &UpgradeArmorResult{Msg: msg, GoodStr: goodStr, IsProtected: isProtected, IsSucc: isSucc, Armor: armor}, nil
}

// checkAndReduceUpgradeGood 对齐 ArmorFunc.php:1286（校验并扣升阶材料，返回 "g1,c1,g2,c2,12157,c3"）。
func (s *Service) checkAndReduceUpgradeGood(ctx context.Context, uid, tieid, isProtected int) (string, error) {
	var gid1, gid2 int
	switch tieid {
	case 11002:
		gid1, gid2 = 12158, 12159
	case 12003:
		gid1, gid2 = 12158, 12160
	case 10008, 12006, 12007:
		gid1, gid2 = 12158, 12161
	default: // 含 tieid=0（PHP case 0 无 break → 落 default 抛错，1:1 保留）
		return "", errf(msgArmorCannotUpgrade)
	}

	c1, err := s.goodsCount(ctx, uid, gid1)
	if err != nil {
		return "", err
	}
	if c1 < 1 {
		return "", errf(msgUpgradeMaterialNotEnough)
	}
	c2, err := s.goodsCount(ctx, uid, gid2)
	if err != nil {
		return "", err
	}
	if c2 < 1 {
		return "", errf(msgUpgradeMaterialNotEnough)
	}
	if isProtected == 1 {
		pc, err := s.goodsCount(ctx, uid, 12157)
		if err != nil {
			return "", err
		}
		if pc < 1 {
			return "", errf(msgUpgradeProtectNotEnough)
		}
		if err := s.addGoods(ctx, uid, 12157, -1, 1115); err != nil {
			return "", err
		}
	}
	if err := s.addGoods(ctx, uid, gid1, -1, 1115); err != nil {
		return "", err
	}
	if err := s.addGoods(ctx, uid, gid2, -1, 1115); err != nil {
		return "", err
	}

	g1, err := s.greatestZeroCount(ctx, uid, gid1)
	if err != nil {
		return "", err
	}
	g2, err := s.greatestZeroCount(ctx, uid, gid2)
	if err != nil {
		return "", err
	}
	pc, err := s.greatestZeroCount(ctx, uid, 12157)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d,%d,%d,%d,12157,%d", gid1, g1, gid2, g2, pc), nil
}

// greatestZeroCount 对齐 `select GREATEST(0,count)`（无行→0）。
func (s *Service) greatestZeroCount(ctx context.Context, uid, gid int) (int64, error) {
	return cellInt(ctx, s.db, "select GREATEST(0,`count`) from user_goods where user_id=? and gid=?", uid, gid)
}

// getUpgradeResultMsg 对齐 ArmorFunc.php:1340 getUpgradeResultMsg。
func (s *Service) getUpgradeResultMsg(ctx context.Context, isSucc, isProtected bool, targetArmorid int) (string, error) {
	if isSucc {
		name, err := cellStr(ctx, s.db, "select name from cfg_armor where id=? limit 1", targetArmorid)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf(msgUpgradeSucc, name), nil
	}
	if isProtected {
		return msgUpgradeFail1, nil
	}
	return msgUpgradeFail2, nil
}

// getOneArmorBySid 对齐 ArmorFunc.php:1355（返回一行装备信息；legacy 的 7 段属性聚合降级为单行）。
func (s *Service) getOneArmorBySid(ctx context.Context, uid, sid int) (*BagArmor, error) {
	r, err := s.fetchOneOrNil(ctx, `select a.*, c.name, c.part, c.type, c.hero_level, c.value,
		c.attribute, c.tieid, c.description
		from user_armors a left join cfg_armor c on c.id=a.armorid
		where a.user_id=? and a.sid=? limit 1`, uid, sid)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, nil
	}
	b := bagArmorOf(r)
	return &b, nil
}

// ── loadEmbedPearlByArmor（EquipmentFunc.php:859）─────────────────────────

// LoadEmbedPearlByArmor 对齐 EquipmentFunc.php:859：按 embed_pearls 串逐个取 cfg_goods 行（"0"→0）。
func (s *Service) LoadEmbedPearlByArmor(ctx context.Context, uid int, gidstr string) ([]any, error) {
	gids := strings.Split(gidstr, ",")
	out := make([]any, 0, len(gids))
	for _, g := range gids {
		if atoi(g) == 0 {
			out = append(out, 0)
			continue
		}
		rec, err := s.fetchOneOrNil(ctx, "select * from cfg_goods where gid=? limit 1", atoi(g))
		if err != nil {
			return nil, err
		}
		if rec == nil { // legacy array_push($objs, $record=false)
			out = append(out, nil)
			continue
		}
		out = append(out, rec)
	}
	return out, nil
}

// ── loadUnActiveHorseArmor（EquipmentFunc.php:12）─────────────────────────

// LoadUnActiveHorseArmor 对齐 EquipmentFunc.php:12：未激活（embed_holes 空）且未穿戴的坐骑装备列表。
// 降级：legacy 返回 7 段（列表 + 6 段属性聚合），后 6 段依赖未移植的聚合辅助函数 → 此处仅返回列表。
func (s *Service) LoadUnActiveHorseArmor(ctx context.Context, uid int) ([]BagArmor, error) {
	rows, err := s.db.FetchRows(ctx, `select a.*, c.name, c.part, c.type, c.hero_level, c.value,
		c.attribute, c.tieid, c.description
		from user_armors a left join cfg_armor c on c.id=a.armorid
		where a.user_id=? and (a.embed_holes='' or a.embed_holes is null) and a.hid=0 and c.part=?
		order by a.strong_level desc, a.combine_level desc`, uid, partMount)
	if err != nil {
		return nil, err
	}
	out := make([]BagArmor, 0, len(rows))
	for _, r := range rows {
		out = append(out, bagArmorOf(r))
	}
	return out, nil
}
