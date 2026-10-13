package goods

import (
	"context"
	"database/sql"
	"errors"
	"math/rand"
)

// compose.go 1:1 复刻 legacy server/game/GoodsFunc.php 的「背包批量操作」两条 RPC：
//   multiUseCard(4647) —— 名将卡批量使用（客户端 MyGoodsDialog 页签 6「名将卡」→ 卡批量使用）
//   goodsCompose(4836) —— 宝物合成（聚魂珠 10830-10839 / 马鬃 19081 / 马鞭 19082）
//
// 表映射：sys_goods → user_goods、cfg_goods → cfg_goods、mem_state → mem_state、sys_inform → （无表）。
// 客户端入口（tools/export/BloodWar/scripts/Goods/）：
//   UserGoodsItem.as:489 `multi` → MyGoodsDialog.multiUse4Card(:1653) → goods.multiUseCard
//   UserGoodsItem.as:493 `compose` → MyGoodsDialog.goodCompose(:858) → onComposeSubmit(:1767) → goods.goodsCompose

const (
	msgNotAvailable     = "该功能未开放"        // goods.not_available (lang.php:2216)
	msgCanNotCompose    = "该道具不能进行合成操作"   // goods.can_not_compose (lang.php:2454)
	msgComposeMaxLevel  = "该聚魂珠的品质已经达到最高" // goods.good_compose_maxLevel (lang.php:2456)
	msgComposeNotEnough = "你进行合成的聚魂珠数量不够" // goods.good_not_enough (lang.php:2455)
	msgComposeManeLow   = "你进行合成的马鬃数量不够"  // goods.good_not_enough_1 (lang.php:2469)
	msgComposeWhipLow   = "你进行合成的马鞭数量不够"  // goods.good_not_enough_2 (lang.php:2470)
)

// multiUseCardGids 对齐 GoodsFunc.php:4840 composeGidArr（可合成 gid 白名单）。
var composeGidArr = map[int]bool{
	10830: true, 10831: true, 10832: true, 10833: true, 10834: true,
	10835: true, 10836: true, 10837: true, 10838: true, 10839: true,
	19081: true, 19082: true,
}

// MultiUseCard 对齐 GoodsFunc.php:4647 multiUseCard（名将卡批量使用，一次最多 100 张）。
//
// 【声明性裁剪 · 与 giveMeHeroCard(4433)/generateHero4Card(4482)/useArmyOrder(4623) 同一降级口径】
//   原版首行门禁（:4649-4652）：`$yysType = select value from mem_state where state=197`，
//   非 60/55555555 即 throw goods.not_available("该功能未开放")。新库 mem_state 无 state=197 行 →
//   yysType 恒 0 → 恒抛该文案（与 legacy 默认服一致）。
//   客户端同样以 yysType==60||55555555 决定「名将卡」页签是否可见（MyGoodsDialog.as:1271），
//   故该页签在新库亦不显示。
//   门禁放行后的批量开卡循环（:4653-4686 / useOneHeroCard:4692）依赖 cfg_hero_card_schedule、
//   mem_state state=250、cfg_npc_hero、sys_inform 四张表，新库均无 → 无法 1:1 移植，保留入口与原文案。
func (s *Service) MultiUseCard(ctx context.Context, uid int) ([]any, error) {
	_ = uid
	yysType, err := s.memState(ctx, 197)
	if err != nil {
		return nil, err
	}
	if yysType != 60 && yysType != 55555555 {
		return nil, errLegacy(msgNotAvailable)
	}
	// 不可达（新库 mem_state 无 state=197）；见上方声明性裁剪说明。
	return nil, errLegacy(msgNotAvailable)
}

// memState 读取 mem_state.value（无行返回 0，对齐 legacy sql_fetch_one_cell 的 empty→0）。
func (s *Service) memState(ctx context.Context, state int) (int64, error) {
	v, err := s.db.FetchCellInt64(ctx, "select value from mem_state where state=?", state)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return v, err
}

// ComposeResult 对齐 legacy goodsCompose ret：$isSucc(1/0) + oneGood（cfg_goods 目标行，附 count=1）。
// 新库将此数组重塑为具名 JSON（与 UseGoods→UseResult 的既有约定一致）。
type ComposeResult struct {
	Success int              `json:"success"` // 1=成功 0=失败（仅马鞭合成有失败可能）
	Goods   []map[string]any `json:"goods"`   // 目标 gid 的 cfg_goods 行（附 count=1）
}

// GoodsCompose 对齐 GoodsFunc.php:4836 goodsCompose。入参 gid 为待合成道具。
// 分支：19081 马鬃（耗 5 成 1）、19082 马鞭（耗 2，25% 成 1）、其余聚魂珠 10830-10838（耗 2 成 1）。
// 怪癖/文案 1:1 保留：10839 先于白名单判定之后抛「已达最高」；聚魂珠/马鬃/马鞭三档不同缺料文案。
func (s *Service) GoodsCompose(ctx context.Context, uid, gid int) (*ComposeResult, error) {
	if !composeGidArr[gid] {
		return nil, errLegacy(msgCanNotCompose)
	}
	if gid == 10839 {
		return nil, errLegacy(msgComposeMaxLevel)
	}

	var out *ComposeResult
	err := s.lk.WithUserLock(ctx, uid, "usegoods", func(ctx context.Context) error {
		cur, err := s.goodsCount(ctx, uid, gid)
		if err != nil {
			return err
		}
		newGid := gid + 1
		isSucc := 1
		switch gid {
		case 19081: // 马鬃：需 5，耗 5 成 1（GoodsFunc.php:4856）
			if cur < 5 {
				return errLegacy(msgComposeManeLow)
			}
			if err := s.AddGoods(ctx, uid, gid, -5, 917); err != nil {
				return err
			}
			if err := s.AddGoods(ctx, uid, newGid, 1, 917); err != nil {
				return err
			}
		case 19082: // 马鞭：需 2，耗 2；25% 成 1，否则仅耗料（GoodsFunc.php:4864）
			if cur < 2 {
				return errLegacy(msgComposeWhipLow)
			}
			if rand.Intn(100)+1 <= 25 {
				if err := s.AddGoods(ctx, uid, gid, -2, 917); err != nil {
					return err
				}
				if err := s.AddGoods(ctx, uid, newGid, 1, 917); err != nil {
					return err
				}
			} else {
				if err := s.AddGoods(ctx, uid, gid, -2, 917); err != nil {
					return err
				}
				isSucc = 0
			}
		default: // 聚魂珠 10830-10838：需 2，耗 2 成 1（GoodsFunc.php:4880）
			if cur < 2 {
				return errLegacy(msgComposeNotEnough)
			}
			if err := s.AddGoods(ctx, uid, gid, -2, 917); err != nil {
				return err
			}
			if err := s.AddGoods(ctx, uid, newGid, 1, 917); err != nil {
				return err
			}
		}

		// legacy: select *,$cnt as count from cfg_goods where gid=$newGid（$cnt 恒 1）
		rows, err := s.db.FetchRows(ctx, "select *, 1 as `count` from cfg_goods where gid=?", newGid)
		if err != nil {
			return err
		}
		out = &ComposeResult{Success: isSucc, Goods: rows}
		return nil
	})
	if err != nil {
		if err.Error() == "lock_busy" {
			return nil, legacyBusy()
		}
		return nil, err
	}
	return out, nil
}
