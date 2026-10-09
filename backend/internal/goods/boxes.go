package goods

// boxes.go 复刻 legacy server/game/GoodsFunc.php 的"开礼盒/掉落类道具"分支（1:1，含原版怪癖）。
// 缺表/缺列降级策略（不建表，注释逐函数说明）：
//   - cfg_goods 无 copperbox/silverbox/goldbox/treasurebox/woodbox 权重列 → 随机掉落查询视为空列表，
//     礼盒仍正常扣盒子/钥匙并返回空 []map[string]any；
//   - cfg_pack_goods 无 → openDynamicBox 走 legacy "无配置" 分支 errLegacy("礼包不存在，请与客服联系。")；
//   - cfg_box_details / cfg_box_open_condition / cfg_box_combine_details 无 → 相关分支按 legacy 空结果路径；
//   - cfg_armor / sys_user_armor / sys_user_book / cfg_book / cfg_things 无 → 对应查询按空结果降级；
//   - sys_inform 广播表无 → 省略全部 sendSysInform/sendOpenBoxInform 广播（不发错误）；
//   - users 无 gift 列 → addGoods(gid=0)/addGift 的礼金累加跳过（10010 元宝50 改走 users.money）；
//   - 用户级锁：legacy 函数体内 lockUser/unlockUser 由 Go 版 UseGoods 入口统一 WithUserLock 包裹，此处忽略。

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"strconv"

	"rxsg/backend/internal/model"
)

// ---------- 私有 helper（box 前缀，避免与并行文件冲突） ----------

// boxRandInt 对齐 PHP mt_rand(a,b)/rand(a,b)：含端点。
func boxRandInt(min, max int64) int64 {
	if max < min {
		return 0 // PHP mt_rand(1,0) 告警返回 false→0（原版无掉落语义）
	}
	return rand.Int63n(max-min+1) + min
}

// boxDropRate 对齐 legacy dropRate 行 {gid, rate, value}。
type boxDropRate struct {
	gid   int
	rate  int64
	value int64
}

// boxGoodsPair 保序掉落对（PHP 关联数组 foreach 按插入序，Go map 无序，必须用切片）。
type boxGoodsPair struct {
	gid int
	cnt int64
}

// boxCfgGoodsRow 对齐 sql_fetch_one("select *,$cnt as count from cfg_goods where gid='$gid'")；
// 无行时 legacy 返回 false（JSON null），Go 用 nil map 表示。
func (s *Service) boxCfgGoodsRow(ctx context.Context, gid int, cnt int64) (map[string]any, error) {
	row, err := s.db.FetchOne(ctx, "select *, ? as `count` from cfg_goods where gid=?", cnt, gid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return row, err
}

// boxCfgGoodsName 对齐 sql_fetch_one_cell("select name from cfg_goods where gid=...")；无行→""。
func (s *Service) boxCfgGoodsName(ctx context.Context, gid int) (string, error) {
	v, err := s.db.FetchCellString(ctx, "select `name` from cfg_goods where gid=?", gid)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// boxGetValueRand 对齐 GoodsFunc.php:2731 getValueRand（mt_rand()%100 分段）。
func boxGetValueRand(min1, max1, min2, max2, min3, max3, min4, max4 int64) int64 {
	rnd := rand.Intn(100) // mt_rand() % 100
	switch {
	case rnd < 65:
		return boxRandInt(min1, max1)
	case rnd < 80:
		return boxRandInt(min2, max2)
	case rnd < 95:
		return boxRandInt(min3, max3)
	default:
		return boxRandInt(min4, max4)
	}
}

// boxOpenTreasureBox 对齐 GoodsFunc.php:2662 openTreasureBox。
// 降级：cfg_goods 无 *box 权重列 → dropRate 恒空 → 直接返回空列表
// （legacy 空 dropRate 时还会 addGift(users.gift 列缺失→跳过) 并 push gid=0 行的 false，按降级规范返回空）。
func (s *Service) boxOpenTreasureBox(ctx context.Context, uid int, totalValue int64, dropRate []boxDropRate, typ int) ([]map[string]any, error) {
	ret := make([]map[string]any, 0)
	if len(dropRate) == 0 {
		return ret, nil
	}
	dropCount := 9
	tryCount := 50
	var order []int
	goodsGet := map[int]int64{}
	for totalValue > 0 && dropCount > 0 && len(dropRate) > 0 && tryCount > 0 {
		allrate := int64(0)
		for _, g := range dropRate {
			allrate += g.rate
		}
		rnd := rand.Int63n(allrate) // mt_rand() % allrate
		ratesum := int64(0)
		tryCount--
		for i := 0; i < len(dropRate); i++ {
			g := dropRate[i]
			ratesum += g.rate
			if rnd < ratesum {
				countmax := int64(0)
				if g.value > 0 {
					countmax = totalValue / g.value // floor；value=0 → PHP 除零告警→无掉落，Go 记 0
				}
				var cnt int64
				if g.rate <= 5 {
					cnt = 1
				} else if countmax > 0 {
					hi := (countmax + 1) / 2 // ceil(countmax/2)
					cnt = boxRandInt(1, hi)
				}
				if cnt > 0 && countmax > 0 {
					if _, ok := goodsGet[g.gid]; !ok {
						order = append(order, g.gid)
					}
					goodsGet[g.gid] = cnt
					totalValue -= g.value * cnt
					dropCount--
					dropRate = append(dropRate[:i], dropRate[i+1:]...)
					break
				}
			}
		}
	}
	if totalValue > 0 && typ > 0 {
		switch typ {
		case 1:
			if totalValue > 10 {
				totalValue = boxRandInt(1, 10)
			}
		case 2:
			if totalValue > 25 {
				totalValue = boxRandInt(1, 25)
			}
		}
		// addGift($uid,$totalValue,20)：users 无 gift 列 → 跳过礼金累加（降级）。
		if _, ok := goodsGet[0]; !ok {
			order = append(order, 0)
		}
		goodsGet[0] = totalValue
	}
	for _, gid := range order {
		cnt := goodsGet[gid]
		if gid > 0 {
			if err := s.AddGoods(ctx, uid, gid, cnt, 3); err != nil {
				return nil, err
			}
		}
		// gid==0：legacy addGoods 走 gift 累加分支（gift 列缺失→跳过）。
		row, err := s.boxCfgGoodsRow(ctx, gid, cnt)
		if err != nil {
			return nil, err
		}
		ret = append(ret, row)
		// isSentGood/sendOpenBoxInform：sys_inform 广播表缺失 → 省略。
	}
	return ret, nil
}

// boxOpenSimpleGoodsBox 对齐 GoodsFunc.php:2001 openSimpleGoodsBox。
// 降级：dropRate 恒空（权重列缺失）→ 空列表；legacy 尾部 checkAndDoSimpleBoxAct 依赖 cfg_act（无表）→ 无活动奖励。
func (s *Service) boxOpenSimpleGoodsBox(ctx context.Context, uid int, dropRate []boxDropRate, typ int) ([]map[string]any, error) {
	ret := make([]map[string]any, 0)
	if len(dropRate) == 0 {
		return ret, nil
	}
	allrate := int64(0)
	for _, g := range dropRate {
		allrate += g.rate
	}
	rnd := rand.Int63n(allrate)
	ratesum := int64(0)
	cnt := int64(1)
	for i := 0; i < len(dropRate); i++ {
		g := dropRate[i]
		ratesum += g.rate
		if rnd < ratesum {
			if err := s.AddGoods(ctx, uid, g.gid, cnt, 3); err != nil {
				return nil, err
			}
			row, err := s.boxCfgGoodsRow(ctx, g.gid, cnt)
			if err != nil {
				return nil, err
			}
			ret = append(ret, row)
			// sendOpenBoxInform：sys_inform 缺失 → 省略。
			break
		}
	}
	return ret, nil
}

// boxOpenCopperBox 对齐 GoodsFunc.php:2743 openCopperBox（copperbox 权重列缺失→空掉落）。
func (s *Service) boxOpenCopperBox(ctx context.Context, uid int) ([]map[string]any, error) {
	totalValue := boxGetValueRand(21, 30, 31, 35, 15, 20, 36, 60)
	return s.boxOpenTreasureBox(ctx, uid, totalValue, nil, 0)
}

// boxOpenSilverBox 对齐 GoodsFunc.php:2749 openSilverBox（silverbox 列缺失→空掉落）。
func (s *Service) boxOpenSilverBox(ctx context.Context, uid, typ int) ([]map[string]any, error) {
	totalValue := boxGetValueRand(121, 180, 181, 210, 90, 120, 211, 360)
	return s.boxOpenTreasureBox(ctx, uid, totalValue, nil, typ)
}

// boxOpenGoldBox 对齐 GoodsFunc.php:2756 openGoldBox（goldbox 列缺失→空掉落）。
func (s *Service) boxOpenGoldBox(ctx context.Context, uid int) ([]map[string]any, error) {
	totalValue := boxGetValueRand(321, 480, 481, 560, 240, 320, 561, 960)
	return s.boxOpenTreasureBox(ctx, uid, totalValue, nil, 2)
}

// boxOpenTreasure 对齐 GoodsFunc.php:2763 openTreasure（treasurebox 列缺失→空掉落）。
func (s *Service) boxOpenTreasure(ctx context.Context, uid int) ([]map[string]any, error) {
	return s.boxOpenSimpleGoodsBox(ctx, uid, nil, 119)
}

// boxOpenOldWoodBox 对齐 GoodsFunc.php:2836 openOldWoodBox（woodbox 列缺失→空掉落）。
// 掉落首件 gid==18 的 finishAchivement(uid,7) 属 M8，未接线；空列表时无首件，不触发。
func (s *Service) boxOpenOldWoodBox(ctx context.Context, uid int) ([]map[string]any, error) {
	return s.boxOpenSimpleGoodsBox(ctx, uid, nil, 50)
}

// ---------- 金砖/金条/辎重包 ----------

// openGoldBar 对齐 GoodsFunc.php:3224。
// 原版怪癖保留：goodCnt>50 截断为 50 后 useCount=goodCnt 覆盖入参（一次最多开 50 个）。
func (s *Service) openGoldBar(ctx context.Context, uid, cid, gid, useCount int) (string, error) {
	goodCnt, err := s.goodsCount(ctx, uid, gid)
	if err != nil {
		return "", err
	}
	if goodCnt == 0 || goodCnt < int64(useCount) {
		return "", errLegacy("当前物品不足")
	}
	if goodCnt > 50 {
		goodCnt = 50
	}
	useCount = int(goodCnt)
	goldAdd := int64(1000000) * int64(useCount)
	if gid == 85 {
		goldAdd = int64(100000) * int64(useCount)
	}
	if err := s.addCityResources(ctx, cid, 0, 0, 0, 0, goldAdd); err != nil {
		return "", err
	}
	if err := s.ReduceGoods(ctx, uid, gid, int64(useCount), 0); err != nil {
		return "", err
	}
	return fmt.Sprintf("获得黄金%d", goldAdd), nil
}

// openResBox 对齐 GoodsFunc.php:3239。87-90→100000×、91-94→1000000×；
// addCityResources 参数位 (wood,rock,iron,food,gold)：87/91 粮、88/92 木、89/93 石、90/94 铁。
// 原版怪癖保留：gid 不在 87-94 时 $msg 未定义→返回空串。
func (s *Service) openResBox(ctx context.Context, uid, cid, gid, useCount int) (string, error) {
	goodCnt, err := s.goodsCount(ctx, uid, gid)
	if err != nil {
		return "", err
	}
	if goodCnt == 0 || goodCnt < int64(useCount) {
		return "", errLegacy("当前物品不足")
	}
	msg := ""
	resAdd := int64(0)
	switch gid {
	case 87:
		resAdd = 100000 * int64(useCount)
		err = s.addCityResources(ctx, cid, 0, 0, 0, resAdd, 0)
		msg = fmt.Sprintf("获得粮食%d", resAdd)
	case 88:
		resAdd = 100000 * int64(useCount)
		err = s.addCityResources(ctx, cid, resAdd, 0, 0, 0, 0)
		msg = fmt.Sprintf("获得木材%d", resAdd)
	case 89:
		resAdd = 100000 * int64(useCount)
		err = s.addCityResources(ctx, cid, 0, resAdd, 0, 0, 0)
		msg = fmt.Sprintf("获得石料%d", resAdd)
	case 90:
		resAdd = 100000 * int64(useCount)
		err = s.addCityResources(ctx, cid, 0, 0, resAdd, 0, 0)
		msg = fmt.Sprintf("获得铁锭%d", resAdd)
	case 91:
		resAdd = 1000000 * int64(useCount)
		err = s.addCityResources(ctx, cid, 0, 0, 0, resAdd, 0)
		msg = fmt.Sprintf("获得粮食%d", resAdd)
	case 92:
		resAdd = 1000000 * int64(useCount)
		err = s.addCityResources(ctx, cid, resAdd, 0, 0, 0, 0)
		msg = fmt.Sprintf("获得木材%d", resAdd)
	case 93:
		resAdd = 1000000 * int64(useCount)
		err = s.addCityResources(ctx, cid, 0, resAdd, 0, 0, 0)
		msg = fmt.Sprintf("获得石料%d", resAdd)
	case 94:
		resAdd = 1000000 * int64(useCount)
		err = s.addCityResources(ctx, cid, 0, 0, resAdd, 0, 0)
		msg = fmt.Sprintf("获得铁锭%d", resAdd)
	}
	if err != nil {
		return "", err
	}
	if err := s.ReduceGoods(ctx, uid, gid, int64(useCount), 0); err != nil {
		return "", err
	}
	return msg, nil
}

// ---------- 礼包 ----------

// useResourcePackage 对齐 GoodsFunc.php:3372。
func (s *Service) useResourcePackage(ctx context.Context, uid, gid int) (string, error) {
	ok, err := s.checkGoods(ctx, uid, gid)
	if err != nil {
		return "", err
	}
	if !ok {
		name, err := s.boxCfgGoodsName(ctx, gid)
		if err != nil {
			return "", err
		}
		return "", errLegacy(fmt.Sprintf("你没有%s，不能使用。", name))
	}
	cid, err := s.lastCityID(ctx, uid)
	if err != nil {
		return "", err
	}
	if err := s.addCityResources(ctx, cid, 100000, 100000, 100000, 100000, 10000); err != nil {
		return "", err
	}
	if err := s.ReduceGoods(ctx, uid, gid, 1, 0); err != nil {
		return "", err
	}
	return "获得黄金10000，粮食100000，木材100000，石料100000，铁锭100000。", nil
}

// openDynamicBox 对齐 GoodsFunc.php:2566。
// 降级：cfg_pack_goods 表缺失 → legacy record 恒空 → errLegacy("礼包不存在，请与客服联系。")。
func (s *Service) openDynamicBox(ctx context.Context, uid, gid int) ([]map[string]any, error) {
	return nil, errLegacy("礼包不存在，请与客服联系。")
}

// useHuodongGoods 对齐 GoodsFunc.php:3386（GID>10000 活动宝物）。
// 降级：10010 addGift(50,2) 的 users.gift 列缺失 → 元宝 50 改走 users.money；
// 广播/成就省略。官府等级查新库 buildings(city_id,building_id=6)。
func (s *Service) useHuodongGoods(ctx context.Context, uid, gid int) ([]map[string]any, error) {
	ok, err := s.checkGoods(ctx, uid, gid)
	if err != nil {
		return nil, err
	}
	if !ok {
		name, err := s.boxCfgGoodsName(ctx, gid)
		if err != nil {
			return nil, err
		}
		return nil, errLegacy(fmt.Sprintf("你没有%s，不能使用。", name))
	}
	cid, err := s.lastCityID(ctx, uid)
	if err != nil {
		return nil, err
	}
	governmentLevel, err := s.db.FetchCellInt64(ctx,
		"select level from buildings where city_id=? and building_id=6", cid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if errors.Is(err, sql.ErrNoRows) {
		governmentLevel = 0
	}

	ret := make([]map[string]any, 0)
	var goodslist []boxGoodsPair
	switch gid {
	case 10001: // 新手礼包
		return s.openDynamicBox(ctx, uid, gid)
	case 10002: // 升级礼包，需要官府3
		if governmentLevel < 3 {
			return nil, errLegacy("你的官府等级不足3级，不能打开升级礼包。")
		}
		return s.openDynamicBox(ctx, uid, gid)
	case 10010: // 功勋礼包
		if governmentLevel < 3 {
			return nil, errLegacy("你的官府等级不足3级，不能打开功勋礼包。")
		}
		// addGift($uid,50,2)：users 无 gift 列 → 元宝 50 走 users.money（降级约定）。
		if _, err := s.db.Exec(ctx, "update users set money=money+50 where id=?", uid); err != nil {
			return nil, err
		}
		ret = append(ret, map[string]any{"name": "元宝", "count": "50", "gid": 0})
	case 10011, 10012, 10018, 10019: // 生产/高级生产/建设/城主礼包
		return s.openDynamicBox(ctx, uid, gid)
	case 10016: // 伯乐包
		goodslist = []boxGoodsPair{{22, 10}, {23, 10}}
	case 10020: // 天御礼包：八卦阵图1、智多星1、虎符1
		goodslist = []boxGoodsPair{{7, 1}, {29, 1}, {26, 1}}
	case 10021: // 武神礼包：陷阵战鼓1、武曲星1、虎符1
		goodslist = []boxGoodsPair{{6, 1}, {28, 1}, {26, 1}}
	case 10022: // 遁世礼包：迁城令2、免战牌2
		goodslist = []boxGoodsPair{{24, 2}, {12, 2}}
	case 10023: // 中包洗髓丹：洗髓丹5
		goodslist = []boxGoodsPair{{22, 5}}
	case 10024: // 中包锦囊：锦囊20
		goodslist = []boxGoodsPair{{13, 20}}
	case 10072: // "礼上加礼"新手大礼包：需脱离新手保护
		state, err := s.db.FetchCellInt64(ctx, "select state from users where id=?", uid)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if state == 1 {
			return nil, errLegacy("你未脱离新手保护期，不能打开礼包。")
		}
		goodslist = []boxGoodsPair{
			{1, 1}, {2, 1}, {3, 1}, {4, 1}, {5, 1}, {54, 1}, {12, 3}, {56, 1},
			{24, 3}, {67, 1}, {68, 1}, {69, 1}, {8, 1}, {65, 1}, {9, 1}, {63, 1},
			{64, 1}, {40, 1},
		}
	}
	for _, p := range goodslist {
		if err := s.AddGoods(ctx, uid, p.gid, p.cnt, 6); err != nil {
			return nil, err
		}
		row, err := s.boxCfgGoodsRow(ctx, p.gid, p.cnt)
		if err != nil {
			return nil, err
		}
		ret = append(ret, row)
	}
	if err := s.ReduceGoods(ctx, uid, gid, 1, 0); err != nil {
		return nil, err
	}
	return ret, nil
}

// ---------- 青铜/白银/黄金/宝藏/木盒/答题 ----------

// useCopperBox 对齐 GoodsFunc.php:2767（1% 变白银盒；青铜盒/钥匙各扣 1）。
func (s *Service) useCopperBox(ctx context.Context, uid int) ([]map[string]any, error) {
	ok, err := s.checkGoods(ctx, uid, 16)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errLegacy("你没有青铜礼匣，不能使用钥匙。")
	}
	ok, err = s.checkGoods(ctx, uid, 19)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errLegacy("你没有青铜钥匙，不能打开礼匣。")
	}
	var ret []map[string]any
	if rand.Intn(100) == 0 { // mt_rand()%100==0 → 1%
		ret, err = s.boxOpenSilverBox(ctx, uid, 0)
	} else {
		ret, err = s.boxOpenCopperBox(ctx, uid)
	}
	if err != nil {
		return nil, err
	}
	if err := s.ReduceGoods(ctx, uid, 16, 1, 0); err != nil {
		return nil, err
	}
	if err := s.ReduceGoods(ctx, uid, 19, 1, 0); err != nil {
		return nil, err
	}
	return ret, nil
}

// useSilverBox 对齐 GoodsFunc.php:2787（1% 变黄金盒；白银盒/钥匙各扣 1）。
func (s *Service) useSilverBox(ctx context.Context, uid int) ([]map[string]any, error) {
	ok, err := s.checkGoods(ctx, uid, 17)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errLegacy("你没有白银礼匣，不能使用钥匙。")
	}
	ok, err = s.checkGoods(ctx, uid, 20)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errLegacy("你没有白银钥匙，不能打开礼匣。")
	}
	var ret []map[string]any
	if rand.Intn(100) == 0 {
		ret, err = s.boxOpenGoldBox(ctx, uid)
	} else {
		ret, err = s.boxOpenSilverBox(ctx, uid, 1) // openSilverBox 默认 $type=1
	}
	if err != nil {
		return nil, err
	}
	if err := s.ReduceGoods(ctx, uid, 17, 1, 0); err != nil {
		return nil, err
	}
	if err := s.ReduceGoods(ctx, uid, 20, 1, 0); err != nil {
		return nil, err
	}
	return ret, nil
}

// useGoldBox 对齐 GoodsFunc.php:2818（无互转怪癖；黄金盒/钥匙各扣 1）。
func (s *Service) useGoldBox(ctx context.Context, uid int) ([]map[string]any, error) {
	ok, err := s.checkGoods(ctx, uid, 18)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errLegacy("你没有黄金礼匣，不能使用钥匙。")
	}
	ok, err = s.checkGoods(ctx, uid, 21)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errLegacy("你没有黄金钥匙，不能打开礼匣。")
	}
	ret, err := s.boxOpenGoldBox(ctx, uid)
	if err != nil {
		return nil, err
	}
	if err := s.ReduceGoods(ctx, uid, 18, 1, 0); err != nil {
		return nil, err
	}
	if err := s.ReduceGoods(ctx, uid, 21, 1, 0); err != nil {
		return nil, err
	}
	return ret, nil
}

// useTreasureBox 对齐 GoodsFunc.php:2807（宝藏盒 119，无钥匙；gid 入参 legacy 未用，签名按 dispatch）。
func (s *Service) useTreasureBox(ctx context.Context, uid, gid int) ([]map[string]any, error) {
	ok, err := s.checkGoods(ctx, uid, 119)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errLegacy("你没有宝藏匣，不能使用。")
	}
	ret, err := s.boxOpenTreasure(ctx, uid)
	if err != nil {
		return nil, err
	}
	if err := s.ReduceGoods(ctx, uid, 119, 1, 0); err != nil {
		return nil, err
	}
	return ret, nil
}

// useOldWoodBox 对齐 GoodsFunc.php:2850（古朴木盒 50）。
func (s *Service) useOldWoodBox(ctx context.Context, uid int) ([]map[string]any, error) {
	ok, err := s.checkGoods(ctx, uid, 50)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errLegacy("你没有古朴木匣，不能使用。")
	}
	ret, err := s.boxOpenOldWoodBox(ctx, uid)
	if err != nil {
		return nil, err
	}
	if err := s.ReduceGoods(ctx, uid, 50, 1, 0); err != nil {
		return nil, err
	}
	return ret, nil
}

// useDaTiLiBao 对齐 GoodsFunc.php:2859（答题礼包 150，复用 openOldWoodBox 掉落）。
func (s *Service) useDaTiLiBao(ctx context.Context, uid int) ([]map[string]any, error) {
	ok, err := s.checkGoods(ctx, uid, 150)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errLegacy("你没有答题礼包，不能使用。")
	}
	ret, err := s.boxOpenOldWoodBox(ctx, uid)
	if err != nil {
		return nil, err
	}
	if err := s.ReduceGoods(ctx, uid, 150, 1, 0); err != nil {
		return nil, err
	}
	return ret, nil
}

// ---------- 钥匙链/碎片盒/宝珠盒/宝石箱 ----------

// useKeyChain 对齐 GoodsFunc.php:2868（10017，10 连开：rnd<5→21、<55→20、否则 19）。
// 原版怪癖保留：lang.php 未定义 $GLOBALS['useKeyChain']['no_KeyChain'] → PHP null → 异常消息为空串。
func (s *Service) useKeyChain(ctx context.Context, uid int) ([]map[string]any, error) {
	mygid := 10017
	ok, err := s.checkGoods(ctx, uid, mygid)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errLegacy("")
	}
	dropCount := 10
	var order []int
	goodsGet := map[int]int64{}
	for dropCount > 0 {
		dropCount--
		rnd := rand.Intn(1000) // mt_rand() % 1000
		gid := 19
		if rnd < 5 {
			gid = 21
		} else if rnd < 55 {
			gid = 20
		}
		if _, exists := goodsGet[gid]; !exists {
			order = append(order, gid)
		}
		goodsGet[gid]++
	}
	ret := make([]map[string]any, 0)
	for _, gid := range order {
		cnt := goodsGet[gid]
		if err := s.AddGoods(ctx, uid, gid, cnt, 3); err != nil {
			return nil, err
		}
		row, err := s.boxCfgGoodsRow(ctx, gid, cnt)
		if err != nil {
			return nil, err
		}
		ret = append(ret, row)
	}
	if err := s.ReduceGoods(ctx, uid, mygid, 1, 0); err != nil {
		return nil, err
	}
	return ret, nil
}

// openSuiPianHe 对齐 GoodsFunc.php:2915。
// 原版怪癖保留：mt_rand(0,sizeof($goods)) 上界越界 → 取到 $goods[count] 时 PHP 未定义索引→null→choose=0。
// 降级：种子 cfg_goods 无 100000-101026 段 → 列表恒空 → choose=0 → addGoods 走 gift 分支（gift 列缺失→跳过）；
// enter_6 广播（sys_inform 缺失）省略。
func (s *Service) openSuiPianHe(ctx context.Context, uid, gid int) ([]map[string]any, error) {
	ok, err := s.checkGoods(ctx, uid, gid)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errLegacy("你没有宝珠箱。")
	}
	goods, err := s.db.FetchRows(ctx, "select gid,name from cfg_goods where gid between 100000 and 101026")
	if err != nil {
		return nil, err
	}
	choose := 0
	idx := boxRandInt(0, int64(len(goods))) // mt_rand(0,count)，含越界怪癖
	if idx < int64(len(goods)) {
		choose = model.Int(goods[idx], "gid")
	}
	if choose != 0 {
		if err := s.AddGoods(ctx, uid, choose, 1, 3); err != nil {
			return nil, err
		}
	}
	ret := make([]map[string]any, 0)
	row, err := s.boxCfgGoodsRow(ctx, choose, 1)
	if err != nil {
		return nil, err
	}
	ret = append(ret, row)
	if err := s.ReduceGoods(ctx, uid, gid, 1, 0); err != nil {
		return nil, err
	}
	return ret, nil
}

// openBazhuHe 对齐 GoodsFunc.php:2942（权重 {1:45,2:35,3:15,4:5} → gid="3"+rand(0,7)+selected）。
// 原版怪癖保留：rand(1,100)=100 时不命中任何档 → selected 保持初始 1。
// 降级：种子无 3xx 段 cfg_goods → 行返回 null；enter_8 广播（sys_inform 缺失）省略。
func (s *Service) openBazhuHe(ctx context.Context, uid, gid int) ([]map[string]any, error) {
	ok, err := s.checkGoods(ctx, uid, gid)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errLegacy("你没有宝珠箱。")
	}
	goodrate := []boxGoodsPair{{1, 45}, {2, 35}, {3, 15}, {4, 5}}
	rateNum := boxRandInt(1, 100) // PHP rand(1,100)
	totalrate := int64(0)
	selected := int64(1)
	for _, g := range goodrate {
		totalrate += g.cnt
		if rateNum < totalrate {
			selected = int64(g.gid)
			break
		}
	}
	selected--
	choose := 300 + int(boxRandInt(0, 7))*10 + int(selected) // "3"+mt_rand(0,7)+selected
	if err := s.AddGoods(ctx, uid, choose, 1, 3); err != nil {
		return nil, err
	}
	ret := make([]map[string]any, 0)
	row, err := s.boxCfgGoodsRow(ctx, choose, 1)
	if err != nil {
		return nil, err
	}
	ret = append(ret, row)
	if err := s.ReduceGoods(ctx, uid, gid, 1, 0); err != nil {
		return nil, err
	}
	return ret, nil
}

// openBaozhuhe 对齐 GoodsFunc.php:2983（{209:15,210:15,211:15,1:55}；选中 1 → 300+(rand(1,8)-1)*10）。
// 降级：种子无 209/210/211/30x → 行返回 null。
func (s *Service) openBaozhuhe(ctx context.Context, uid, gid int) ([]map[string]any, error) {
	ok, err := s.checkGoods(ctx, uid, gid)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errLegacy("你没有宝珠箱。")
	}
	gems := []boxGoodsPair{{209, 15}, {210, 15}, {211, 15}, {1, 55}}
	choose := boxPickFirstLE(gems, boxRandInt(1, 100), 1)
	if choose == 1 {
		typ := boxRandInt(1, 8)
		choose = 300 + int(typ-1)*10 + choose - 1 // "只能这么写。。。"
	}
	if err := s.AddGoods(ctx, uid, choose, 1, 3); err != nil {
		return nil, err
	}
	ret := make([]map[string]any, 0)
	row, err := s.boxCfgGoodsRow(ctx, choose, 1)
	if err != nil {
		return nil, err
	}
	ret = append(ret, row)
	if err := s.ReduceGoods(ctx, uid, gid, 1, 0); err != nil {
		return nil, err
	}
	return ret, nil
}

// openBaoshiBox 对齐 GoodsFunc.php:3010（{209:13,210:13,211:13,1:50,2:8,3:3}；
// 选中 1-3 → gid=300+(rand(1,8)-1)*10+choose-1；209-211 → 数量+2）。
// 原版怪癖保留：ret 行的 count 恒为 1（即使实际入包 2 个）。
// 降级：种子无 209/210/211/30x → 行返回 null。
func (s *Service) openBaoshiBox(ctx context.Context, uid, gid int) ([]map[string]any, error) {
	ok, err := s.checkGoods(ctx, uid, gid)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errLegacy("你没有宝珠箱。")
	}
	gems := []boxGoodsPair{{209, 13}, {210, 13}, {211, 13}, {1, 50}, {2, 8}, {3, 3}}
	choose := boxPickFirstLE(gems, boxRandInt(1, 100), 1)
	if choose >= 1 && choose <= 3 {
		typ := boxRandInt(1, 8)
		choose = 300 + int(typ-1)*10 + choose - 1
	}
	cnt := int64(1)
	if choose == 209 || choose == 210 || choose == 211 {
		cnt = 2
	}
	if err := s.AddGoods(ctx, uid, choose, cnt, 3); err != nil {
		return nil, err
	}
	ret := make([]map[string]any, 0)
	row, err := s.boxCfgGoodsRow(ctx, choose, 1) // 怪癖：count 恒 1
	if err != nil {
		return nil, err
	}
	ret = append(ret, row)
	if err := s.ReduceGoods(ctx, uid, gid, 1, 0); err != nil {
		return nil, err
	}
	return ret, nil
}

// boxPickFirstLE 权重累加选档：randnum<=累计和 即选中；未命中返回 initial（对齐 PHP $choose=1 初始值）。
func boxPickFirstLE(pairs []boxGoodsPair, randnum, initial int64) int {
	sum := int64(0)
	for _, p := range pairs {
		sum += p.cnt
		if randnum <= sum {
			return p.gid
		}
	}
	return int(initial)
}

// ---------- 珠宝盒/玄冰匣/彩蛋盒 ----------

// openGemBox 对齐 GoodsFunc.php:3044（41/42/43；dropCount=3、tryCount=20）。
// 降级：种子 cfg_goods 无 30-38 → value=0 → countmax=0（PHP 除零告警→mt_rand(1,false)→无掉落）→ 实际不掉落，只扣盒。
func (s *Service) openGemBox(ctx context.Context, uid, gid int) ([]map[string]any, error) {
	var gems []boxGoodsPair
	totalValue := int64(0)
	switch gid {
	case 41:
		gems = []boxGoodsPair{{30, 9}, {31, 8}, {32, 7}}
		totalValue = 80
	case 42:
		gems = []boxGoodsPair{{33, 12}, {34, 10}, {35, 8}}
		totalValue = 140
	case 43:
		gems = []boxGoodsPair{{36, 15}, {37, 10}, {38, 5}}
		totalValue = 240
	}
	dropRate := make([]boxDropRate, 0, len(gems))
	for _, g := range gems {
		v, err := s.db.FetchCellInt64(ctx, "select value from cfg_goods where gid=?", g.gid)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		dropRate = append(dropRate, boxDropRate{gid: g.gid, rate: g.cnt, value: v})
	}

	dropCount := 3
	tryCount := 20
	var order []int
	goodsGet := map[int]int64{}
	for totalValue > 0 && dropCount > 0 && len(dropRate) > 0 && tryCount > 0 {
		allrate := int64(0)
		for _, g := range dropRate {
			allrate += g.rate
		}
		rnd := rand.Int63n(allrate)
		ratesum := int64(0)
		tryCount--
		for i := 0; i < len(dropRate); i++ {
			g := dropRate[i]
			ratesum += g.rate
			if rnd < ratesum {
				countmax := int64(0)
				if g.value > 0 {
					countmax = totalValue / g.value
				}
				var cnt int64
				if g.rate <= 5 {
					cnt = 1
				} else {
					cnt = boxRandInt(1, countmax) // countmax=0 → PHP mt_rand(1,0) false→0 → 无掉落
				}
				if cnt > 0 && countmax > 0 {
					if _, exists := goodsGet[g.gid]; !exists {
						order = append(order, g.gid)
					}
					goodsGet[g.gid] = cnt
					totalValue -= g.value * cnt
					dropCount--
					dropRate = append(dropRate[:i], dropRate[i+1:]...)
					break
				}
			}
		}
	}
	ret := make([]map[string]any, 0)
	for _, gid := range order {
		cnt := goodsGet[gid]
		if err := s.AddGoods(ctx, uid, gid, cnt, 3); err != nil {
			return nil, err
		}
		row, err := s.boxCfgGoodsRow(ctx, gid, cnt)
		if err != nil {
			return nil, err
		}
		ret = append(ret, row)
	}
	if err := s.ReduceGoods(ctx, uid, gid, 1, 0); err != nil {
		return nil, err
	}
	return ret, nil
}

// openXuanBinBox 对齐 GoodsFunc.php:3108。
// 原版怪癖保留：10487（竹编箱子）在 legacy 无配置分支 → $sumrate/$rewardrates 未定义 →
// mt_rand(0,null)=0、foreach null 空转、$reward empty → goodsGet 空 → 只扣盒不掉落。
// 10428：droprate=mt_rand(0,100) 含端点 → ==100 时命中 reward1（礼金999，1%）。
// 降级：礼金 gid=0 → legacy gift 累加（users.gift 缺失→跳过）；10500 无种子 → 行返回 null。
func (s *Service) openXuanBinBox(ctx context.Context, uid, gid int) ([]map[string]any, error) {
	var rewards [][]boxGoodsPair
	var rewardrates []int64
	sumrate := int64(0)
	if gid == 10428 {
		rewards = [][]boxGoodsPair{
			{{10500, 1}},
			{{10500, 1}, {0, 999}},
		}
		rewardrates = []int64{99, 1}
		sumrate = 100
	}
	droprate := boxRandInt(0, sumrate) // mt_rand(0,$sumrate)；sumrate=0 → 恒 0
	reward := []boxGoodsPair(nil)
	cursum := int64(0)
	for i, rate := range rewardrates {
		cursum += rate
		if cursum >= droprate {
			reward = rewards[i]
			break
		}
	}
	goodsGet := reward
	if len(goodsGet) == 0 {
		goodsGet = nil
	}
	ret := make([]map[string]any, 0)
	for _, p := range goodsGet {
		if p.gid != 0 {
			if err := s.AddGoods(ctx, uid, p.gid, p.cnt, 3); err != nil {
				return nil, err
			}
		}
		// gid==0：legacy addGoods 走 gift 累加分支（gift 列缺失→跳过）。
		row, err := s.boxCfgGoodsRow(ctx, p.gid, p.cnt)
		if err != nil {
			return nil, err
		}
		ret = append(ret, row)
	}
	if err := s.ReduceGoods(ctx, uid, gid, 1, 0); err != nil {
		return nil, err
	}
	return ret, nil
}

// openEggBox 对齐 GoodsFunc.php:4722。
// 原版怪癖保留：gid 非 10303/10304 时不检查砸蛋锤，但结尾仍无条件 reduceGoods(10307,1)。
// 降级：10305/10306/205/203/155 无种子 → 对应行返回 null。
func (s *Service) openEggBox(ctx context.Context, uid, gid int) ([]map[string]any, error) {
	ok, err := s.checkGoods(ctx, uid, gid)
	if err != nil {
		return nil, err
	}
	if !ok {
		name, err := s.boxCfgGoodsName(ctx, gid)
		if err != nil {
			return nil, err
		}
		return nil, errLegacy(fmt.Sprintf("你没有%s,不能使用。", name))
	}
	if gid == 10303 {
		ok, err := s.checkGoods(ctx, uid, 10307)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errLegacy("你没有砸蛋锤，不能使用银蛋。")
		}
	} else if gid == 10304 {
		ok, err := s.checkGoods(ctx, uid, 10307)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errLegacy("你没有砸蛋锤，不能使用金蛋。")
		}
	}
	ret := make([]map[string]any, 0)
	if gid == 10303 {
		rnd := boxRandInt(1, 100)
		count := int64(3)
		if rnd <= 60 {
			count = 1
		} else if rnd <= 90 {
			count = 2
		}
		for _, p := range []boxGoodsPair{{10305, count}, {23, 2}} {
			row, err := s.boxCfgGoodsRow(ctx, p.gid, p.cnt)
			if err != nil {
				return nil, err
			}
			ret = append(ret, row)
		}
		if err := s.AddGoods(ctx, uid, 10305, count, 6); err != nil {
			return nil, err
		}
		if err := s.AddGoods(ctx, uid, 23, 2, 6); err != nil {
			return nil, err
		}
	} else if gid == 10304 {
		rnd := boxRandInt(1, 100)
		count := int64(3)
		if rnd <= 50 {
			count = 1
		} else if rnd <= 95 {
			count = 2
		}
		drops := []boxGoodsPair{{10306, count}, {205, 2}, {203, 1}, {155, 1}, {22, 1}}
		for _, p := range drops {
			row, err := s.boxCfgGoodsRow(ctx, p.gid, p.cnt)
			if err != nil {
				return nil, err
			}
			ret = append(ret, row)
		}
		for _, p := range drops {
			if err := s.AddGoods(ctx, uid, p.gid, p.cnt, 6); err != nil {
				return nil, err
			}
		}
	}
	if err := s.ReduceGoods(ctx, uid, gid, 1, 0); err != nil {
		return nil, err
	}
	if err := s.ReduceGoods(ctx, uid, 10307, 1, 0); err != nil {
		return nil, err
	}
	return ret, nil
}

// ---------- 装备箱/技能书匣 ----------

// useArmorBox 对齐 GoodsFunc.php:3291。
// 降级：users 无 armor_column 列→0，sys_user_armor 表缺失→curCount=0；0>=0 恒成立 →
// checkGoods 通过后直接 errLegacy(armor_box_full)，后续 cfg_armor 掉落逻辑不可达。
func (s *Service) useArmorBox(ctx context.Context, uid, gid int) ([]map[string]any, error) {
	ok, err := s.checkGoods(ctx, uid, gid)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errLegacy("你拥有的该道具数量为0，请去商城购买后再使用。")
	}
	return nil, errLegacy("你已经没有足够的空间来放置新的装备，请将多余的装备回收。")
}

// openCombineArmorBox 对齐 GoodsFunc.php:5292。
// 降级：cfg_box_combine_details 表缺失 → boxInfo 恒空 → errLegacy(invalid_data)。
func (s *Service) openCombineArmorBox(ctx context.Context, uid, gid int) ([]map[string]any, error) {
	ok, err := s.checkGoods(ctx, uid, gid)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errLegacy("你拥有的该道具数量为0，请去商城购买后再使用。")
	}
	return nil, errLegacy("数据错误，请与客服联系。")
}

// openlongyuantieArmorBox 对齐 GoodsFunc.php:3700（dispatch 以 throw msg 形式使用 → 返回 errLegacy(msg)）。
// 降级：cfg_armor/sys_user_armor 表缺失 → 16 件 addArmor 跳过；正常扣盒。
func (s *Service) openlongyuantieArmorBox(ctx context.Context, uid, gid int) (string, error) {
	msg := "恭喜你获得:绝世的龙渊1套16件装备!"
	if gid == 8891 {
		msg = "恭喜你获得:绝世的白虎1套16件装备!"
	}
	if err := s.ReduceGoods(ctx, uid, gid, 1, 0); err != nil {
		return "", err
	}
	return "", errLegacy(msg)
}

// openlongyuanArmorBox 对齐 GoodsFunc.php:3714（dispatch 以 throw msg 形式使用 → 返回 errLegacy(msg)）。
// 原版怪癖保留：use_info 分支先置 $gidname="龙渊宝马！/白虎神兽！"，随后被 cfg_armor 查询结果覆盖。
// 降级：cfg_armor 表缺失 → 覆盖后 gidname=""；addArmor 跳过；广播省略；正常扣盒。
func (s *Service) openlongyuanArmorBox(ctx context.Context, uid, gid int) (string, error) {
	getrand := rand.Intn(101) // mt_rand(0,100) 含端点
	baohename := ""
	switch gid {
	case 8887:
		baohename = "龙渊"
	case 8889:
		baohename = "绝世白虎"
	case 8888:
		baohename = "绝世龙渊"
	case 8890:
		baohename = "白虎"
	}
	gidname := ""
	if getrand > 85 && getrand < 91 {
		gidname = "龙渊宝马！"
		if gid == 8889 || gid == 8890 {
			gidname = "白虎神兽！"
		}
		// $gidname=sql_fetch_one_cell("select name from cfg_armor ...") 覆盖（怪癖）；cfg_armor 缺失→""
		gidname = ""
	}
	usename, err := s.db.FetchCellString(ctx, "select nickname from users where id=?", uid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	msg := ""
	if gid == 8889 || gid == 8888 {
		msg = "恭喜玩家:" + usename + ",打开" + baohename + "装备匣,获得了:绝世的" + gidname
	} else {
		msg = "恭喜玩家:" + usename + ",打开" + baohename + "装备匣,获得了:" + gidname
	}
	if err := s.ReduceGoods(ctx, uid, gid, 1, 0); err != nil {
		return "", err
	}
	return "", errLegacy(msg)
}

// openunionandbattleBox 对齐 GoodsFunc.php:3602（dispatch 以 throw msg 形式使用 → 返回 errLegacy(msg)）。
// 原版怪癖保留：rate%6/%7/%9/%11 判定顺序、goodtype 1 走 things/荣誉不入 cfg_goods 掉落、
// case2/3/4 的 $goodname 未定义→okmsg 拼接空串。
// 降级：cfg_things/cfg_goods 名称查询无行→""；addThings/sys_inform 广播缺失→跳过；荣誉入 users.honour（列存在）。
func (s *Service) openunionandbattleBox(ctx context.Context, uid, gid int) (string, error) {
	okmsg := "恭喜你获得:"
	gcnt := boxRandInt(1, 5)
	for i := int64(0); i < gcnt; i++ {
		rate := boxRandInt(1, 105)
		goodtype := int64(5) // 普通
		if rate%6 == 0 {
			goodtype = 1 // 战场类
		} else if rate%7 == 0 {
			goodtype = 2 // 镶嵌类
		} else if rate%9 == 0 {
			goodtype = 3 // 装备类
		} else if rate%11 == 0 {
			goodtype = 4 // 马装类
		}
		cnt := boxRandInt(2, 5)
		goodname := ""
		goodid := int64(0)
		switch goodtype {
		case 1: // 战场类
			goodname = "荣誉:"
			goodid = boxRandInt(0, 15)
			if goodid > 10 {
				cnt = cnt * boxRandInt(993, 1841)
				if _, err := s.db.Exec(ctx, "update users set honour=honour+? where id=?", cnt, uid); err != nil {
					return "", err
				}
				// sendSysInform 广播省略
			} else {
				if goodid < 8 {
					goodid = goodid + 30000
				} else if goodid == 8 || goodid == 9 {
					goodid = 31008
				} else if goodid == 10 {
					goodid = 30010
				}
				cnt = cnt * 3
				// cfg_things 表缺失 → goodname=""；addThings 跳过；广播省略
				goodname = ""
			}
		case 2: // 镶嵌类
			isok := boxRandInt(0, 1)
			if isok == 1 {
				goodid = boxRandInt(0, 79) + 300
			} else {
				goodid = boxRandInt(401, 419)*100 + boxRandInt(1, 6)
			}
			getrands := boxRandInt(0, 10)
			if getrands%3 == 0 {
				if isok == 1 {
					goodid = boxRandInt(17500, 17539)
				} else {
					goodid = boxRandInt(401, 419)*100 + 10
				}
			}
		case 3: // 装备类
			goodid = boxRandInt(10714, 10729)
			getrands := boxRandInt(0, 10)
			if getrands == 1 || getrands == 7 {
				goodid = 8889
				cnt = 1
			}
		case 4: // 马装类
			goodid = boxRandInt(10219, 10223)
			getrands := boxRandInt(0, 10)
			if getrands == 2 || getrands == 6 {
				goodid = 10617
				cnt = 1
			}
		default:
			goodid = boxRandInt(1, 125)
			if goodid < 101 && goodid > 97 {
				goodid = 97
			}
		}
		if goodtype > 1 {
			name, err := s.boxCfgGoodsName(ctx, int(goodid))
			if err != nil {
				return "", err
			}
			goodname = name
			// sendSysInform 广播省略
			if err := s.AddGoods(ctx, uid, int(goodid), cnt, 3); err != nil {
				return "", err
			}
		}
		okmsg += goodname + strconv.FormatInt(cnt, 10) + ";"
	}
	if err := s.ReduceGoods(ctx, uid, gid, 1, 0); err != nil {
		return "", err
	}
	return "", errLegacy(okmsg)
}

// addSkillBook 对齐 GoodsFunc.php:3142（40000<gid<50000 技能书匣）。
// bid=(gid-40000)/100、level=gid%100。
// 降级：sys_user_book 表缺失 → 按任务约定走 legacy 空 bookCount 路径（user_goods 无该 gid→0→抛错）；
// 若持有该 gid：count=0、curShelfNum 表缺失→100、cfg_book 表缺失→bookName=""、插 book 循环跳过、正常扣书。
func (s *Service) addSkillBook(ctx context.Context, uid, gid, useCount int) (string, error) {
	bid := (gid - 40000) / 100
	level := gid % 100
	_ = bid
	bookCount, err := s.goodsCount(ctx, uid, gid)
	if err != nil {
		return "", err
	}
	if bookCount == 0 {
		// sys_user_book 表缺失 + 未持有技能书 → legacy empty($bookCount) 分支
		return "", errLegacy("没有技能保护符了:(")
	}
	count := int64(0) // sys_user_book 表缺失 → 0（降级）
	curShelfNum := int64(100)
	if count >= curShelfNum || int64(useCount)+count > curShelfNum {
		return "", errLegacy("技能书已满，不能继续打开技能书匣子")
	}
	if err := s.ReduceGoods(ctx, uid, gid, int64(useCount), 0); err != nil {
		return "", err
	}
	// for 循环 insert sys_user_book/log_book：表缺失 → 跳过（降级）
	bookName := "" // cfg_book 表缺失 → sql_fetch_one_cell 空
	return fmt.Sprintf("恭喜你获得%d级%s*1,请到技能书栏下查看", level, bookName), nil
}
