package armor

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"

	"rxsg/backend/internal/db"
	"rxsg/backend/internal/httpx"
	"rxsg/backend/internal/lock"
	"rxsg/backend/internal/model"
)

// service.go 1:1 复刻 legacy 装备批次核心链：
//   ArmorFunc.php equipArmor(351) / offloadArmor(416) / repairArmor(447) / repairAllArmor(583) /
//   renovateArmor(480) / renovateAllArmor(626) / sellArmor(530) / zhuangbeichaijie(703) /
//   getSuipian(879) / checkLoyaltyAdd(219) +
//   EquipmentFunc.php doStrong(178) / strongLimit(131) / reCalculateSuccess(569) /
//   getBestQuality(472) / getProperty(492) / pValue(1407) / fitPValue(1401) / combineArmor(1487) /
//   initHoles(619) / parseHoleRule(670) / getHole(694) / probability(1396) / openHole(768) /
//   doEmbed(964) / embedLimit(942) / assembleEmbedHoles(1444) / getDismantleCount(742) +
//   HeroFunc.php regenerateHeroAttri(1397) / sortArmorAttr(1315) / sortmyArmorAttr(1353) /
//   warmBloodGoldSpearSpecial(1388) + TaskFunc.php cutArmorBySid(1390) +
//   utils.php addGoods/reduceGoods/checkMoney/addMoney/logUserAction + getBufferNobility(1534)。
//
// 原版怪癖 1:1 保留（详见对照表 M5 章节）：
//   - repairAllArmor:583 求和循环 $reduce 未定义（null→0）→ goldNeed 不扣 reduce；
//   - equipArmor:360 先取 part 再 empty() 校验——行缺失时同样报"不能装备在这个部位"；
//   - sellArmor 无 hid/耐久校验（穿戴中、耐久 0 也可直接出售）；
//   - 拆解宝珠 50% 原样返还否则降级；1 级宝珠降级分支不返还；
//   - regenerateHeroAttri：command=level+command_base、攻击%按 bravery×10、防御%按 wisdom×10；
//   - doEmbed 聚魂珠（10830-10839）只能镶第 5 孔；
//   - doStrong 失败降级 before_level=next_level-2（0 级行存在则取 0）。
//
// 缺表/缺数据降级（cfg_* 装备配置为合成种子，见 0009 头注）：
//   - cfg_act 无表 → 强化活动 4422 恒不激活（succ_add 无 +3、reCalculateSuccess 直通）；
//   - mem_hero_buffer 无表 → hasHeroBuffer 恒 false（虎符/马鞭/金枪/buftype3/4 ×1.25 全跳过）；
//   - sys_user_book/cfg_book 无表 → 技能书加成恒 0；
//   - checkHeroLevel(9,80)/(6,50)：sys_user_level 无表 → 恒 false（速度+10 不触发、名将穿戴恒拒绝）；
//   - completeTask/sendSysInform/logUserAction 公告流水属 M8/M9，未接线；
//   - 坐骑强化活动 strongActOnce、熔炼 2013-09 活动奖励（过期时间窗）恒不触发；
//   - 升阶 doUpgradeArmor/神化 deifyArmor/析装 multipAnalyzeArmor 本轮未接路由（前端未使用）。

const (
	partMount = 12 // 坐骑位（cfg_armor.part）

	gidTianGong   = 203 // 天工符 +7%
	gidQianKun    = 204 // 乾坤宝珠 失败保级
	gidStrong     = 205 // 强化宝珠（0-9 级材料）
	gidBoLe       = 212 // 伯乐符 +7%（坐骑）
	gidShiHuang   = 213 // 师皇针 失败保级（坐骑）
	gidLingTong   = 214 // 灵通甘草（坐骑 0-9 级材料）
	gidHighStrong = 11170
	gidHighLTGC   = 11171
	gidFuse1      = 10776 // 熔炼石（<3 级必耗）
	gidFuse2      = 10777 // 玄晶石（≥3 级必耗）
	gidFusePro    = 10778 // 熔炼保护符
	gidChip       = 10400 // 装备碎片（addThings tid）
	gidHuaShiFen  = 201   // 化石粉（拆宝珠）
	gidCaiHua     = 12156 // 五彩化石粉

	marketBuildingID = 13 // ID_BUILDING_MARKET（common.php:59）
	tidBlacksmith    = 21 // 打造技巧
	tidHorse         = 22 // 驯马技巧

	maxStrongLevel = 15
	maxCombine     = 7
)

// levelvalue（HeroFunc.php:1427，扩充 15 级）；下标即强化等级。
var levelvalue = []int{0, 2, 2, 2, 2, 2, 2, 3, 3, 3, 6, 3, 3, 5, 5, 7}

// combineRate（EquipmentFunc.php:1552）目标等级→成功率（百分比）。
var combineRate = map[int]int{1: 75, 2: 35, 3: 15, 4: 10, 5: 5, 6: 3, 7: 1}

// mountHighRate（:336）坐骑 11+ 级强化概率（百分比）。
var mountHighRate = map[int]int{11: 5, 12: 5, 13: 5, 14: 3, 15: 1}

// elevenPearlMap（ArmorFunc.php:761）11 级宝珠降级映射。
var elevenPearlMap = map[int]int{17500: 309, 17505: 319, 17510: 329, 17515: 339, 17520: 349, 17525: 359, 17530: 369, 17535: 379}

// strongLimitArray（lang.php:2190）颜色名。
var strongLimitArray = map[int]string{1: "灰装", 2: "白装", 3: "绿装"}

// lang 文案（server/game/lang.php 逐字）。
const (
	msgArmNotExist        = "该件装备不存在"
	msgNotRightPart       = "不能装备在这个部位"
	msgNoHPMaxEquip       = "装备已经没有耐久，不能使用，请先修复。"
	msgArmInUse           = "该件装备已经被其他武将使用了"
	msgEquipLevelFmt      = "这件装备需要将领等级达到%d级才能使用。"
	msgHeroStateWrong     = "将领不在本城或没有效忠于你。只能给本城内效忠于你的将领换装。"
	msgHeroTypeWrong      = "该装备无法穿戴在当前将领身上"
	msgMustBeNPC          = "该件装备只能穿在名将身上。"
	msgRepairNoNeed       = "这件装备没有损坏，不需要修理。"
	msgRepairNoGold       = "本城黄金不足。"
	msgRepairNoHPMax      = "装备已经没有耐久，不能修理，只能修复了。"
	msgRenovateNoNeed     = "这件装备没有损毁，不需要修复。"
	msgRenovateNoMoney    = "你没有足够的元宝，请充值后再修复。"
	msgSellMarketLow      = "市场达到5级才能回收装备。"
	msgSellNobilityLow    = "爵位达到“公士”才能回收装备。"
	msgSellZuojiEmbed     = "需要先卸下坐骑装备，才能回收坐骑"
	msgNoSuchArmor        = "装备不存在"
	msgNoSuchHorse        = "坐骑不存在"
	msgArmorInHero        = "你的装备在英雄身上，请先卸载"
	msgDataException      = "数据异常，可能是网络延迟导致，请重新操作"
	msgWaiguaInvalid      = "数据异常"
	msgWaiguaForbidden    = "以检测到恶意操作，已记录登录IP信息和启动账号监控！"
	msgWrongItem          = "错误的道具"
	msgNoTLGC             = "你没有通灵甘草"
	msgNoGJLTGC           = "你没有高级通灵甘草"
	msgNoStrongPearl      = "你没有足够的强化宝珠"
	msgNoHighStrong       = "你没有足够的强化宝珠"
	msgCannotStrong       = "你已经不能强化该装备"
	msgStrongLimitFmt     = "%s只能强化到%d级"
	msgStrongTechSmith    = "需要打造技巧(%d级)"
	msgStrongTechBarn     = "需要驯马技巧(%d级)"
	msgStrongLevelLimit   = "目前未开放更高等级的强化"
	msgBlueStrongLimit    = "蓝色装备熔炼等级必须达到玄铁才能强化到10级以上"
	msgNotZuoji           = "你选择的不是坐骑"
	msgIsZuoji            = "你不能选择坐骑，请到马厩装备坐骑"
	msgNoPos              = "你的装备还没有激活开孔，请到铁匠铺激活装备"
	msgNoEmbedPearl       = "你没有足够的镶嵌珍珠"
	msgNoEmbedZuojiArmor  = "你没有足够的坐骑装备"
	msgEmbedLimit         = "镶嵌宝珠的等级不能大于装备的强化等级"
	msgInvalidPos         = "你选择的坐骑位置不对"
	msgZuojiGoodsLevel0   = "坐骑等级不够，不能装备"
	msgNotZuojiGoods      = "你选择的不是坐骑装备"
	msgNotZuojiGoods2     = "该坐骑装备不能装备在此坐骑"
	msgCanNotExist        = "该装备不能被镶嵌"
	msgGoodPositionErr    = "宝珠镶嵌位置出错，请重新操作"
	msgNotEnoughGoldAct   = "你没有足够的黄金激活"
	msgAlreadyActive      = "你已经激活该装备"
	msgAlreadyActiveHorse = "你已经驯服该坐骑"
	msgNotEnoughFuse1     = "你没有足够的熔炼石"
	msgNotEnoughFuse2     = "你没有足够的玄晶石"
	msgCombineLevelNE     = "你所选的装备品质不一致。"
	msgCombineZuoji       = "坐骑不能熔炼。"
	msgCombineMaxLevel    = "已经是最高级的了！"
	msgInvalidParam       = "参数错误"
	msgNoHero             = "该将领不是你的部下。"
	msgChaijieNotActive   = "装备未激活，不能被拆解！"
	msgChaijieHorse       = "拆解马匹？太残忍了吧。"
	msgChaijieGet         = "装备拆解成功，装备消失，获得"
	msgChaijieNothing     = "装备拆解成功，装备消失，未获得任何物品。"
	msgArmorChip          = "装备碎片"
	msgChaijieEnd         = "。"
	msgWrongPearlGID      = "宝珠gid错误"
)

type Service struct {
	db *db.DB
	lk *lock.Locker
}

func NewService(d *db.DB) *Service {
	return &Service{db: d, lk: lock.New(d.DB)}
}

// WithUserLock 暴露用户级锁给 handler。
func (s *Service) WithUserLock(ctx context.Context, uid int, key string, fn func(context.Context) error) error {
	return s.lk.WithUserLock(ctx, uid, key, fn)
}

// errf 构造与 legacy throw new Exception 等价的错误响应。
func errf(msg string) error {
	return httpx.BadRequest("armor_error", msg)
}

// ---------- 行工具（对齐 legacy empty()/sql_fetch_one 无行语义） ----------

// fetchOneOrNil：无行→nil（legacy sql_fetch_one empty 语义）。
func (s *Service) fetchOneOrNil(ctx context.Context, q string, args ...any) (map[string]any, error) {
	row, err := s.db.FetchOne(ctx, q, args...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return row, nil
}

func cellInt(ctx context.Context, d *db.DB, q string, args ...any) (int64, error) {
	v, err := d.FetchCellInt64(ctx, q, args...)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return v, err
}

func cellStr(ctx context.Context, d *db.DB, q string, args ...any) (string, error) {
	v, err := d.FetchCellString(ctx, q, args...)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// atoi 对齐 intval()（非法→0）。
func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

// ---------- 穿戴/卸下 ----------

// isHeroInCity 对齐 HeroFunc.php:1949：state∈{0,1,7,8}。
func isHeroInCity(state int) bool {
	return state == 0 || state == 1 || state == 7 || state == 8
}

// EquipResult 穿戴/卸下返回（legacy ret=[getHeroDetail, doGetHeroArmor]，
// Go 版返回重算后的属性加成 + 该武将穿戴列表）。
type EquipResult struct {
	HID          int         `json:"hid"`
	CommandAddOn int         `json:"command_add_on"`
	AffairsAddOn int         `json:"affairs_add_on"`
	BraveryAddOn int         `json:"bravery_add_on"`
	WisdomAddOn  int         `json:"wisdom_add_on"`
	SpeedAddOn   int         `json:"speed_add_on"`
	AttackAddOn  int         `json:"attack_add_on"`
	DefenceAddOn int         `json:"defence_add_on"`
	Armors       []HeroArmor `json:"armors"`
}

// EquipArmor 对齐 ArmorFunc.php:351 equipArmor。
func (s *Service) EquipArmor(ctx context.Context, uid, hid, sid, spart int) (*EquipResult, error) {
	// 怪癖保留：legacy 先取 $armorInfo["part"] 再 empty() 校验；行缺失时
	// null != floor(spart/10)（spart>0）→ 同样报"不能装备在这个部位"。
	armorInfo, err := s.fetchOneOrNil(ctx, `select u.*, c.part, c.type, c.hero_level, c.tieid
		from user_armors u left join cfg_armor c on c.id=u.armorid where u.sid=? and u.user_id=? limit 1`, sid, uid)
	if err != nil {
		return nil, err
	}
	part := model.Int(armorInfo, "part") // nil→0（model.Int 支持 nil map）
	if part != spart/10 {
		return nil, errf(msgNotRightPart)
	}
	if armorInfo == nil {
		return nil, errf(msgArmNotExist)
	}
	if model.Int(armorInfo, "hid") != 0 {
		return nil, errf(msgArmInUse)
	}
	hp := int(math.Ceil(float64(model.Int(armorInfo, "hp")) / 10))
	if hp <= 0 {
		return nil, errf(msgNoHPMaxEquip)
	}
	hero, err := s.fetchOneOrNil(ctx, "select * from heroes where id=? limit 1", hid)
	if err != nil {
		return nil, err
	}
	if hero == nil || !isHeroInCity(model.Int(hero, "state")) {
		return nil, errf(msgHeroStateWrong)
	}
	armorid := model.Int(armorInfo, "armorid")
	if model.Int(hero, "hero_type") != 1000 && armorid >= 15000 && armorid < 16000 {
		return nil, errf(msgHeroTypeWrong)
	}
	if model.Int(armorInfo, "hero_level") > model.Int(hero, "level") {
		return nil, errf(fmt.Sprintf(msgEquipLevelFmt, model.Int(armorInfo, "hero_level")))
	}
	if armorid >= 12001 && armorid <= 12008 && hid >= 1030 {
		ok, err := s.checkHeroLevel6_50(ctx, uid)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errf(msgMustBeNPC)
		}
	}
	old, err := s.fetchOneOrNil(ctx, "select sid from hero_armors where hid=? and spart=? limit 1", hid, spart)
	if err != nil {
		return nil, err
	}
	if old != nil {
		if _, err := s.db.Exec(ctx, "update user_armors set hid=0 where sid=?", model.Int(old, "sid")); err != nil {
			return nil, err
		}
	}
	if _, err := s.db.Exec(ctx, "update user_armors set hid=? where sid=?", hid, sid); err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(ctx, `insert into hero_armors (hid, spart, sid, armorid) values (?,?,?,?)
		on duplicate key update sid=values(sid), armorid=values(armorid)`, hid, spart, sid, armorid); err != nil {
		return nil, err
	}
	if model.Int(hero, "state") == 1 {
		if err := s.markResChanging(ctx, model.Int(hero, "city_id")); err != nil {
			return nil, err
		}
	}
	if err := s.checkLoyaltyAdd(ctx, hid); err != nil {
		return nil, err
	}
	return s.regenerateHeroAttri(ctx, uid, hid)
}

// OffloadArmor 对齐 ArmorFunc.php:416 offloadArmor。
func (s *Service) OffloadArmor(ctx context.Context, uid, hid, spart int) (*EquipResult, error) {
	info, err := s.fetchOneOrNil(ctx, `select h.sid from hero_armors h
		left join user_armors u on u.sid=h.sid where h.hid=? and h.spart=? limit 1`, hid, spart)
	if err != nil {
		return nil, err
	}
	if info == nil {
		return nil, errf(msgArmNotExist)
	}
	hero, err := s.fetchOneOrNil(ctx, "select * from heroes where id=? limit 1", hid)
	if err != nil {
		return nil, err
	}
	if hero == nil || !isHeroInCity(model.Int(hero, "state")) {
		return nil, errf(msgHeroStateWrong)
	}
	sid := model.Int(info, "sid")
	if _, err := s.db.Exec(ctx, "update user_armors set hid=0 where sid=?", sid); err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(ctx, "delete from hero_armors where hid=? and spart=?", hid, spart); err != nil {
		return nil, err
	}
	if err := s.checkLoyaltyAdd(ctx, hid); err != nil {
		return nil, err
	}
	res, err := s.regenerateHeroAttri(ctx, uid, hid)
	if err != nil {
		return nil, err
	}
	if model.Int(hero, "state") == 1 {
		if err := s.markResChanging(ctx, model.Int(hero, "city_id")); err != nil {
			return nil, err
		}
	}
	return res, nil
}

// checkLoyaltyAdd 对齐 ArmorFunc.php:219（洛神玉佩 12010 active_special 忠诚封顶）。
func (s *Service) checkLoyaltyAdd(ctx context.Context, hid int) error {
	count, err := cellInt(ctx, s.db, `select count(*) from user_armors a, hero_armors b
		where a.sid=b.sid and b.hid=? and b.armorid=12010 and a.active_special=1`, hid)
	if err != nil {
		return err
	}
	heroType, err := cellInt(ctx, s.db, "select hero_type from heroes where id=? limit 1", hid)
	if err != nil {
		return err
	}
	base := int64(100)
	if heroType == 10001 {
		base = 300
	}
	maxLoyalty := base
	if count == 1 {
		maxLoyalty = base + 20
	} else if count == 2 {
		maxLoyalty = base + 50
	}
	_, err = s.db.Exec(ctx, "update heroes set loyalty=LEAST(loyalty,?) where id=?", maxLoyalty, hid)
	return err
}

// checkHeroLevel6_50 对齐 checkHeroLevel(uid,6,50)（名将穿戴门槛）。
// 降级：sys_user_level 无表 → mUserLevel 恒 0 → 恒 false（与 hero 包同口径）。
func (s *Service) checkHeroLevel6_50(ctx context.Context, uid int) (bool, error) {
	mHeroLevel, err := cellInt(ctx, s.db, "select level from heroes where user_id=? and hero_type=1000 limit 1", uid)
	if err != nil {
		return false, err
	}
	return mHeroLevel >= 50 && false, nil // mUserLevel(0) >= 6 恒 false
}

// markResChanging 对齐 updateCityResourceAdd 的 city_res_add.resource_changing 标记。
func (s *Service) markResChanging(ctx context.Context, cid int) error {
	_, err := s.db.Exec(ctx, `insert into city_res_add (city_id, resource_changing) values (?,1)
		on duplicate key update resource_changing=1`, cid)
	return err
}

// ---------- 耐久：维修/翻新 ----------

// RepairArmor 对齐 ArmorFunc.php:447 repairArmor。
func (s *Service) RepairArmor(ctx context.Context, uid, cid, sid int) (int, int, int64, error) {
	info, err := s.fetchOneOrNil(ctx, "select * from user_armors where sid=? and user_id=? limit 1", sid, uid)
	if err != nil {
		return 0, 0, 0, err
	}
	if info == nil {
		return 0, 0, 0, errf(msgArmNotExist)
	}
	hp := int(math.Ceil(float64(model.Int(info, "hp")) / 10))
	hpmax := model.Int(info, "hp_max")
	if hp < 0 {
		return 0, 0, 0, errf(msgRepairNoHPMax)
	}
	goldNeed := (hpmax - hp) * 100
	if goldNeed <= 0 {
		return 0, 0, 0, errf(msgRepairNoNeed)
	}
	cityGold, err := cellInt(ctx, s.db, "select gold from city_resources where city_id=? limit 1", cid)
	if err != nil {
		return 0, 0, 0, err
	}
	if int64(goldNeed) > cityGold {
		return 0, 0, 0, errf(msgRepairNoGold)
	}
	reduce := int(math.Max(1, math.Ceil(float64(hpmax-hp)/10)))
	hpmax = int(math.Max(0, float64(hpmax-reduce)))
	if _, err := s.db.Exec(ctx, "update user_armors set hp=?, hp_max=? where sid=?", hpmax*10, hpmax, sid); err != nil {
		return 0, 0, 0, err
	}
	if _, err := s.db.Exec(ctx, "update city_resources set gold=GREATEST(0, gold-?) where city_id=?", goldNeed, cid); err != nil {
		return 0, 0, 0, err
	}
	if model.Int(info, "hid") != 0 {
		if _, err := s.regenerateHeroAttri(ctx, uid, model.Int(info, "hid")); err != nil {
			return 0, 0, 0, err
		}
	}
	newGold, err := cellInt(ctx, s.db, "select gold from city_resources where city_id=? limit 1", cid)
	if err != nil {
		return 0, 0, 0, err
	}
	return sid, hpmax, newGold, nil
}

// RepairAllArmor 对齐 ArmorFunc.php:583 repairAllArmor。
// 怪癖保留：求和循环 $reduce 未定义（null→0）→ goldNeed 不扣 reduce；执行循环才扣。
func (s *Service) RepairAllArmor(ctx context.Context, uid, cid int, sids []int) (int64, error) {
	if len(sids) == 0 {
		return 0, errf(msgArmNotExist)
	}
	ph := strings.Repeat("?,", len(sids))
	ph = ph[:len(ph)-1]
	args := make([]any, len(sids))
	for i, v := range sids {
		args[i] = v
	}
	rows, err := s.db.FetchRows(ctx, fmt.Sprintf(
		"select * from user_armors where user_id=? and sid in (%s)", ph),
		append([]any{uid}, args...)...)
	if err != nil {
		return 0, err
	}
	goldNeed := 0
	for _, a := range rows {
		hp := int(math.Ceil(float64(model.Int(a, "hp")) / 10))
		hpmax := model.Int(a, "hp_max")
		if hp < 0 {
			return 0, errf(msgRepairNoHPMax)
		}
		goldNeed += (hpmax - hp) * 100 // $reduce 未定义 → 不扣
	}
	if goldNeed <= 0 {
		return 0, errf(msgRepairNoNeed)
	}
	cityGold, err := cellInt(ctx, s.db, "select gold from city_resources where city_id=? limit 1", cid)
	if err != nil {
		return 0, err
	}
	if int64(goldNeed) > cityGold {
		return 0, errf(msgRepairNoGold)
	}
	for _, a := range rows {
		hp := int(math.Ceil(float64(model.Int(a, "hp")) / 10))
		hpmax := model.Int(a, "hp_max")
		reduce := int(math.Max(1, math.Ceil(float64(hpmax-hp)/10)))
		hpmax = int(math.Max(0, float64(hpmax-reduce)))
		if _, err := s.db.Exec(ctx, "update user_armors set hp=?, hp_max=? where sid=?", hpmax*10, hpmax, model.Int(a, "sid")); err != nil {
			return 0, err
		}
	}
	if _, err := s.db.Exec(ctx, "update city_resources set gold=GREATEST(0, gold-?) where city_id=?", goldNeed, cid); err != nil {
		return 0, err
	}
	if len(rows) > 0 && model.Int(rows[0], "hid") != 0 {
		if _, err := s.regenerateHeroAttri(ctx, uid, model.Int(rows[0], "hid")); err != nil {
			return 0, err
		}
	}
	newGold, err := cellInt(ctx, s.db, "select gold from city_resources where city_id=? limit 1", cid)
	if err != nil {
		return 0, err
	}
	return newGold, nil
}

// RenovateArmor 对齐 ArmorFunc.php:480 renovateArmor（元宝修复）。
func (s *Service) RenovateArmor(ctx context.Context, uid, sid int) (int, error) {
	info, err := s.fetchOneOrNil(ctx, `select u.* from user_armors u
		left join cfg_armor c on c.id=u.armorid where u.sid=? and u.user_id=? limit 1`, sid, uid)
	if err != nil {
		return 0, err
	}
	if info == nil {
		return 0, errf(msgArmNotExist)
	}
	hp := int(math.Ceil(float64(model.Int(info, "hp")) / 10))
	hpmax := model.Int(info, "hp_max")
	orihpmax := model.Int(info, "ori_hp_max")
	moneyNeed := (orihpmax - hpmax) + int(math.Ceil(float64(hpmax-hp)/10))
	if moneyNeed <= 0 {
		return 0, errf(msgRenovateNoNeed)
	}
	ok, err := s.checkMoneyOK(ctx, uid, int64(moneyNeed))
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, errf(msgRenovateNoMoney)
	}
	if err := s.addMoney(ctx, uid, -int64(moneyNeed), 100); err != nil {
		return 0, err
	}
	if _, err := s.db.Exec(ctx, "update user_armors set hp=?, hp_max=? where sid=?", orihpmax*10, orihpmax, sid); err != nil {
		return 0, err
	}
	if model.Int(info, "hid") != 0 {
		if _, err := s.regenerateHeroAttri(ctx, uid, model.Int(info, "hid")); err != nil {
			return 0, err
		}
	}
	return sid, nil
}

// RenovateAllArmor 对齐 ArmorFunc.php:626 renovateAllArmor。
func (s *Service) RenovateAllArmor(ctx context.Context, uid int, sids []int) error {
	if len(sids) == 0 {
		return errf(msgArmNotExist)
	}
	ph := strings.Repeat("?,", len(sids))
	ph = ph[:len(ph)-1]
	args := make([]any, len(sids))
	for i, v := range sids {
		args[i] = v
	}
	rows, err := s.db.FetchRows(ctx, fmt.Sprintf(
		"select * from user_armors where user_id=? and sid in (%s)", ph),
		append([]any{uid}, args...)...)
	if err != nil {
		return err
	}
	moneyNeed := 0
	for _, a := range rows {
		hp := int(math.Ceil(float64(model.Int(a, "hp")) / 10))
		hpmax := model.Int(a, "hp_max")
		orihpmax := model.Int(a, "ori_hp_max")
		moneyNeed += (orihpmax - hpmax) + int(math.Ceil(float64(hpmax-hp)/10))
	}
	if moneyNeed <= 0 {
		return errf(msgRenovateNoNeed)
	}
	ok, err := s.checkMoneyOK(ctx, uid, int64(moneyNeed))
	if err != nil {
		return err
	}
	if !ok {
		return errf(msgRenovateNoMoney)
	}
	if err := s.addMoney(ctx, uid, -int64(moneyNeed), 100); err != nil {
		return err
	}
	for _, a := range rows {
		orihpmax := model.Int(a, "ori_hp_max")
		if _, err := s.db.Exec(ctx, "update user_armors set hp=?, hp_max=? where sid=?", orihpmax*10, orihpmax, model.Int(a, "sid")); err != nil {
			return err
		}
	}
	if len(rows) > 0 && model.Int(rows[0], "hid") != 0 {
		if _, err := s.regenerateHeroAttri(ctx, uid, model.Int(rows[0], "hid")); err != nil {
			return err
		}
	}
	return nil
}

// SellArmor 对齐 ArmorFunc.php:530 sellArmor。
// 怪癖保留：不校验 hid（穿戴中也可出售）、不校验耐久。
func (s *Service) SellArmor(ctx context.Context, uid, cid, sid int) (int, int, int64, error) {
	marketLevel, err := cellInt(ctx, s.db, "select level from buildings where city_id=? and building_id=? limit 1", cid, marketBuildingID)
	if err != nil {
		return 0, 0, 0, err
	}
	if marketLevel < 5 {
		return 0, 0, 0, errf(msgSellMarketLow)
	}
	nobilityStr, err := cellStr(ctx, s.db, "select nobility from users where id=? limit 1", uid)
	if err != nil {
		return 0, 0, 0, err
	}
	nobility := 0.0
	if v, e := strconv.ParseFloat(nobilityStr, 64); e == nil {
		nobility = v
	}
	nobilityBuf, err := s.getBufNobility(ctx, uid, nobility)
	if err != nil {
		return 0, 0, 0, err
	}
	if nobilityBuf < 1 {
		return 0, 0, 0, errf(msgSellNobilityLow)
	}
	info, err := s.fetchOneOrNil(ctx, `select u.*, c.part, c.type, c.value, c.ori_hp_max
		from user_armors u, cfg_armor c where u.sid=? and u.user_id=? and c.id=u.armorid limit 1`, sid, uid)
	if err != nil {
		return 0, 0, 0, err
	}
	if info == nil {
		return 0, 0, 0, errf(msgArmNotExist)
	}
	armorid := model.Int(info, "armorid")
	if model.Int(info, "part") == partMount {
		for _, p := range strings.Split(model.Str(info, "embed_pearls"), ",") {
			if atoi(p) > 0 {
				return 0, 0, 0, errf(msgSellZuojiEmbed)
			}
		}
	}
	hp := int(math.Ceil(float64(model.Int(info, "hp")) / 10))
	orihpmax := model.Int(info, "ori_hp_max")
	ratio := math.Max(1, math.Floor(float64(hp)/float64(orihpmax)))
	goldAdd := int(ratio*float64(model.Int(info, "value"))) * 500
	if _, err := s.db.Exec(ctx, "update city_resources set gold=gold+? where city_id=?", goldAdd, cid); err != nil {
		return 0, 0, 0, err
	}
	if _, err := s.db.Exec(ctx, `insert into log_selled_armor
		(sid, user_id, armorid, hp, hp_max, hid, strong_level, strong_value, embed_pearls, embed_holes, deified, time)
		select sid, user_id, armorid, hp, hp_max, hid, strong_level, strong_value, embed_pearls, embed_holes, deified, unix_timestamp()
		from user_armors where sid=? and user_id=?`, sid, uid); err != nil {
		return 0, 0, 0, err
	}
	if _, err := s.db.Exec(ctx, "delete from user_armors where sid=? and user_id=?", sid, uid); err != nil {
		return 0, 0, 0, err
	}
	if _, err := s.db.Exec(ctx, `insert into log_armor (user_id, armorid, count, time, type) values (?,?,-1,unix_timestamp(),9)`, uid, armorid); err != nil {
		return 0, 0, 0, err
	}
	newGold, err := cellInt(ctx, s.db, "select gold from city_resources where city_id=? limit 1", cid)
	if err != nil {
		return 0, 0, 0, err
	}
	return sid, cid, newGold, nil
}

// ---------- 拆解 ----------

// Chaijie 对齐 ArmorFunc.php:703 zhuangbeichaijie。
func (s *Service) Chaijie(ctx context.Context, uid, sid int) (string, error) {
	info, err := s.fetchOneOrNil(ctx, `select u.*, c.hero_level, c.attribute, c.part
		from user_armors u left join cfg_armor c on c.id=u.armorid
		where u.sid=? and u.user_id=? limit 1`, sid, uid)
	if err != nil {
		return "", err
	}
	if info == nil {
		return "", errf(msgArmNotExist)
	}
	if model.Int(info, "hid") != 0 {
		return "", errf(msgArmInUse)
	}
	if model.Str(info, "embed_holes") == "" {
		return "", errf(msgChaijieNotActive)
	}
	if model.Int(info, "part") == partMount {
		return "", errf(msgChaijieHorse)
	}
	suipianCount := getSuipian(model.Int(info, "hero_level"), model.Str(info, "attribute"))
	if err := s.addThings(ctx, uid, gidChip, int64(suipianCount), 9); err != nil {
		return "", err
	}
	itemstr := msgArmorChip + strconv.Itoa(suipianCount)

	cnt, highCnt := 0, 0
	level := model.Int(info, "strong_level")
	if level > 0 {
		probs, err := s.db.FetchRows(ctx, "select * from cfg_strong_probability order by level")
		if err != nil {
			return "", err
		}
		upto := level
		if upto > 10 {
			upto = 10
		}
		for i := 1; i <= upto && i <= len(probs); i++ {
			if sv := model.Int(probs[i-1], "suc_value"); sv > 0 {
				cnt += int(math.Floor(100.0 / float64(sv)))
			}
		}
		for i := 11; i <= level && i <= len(probs); i++ {
			if sv := model.Int(probs[i-1], "suc_value"); sv > 0 {
				highCnt += int(math.Floor(100.0 / float64(sv) * 0.2))
			}
		}
	}
	tempitems := map[int]int{}
	var order []int
	push := func(gid int) {
		if _, ok := tempitems[gid]; !ok {
			order = append(order, gid)
		}
	}
	if cnt > 0 {
		tempitems[gidStrong] = cnt
		order = append(order, gidStrong)
	}
	if highCnt > 0 {
		tempitems[gidHighStrong] = highCnt
		order = append(order, gidHighStrong)
	}
	// 镶嵌宝珠返还：50% 原样，否则降级（1 级宝珠降级分支不返还）
	for _, gs := range strings.Split(model.Str(info, "embed_pearls"), ",") {
		gid := atoi(gs)
		lv := 0
		switch {
		case gid >= 300 && gid <= 379:
			lv = gid%10 + 1
		case gid >= 17500 && gid <= 17539:
			lv = gid%5 + 11
		default:
			continue
		}
		if rand.Intn(100) < 50 {
			push(gid)
			tempitems[gid]++
		} else if mapped, ok := elevenPearlMap[gid]; ok {
			push(mapped)
			if tempitems[mapped] == 0 {
				tempitems[mapped] = 1
			} else {
				tempitems[mapped] += rand.Intn(2) + 1
			}
		} else if lv > 1 {
			push(gid - 1)
			if tempitems[gid-1] == 0 {
				tempitems[gid-1] = 1
			} else {
				tempitems[gid-1] += rand.Intn(2) + 1
			}
		}
	}
	// 删除装备（cutArmorBySid:1390，拆解前已校验 hid=0）
	if _, err := s.db.Exec(ctx, "delete from user_armors where user_id=? and sid=? and hid=0", uid, sid); err != nil {
		return "", err
	}
	if _, err := s.db.Exec(ctx, `insert into log_armor (user_id, armorid, count, time, type)
		values (?,?,-1,unix_timestamp(),11)`, uid, model.Int(info, "armorid")); err != nil {
		return "", err
	}
	for _, gid := range order {
		c := tempitems[gid]
		if c == 0 {
			continue
		}
		name, err := cellStr(ctx, s.db, "select name from cfg_goods where gid=? limit 1", gid)
		if err != nil {
			return "", err
		}
		if name == "" {
			continue // legacy empty($good) → continue
		}
		desc := name + " " + strconv.Itoa(c)
		if itemstr == "" {
			itemstr = desc
		} else {
			itemstr += ", " + desc
		}
		if err := s.addGoods(ctx, uid, gid, int64(c), 23); err != nil {
			return "", err
		}
	}
	msg := msgChaijieNothing
	if itemstr != "" {
		msg = msgChaijieGet + itemstr + msgChaijieEnd
	}
	return msg, nil
}

// getSuipian 对齐 ArmorFunc.php:879。
// 权重：4×等级 + 2×统 + 4×政 + 8×武 + 4×智 + 3×速；ceil(value/10)，0→1。
func getSuipian(heroLevel int, attribute string) int {
	value := heroLevel * 4
	arr := strings.Split(attribute, ",")
	n := atoi(arr[0])
	idx := 1
	for i := 0; i < n && idx+1 < len(arr); i++ {
		item := atoi(arr[idx])
		num := atoi(arr[idx+1])
		idx += 2
		switch item {
		case 1:
			value += num * 2
		case 2:
			value += num * 4
		case 3:
			value += num * 8
		case 4:
			value += num * 4
		case 11:
			value += num * 3
		}
	}
	count := int(math.Ceil(float64(value) / 10))
	if value == 0 {
		count = 1
	}
	return count
}

// ---------- 查询 ----------

// BagArmor 背包一行（user_armors ⨝ cfg_armor）。
type BagArmor struct {
	SID          int    `json:"sid"`
	ArmorID      int    `json:"armorid"`
	Name         string `json:"name"`
	Part         int    `json:"part"`
	Type         int    `json:"type"`
	HeroLevel    int    `json:"hero_level"`
	Value        int    `json:"value"`
	HP           int    `json:"hp"`
	HPMax        int    `json:"hp_max"`
	OriHPMax     int    `json:"ori_hp_max"`
	HID          int    `json:"hid"`
	StrongLevel  int    `json:"strong_level"`
	StrongValue  int    `json:"strong_value"`
	StrongTimes  int    `json:"strong_times"`
	CombineLevel int    `json:"combine_level"`
	EmbedHoles   string `json:"embed_holes"`
	EmbedPearls  string `json:"embed_pearls"`
	BestQuality  string `json:"best_quality"`
	Deified      int    `json:"deified"`
	ActiveSpec   int    `json:"active_special"`
	Attribute    string `json:"attribute"`
	TieID        int    `json:"tieid"`
	Description  string `json:"description"`
}

// LoadUserArmor 对齐 ArmorFunc.php:118 loadUserArmor（背包列表，hid=0）。
// 降级：armor_column 无列恒 0；末日之刃特效 specialAttid/specialValue 无权威数值省略。
func (s *Service) LoadUserArmor(ctx context.Context, uid int) ([]BagArmor, error) {
	rows, err := s.db.FetchRows(ctx, `select a.*, c.name, c.part, c.type, c.hero_level, c.value,
		c.attribute, c.tieid, c.description
		from user_armors a left join cfg_armor c on c.id=a.armorid
		where a.user_id=? and a.hid=0
		order by a.strong_level desc, a.combine_level desc`, uid)
	if err != nil {
		return nil, err
	}
	out := make([]BagArmor, 0, len(rows))
	for _, r := range rows {
		out = append(out, bagArmorOf(r))
	}
	return out, nil
}

// HeroArmor 武将穿戴一行。
type HeroArmor struct {
	SID     int    `json:"sid"`
	Spart   int    `json:"spart"`
	ArmorID int    `json:"armorid"`
	Name    string `json:"name"`
	Part    int    `json:"part"`
	Type    int    `json:"type"`
}

// GetHeroArmor 对齐 ArmorFunc.php:175 doGetHeroArmor（穿戴列表）。
func (s *Service) GetHeroArmor(ctx context.Context, hid int) ([]HeroArmor, error) {
	rows, err := s.db.FetchRows(ctx, `select h.sid, h.spart, h.armorid, c.name, c.part, c.type
		from hero_armors h left join user_armors u on u.sid=h.sid and u.hid=h.hid
		left join cfg_armor c on c.id=h.armorid where h.hid=?`, hid)
	if err != nil {
		return nil, err
	}
	out := make([]HeroArmor, 0, len(rows))
	for _, r := range rows {
		out = append(out, HeroArmor{
			SID: model.Int(r, "sid"), Spart: model.Int(r, "spart"), ArmorID: model.Int(r, "armorid"),
			Name: model.Str(r, "name"), Part: model.Int(r, "part"), Type: model.Int(r, "type"),
		})
	}
	return out, nil
}

func bagArmorOf(r map[string]any) BagArmor {
	return BagArmor{
		SID: model.Int(r, "sid"), ArmorID: model.Int(r, "armorid"), Name: model.Str(r, "name"),
		Part: model.Int(r, "part"), Type: model.Int(r, "type"), HeroLevel: model.Int(r, "hero_level"),
		Value: model.Int(r, "value"), HP: model.Int(r, "hp"), HPMax: model.Int(r, "hp_max"),
		OriHPMax: model.Int(r, "ori_hp_max"), HID: model.Int(r, "hid"),
		StrongLevel: model.Int(r, "strong_level"), StrongValue: model.Int(r, "strong_value"),
		StrongTimes: model.Int(r, "strong_times"), CombineLevel: model.Int(r, "combine_level"),
		EmbedHoles: model.Str(r, "embed_holes"), EmbedPearls: model.Str(r, "embed_pearls"),
		BestQuality: model.Str(r, "best_quality"), Deified: model.Int(r, "deified"),
		ActiveSpec: model.Int(r, "active_special"), Attribute: model.Str(r, "attribute"),
		TieID: model.Int(r, "tieid"), Description: model.Str(r, "description"),
	}
}
