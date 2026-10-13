package armor

import (
	"context"
	"fmt"

	"rxsg/backend/internal/model"
)

// xilian.go 1:1 复刻 legacy EquipmentFunc.php「极品属性洗练」链路：
//   loadXilianGoods(1821) / doXilian(1841)，复用 strong.go 的 getBestQuality(472)/getProperty(492)。
//
// 表映射：sys_user_armor → user_armors（uid→user_id）、cfg_armor → cfg_armor、
//   sys_goods → user_goods、cfg_goods → cfg_goods。
//
// 洗练属性/概率来源（EquipmentFunc.php:472-489）：
//   · 出极品属性判定 mt_rand(1,100)<=rate（出厂概率由调用方给定，祝融之符/洗练符均为 100）；
//   · 命中后 rand_id = mt_rand(1, count(cfg_xilian))，属性按 cfg_xilian.property 加权随机（getProperty）；
//   · 数值 value = mt_rand(cfg_xilian_type.minAdd, maxAdd)（色阶由 cfg_armor.type 决定）。
//   上述均已在 getBestQuality 实现，本文件仅调用。
//
// 怪癖 1:1 保留：
//   - 三个分支各自独立的校验顺序（PHP:1860-1940，best_quality 校验先于道具数量校验，分支 2 的
//     type→count 分支（含"未开放"）先于道具数量校验）；
//   - index=0 分支缺道具文案取自 $GLOBALS['xilian']['lack_xilian']，而 server/game/lang.php
//     【未定义该键】（全文仅 2653-2656 定义 xilian 的 4 个键）→ 原版抛空消息（PHP 未定义索引→null→""）。
//     本实现对空消息按下述"文案缺口"处理，未臆造服务端文案。
//   - index=1/2 缺道具文案为动态拼接 "not_enough_goods{gid}[#{left}]"（非 lang.php 键，原样保留）。
//   - index=1 清洗写 best_quality=null（本项目列为 NOT NULL → 写空串，empty 语义一致）。

const (
	// xilian.param_error（lang.php:2653）
	msgXilianParamErr = "参数异常"
	// xilian.xilian_exist（lang.php:2654）
	msgXilianExist = "装备已存在极品属性"
	// xilian.xilian_not_exist（lang.php:2655）
	msgXilianNotExist = "装备不存在极品属性，不能进行清洗"
	// xilian.xilian_not_open（lang.php:2656）
	msgXilianNotOpen = "未开放"

	// 文案缺口：legacy index=0 缺道具用 $GLOBALS['xilian']['lack_xilian']，lang.php 未定义该键
	//   （原版 = 空消息）。此处取客户端文案（tools/export/BloodWar/binaryData/4_utils.LocaleChinese.bin
	//   逐字："您缺少祝融之符"）作为可读文案；非服务端 lang.php 原文，属"数据缺口"处理。
	msgXilianLack = "您缺少祝融之符"
)

// xilianGids（EquipmentFunc.php:1827）index→道具 gid：0 祝融之符 / 1 极品属性清洗符 / 2 极品属性洗练符。
var xilianGids = []int{12079, 12080, 12081}

// LoadXilianGoods 对齐 EquipmentFunc.php:1821 loadXilianGoods。
// 入参 xilianIndex∈[0,2]（越界抛"参数异常"）；返回 [cfg_goods 行（附该用户 user_goods.count）]。
func (s *Service) LoadXilianGoods(ctx context.Context, uid, xilianIndex int) ([]any, error) {
	if xilianIndex > 2 || xilianIndex < 0 {
		return nil, errf(msgXilianParamErr)
	}
	gid := xilianGids[xilianIndex]
	row, err := s.fetchOneOrNil(ctx, "select * from cfg_goods where gid=? limit 1", gid)
	if err != nil {
		return nil, err
	}
	if row != nil {
		cnt, err := s.goodsCount(ctx, uid, gid) // 无行→0（对齐 legacy empty→0）
		if err != nil {
			return nil, err
		}
		row["count"] = cnt
	}
	return []any{row}, nil
}

// DoXilian 对齐 EquipmentFunc.php:1841 doXilian。
// 参数序（legacy 逐位 array_shift）：xilianIndex, gid, sid。
// 返回沿用 legacy ret 数组：index0 → [best_quality, 1]；index1 → ["", 1]；index2 → [best_quality, count]。
func (s *Service) DoXilian(ctx context.Context, uid, xilianIndex, gid, sid int) ([]any, error) {
	if xilianIndex > 2 || xilianIndex < 0 {
		return nil, errf(msgXilianParamErr)
	}
	if (xilianIndex == 0 && gid != 12079) || (xilianIndex == 1 && gid != 12080) || (xilianIndex == 2 && gid != 12081) {
		return nil, errf(msgXilianParamErr)
	}
	armor, err := s.fetchOneOrNil(ctx, `select u.*, c.type from user_armors u
		left join cfg_armor c on c.id=u.armorid where u.sid=? and u.user_id=? limit 1`, sid, uid)
	if err != nil {
		return nil, err
	}
	if armor == nil {
		return nil, errf(msgNoSuchArmor)
	}
	bestQuality := model.Str(armor, "best_quality") // 空串 ≡ legacy empty(NULL)

	switch xilianIndex {
	case 0: // 祝融之符：无极品属性时赋予
		if bestQuality != "" {
			return nil, errf(msgXilianExist)
		}
		cnt, err := s.goodsCount(ctx, uid, gid)
		if err != nil {
			return nil, err
		}
		if cnt < 1 {
			return nil, errf(msgXilianLack)
		}
		// getBestQuality(100, type)：第一个参数是出极品属性的概率（100=必出）
		bq, err := s.getBestQuality(ctx, 100, model.Int(armor, "type"))
		if err != nil {
			return nil, err
		}
		if bq != "" {
			if _, err := s.db.Exec(ctx, "update user_armors set best_quality=? where user_id=? and sid=?", bq, uid, sid); err != nil {
				return nil, err
			}
		}
		if err := s.addGoods(ctx, uid, gid, -1, 0); err != nil {
			return nil, err
		}
		return []any{bq, 1}, nil

	case 1: // 极品属性清洗符：清除已有极品属性
		if bestQuality == "" {
			return nil, errf(msgXilianNotExist)
		}
		cnt, err := s.goodsCount(ctx, uid, gid)
		if err != nil {
			return nil, err
		}
		if cnt < 1 {
			return nil, errf(fmt.Sprintf("not_enough_goods%d", gid))
		}
		// legacy: update ... set best_quality=null（本项目列 NOT NULL → 写空串，empty 语义一致）
		if _, err := s.db.Exec(ctx, "update user_armors set best_quality='' where user_id=? and sid=?", uid, sid); err != nil {
			return nil, err
		}
		if err := s.addGoods(ctx, uid, gid, -1, 0); err != nil {
			return nil, err
		}
		return []any{"", 1}, nil

	case 2: // 极品属性洗练符：重滚极品属性，按色阶消耗多枚
		if bestQuality == "" {
			return nil, errf(msgXilianNotExist)
		}
		typ := model.Int(armor, "type")
		var count int
		switch {
		case typ <= 3 && typ >= 1:
			count = 1
		case typ == 4:
			count = 2
		case typ == 5:
			count = 3
		case typ == 6:
			count = 4
		default:
			return nil, errf(msgXilianNotOpen)
		}
		cnt, err := s.goodsCount(ctx, uid, gid)
		if err != nil {
			return nil, err
		}
		if cnt < int64(count) {
			left := int64(count) - cnt
			return nil, errf(fmt.Sprintf("not_enough_goods%d#%d", gid, left))
		}
		bq, err := s.getBestQuality(ctx, 100, typ)
		if err != nil {
			return nil, err
		}
		if bq != "" {
			if _, err := s.db.Exec(ctx, "update user_armors set best_quality=? where user_id=? and sid=?", bq, uid, sid); err != nil {
				return nil, err
			}
		}
		if err := s.addGoods(ctx, uid, gid, -int64(count), 0); err != nil {
			return nil, err
		}
		return []any{bq, count}, nil
	}
	// 不可达（xilianIndex 已保证 ∈[0,2]）
	return nil, errf(msgXilianParamErr)
}
