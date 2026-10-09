package goods

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"rxsg/backend/internal/httpx"
)

// store.go 复刻 legacy server/game/utils.php 的道具背包底层：
//   addGoods(971) / reduceGoods(938) / checkGoods(933) / checkGoodsCount(1159)
//   以及 mem_user_buffer / mem_city_resource 的读写小工具。
// 与 legacy 差异（新库缺表，按既有模式处理）：
//   - 用户级并发锁：legacy 在 checkGoodsCount/reduceGoods 内部 lockUser/unlockUser 文件锁；
//     Go 版在 UseGoods 入口统一 WithUserLock 包裹，此处不再单独加解锁。
//   - reduceGoods 内 finishAchivement（武魂/誓师成就）属 M8，未接线。

// goodsCount 读取 user_goods.count；无行返回 0（对齐 legacy empty→0）。
func (s *Service) goodsCount(ctx context.Context, uid, gid int) (int64, error) {
	v, err := s.db.FetchCellInt64(ctx, "select `count` from user_goods where user_id=? and gid=?", uid, gid)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return v, err
}

// AddGoods 对齐 utils.php:971 addGoods：cnt==0 直接返回；否则 upsert count=count+cnt（有符号，不夹 0），写 log_goods 流水。
// legacy 特殊分支（M6 补齐，1:1 保留原版怪癖）：
//   - gid==0：不入背包，直接 users.gift+=cnt 并写 log_gifts（含 gid 列）。buyWorkShopGood 以 addGoods(0,-price) 扣礼金。
//   - gid==152（铜钱）：入账后把 $cnt 改写为入账后的铜钱总数、$gid 改写为 888888，再记一份 888888 入账+流水
//     ——原版双记账 bug，1:1 保留。
//   - gid==160052（誓约）依赖 log_qiyue 表（新库无）→ 按普通道具处理并注释。
func (s *Service) AddGoods(ctx context.Context, uid, gid int, cnt int64, typ int) error {
	if cnt == 0 {
		return nil
	}
	if gid == 0 {
		if _, err := s.db.Exec(ctx, "update users set gift=gift+? where id=?", cnt, uid); err != nil {
			return err
		}
		_, err := s.db.Exec(ctx,
			"insert into log_gifts (user_id, gid, `count`, `time`, `type`) values (?,0,?,unix_timestamp(),?)",
			uid, cnt, typ)
		return err
	}
	if _, err := s.db.Exec(ctx,
		"insert into user_goods (user_id, gid, `count`) values (?,?,?) on duplicate key update `count`=`count`+?",
		uid, gid, cnt, cnt); err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx,
		"insert into log_goods (user_id, gid, `count`, `time`, `type`) values (?,?,?,unix_timestamp(),?)",
		uid, gid, cnt, typ); err != nil {
		return err
	}
	if gid == 152 {
		// 原版怪癖：铜钱入账后按"新总数"再记一份积分(888888)入账与流水。
		total, err := s.goodsCount(ctx, uid, 152)
		if err != nil {
			return err
		}
		if _, err := s.db.Exec(ctx,
			"insert into user_goods (user_id, gid, `count`) values (?,?,?) on duplicate key update `count`=`count`+?",
			uid, 888888, total, total); err != nil {
			return err
		}
		_, err = s.db.Exec(ctx,
			"insert into log_goods (user_id, gid, `count`, `time`, `type`) values (?,?,?,unix_timestamp(),?)",
			uid, 888888, total, typ)
		return err
	}
	return nil
}

// ReduceGoods 对齐 utils.php:938 reduceGoods：count>0 才处理，log_goods 记 -count，count=GREATEST(0,count-n)。
func (s *Service) ReduceGoods(ctx context.Context, uid, gid int, cnt int64, typ int) error {
	if cnt <= 0 {
		return nil
	}
	if _, err := s.db.Exec(ctx,
		"insert into log_goods (user_id, gid, `count`, `time`, `type`) values (?,?,?,unix_timestamp(),?)",
		uid, gid, -cnt, typ); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx,
		"update user_goods set `count`=GREATEST(0,`count`-?) where user_id=? and gid=?", cnt, uid, gid)
	return err
}

// checkGoods 对齐 utils.php:933：拥有数量 >=1。
func (s *Service) checkGoods(ctx context.Context, uid, gid int) (bool, error) {
	return s.checkGoodsCount(ctx, uid, gid, 1)
}

// checkGoodsCount 对齐 utils.php:1159（不含 lockUser，锁由上层持有）。
func (s *Service) checkGoodsCount(ctx context.Context, uid, gid int, need int64) (bool, error) {
	cnt, err := s.goodsCount(ctx, uid, gid)
	if err != nil {
		return false, err
	}
	return cnt >= need, nil
}

// addUserBuffer 对齐 mem_user_buffer 的 insert ... on duplicate key update endtime=endtime+delay；
// 返回写入后的 endtime。insertDelay 用于新行（now+delay），updateDelay 用于已有行（endtime+delay）。
// 多数道具两者相同；徭役令/诏安令的 legacy 原样为 insert 用 ×useCount、update 固定单份（原版 bug，1:1 保留）。
func (s *Service) addUserBuffer(ctx context.Context, uid, buftype int, insertDelay, updateDelay int64) (int64, error) {
	if _, err := s.db.Exec(ctx,
		"insert into user_buffers (user_id, buftype, endtime) values (?,?,unix_timestamp()+?) on duplicate key update endtime=endtime+?",
		uid, buftype, insertDelay, updateDelay); err != nil {
		return 0, err
	}
	return s.bufferEnd(ctx, uid, buftype)
}

// bufferEnd 读取某 buftype 的 endtime（无行返回 0）。
func (s *Service) bufferEnd(ctx context.Context, uid, buftype int) (int64, error) {
	v, err := s.db.FetchCellInt64(ctx, "select endtime from user_buffers where user_id=? and buftype=?", uid, buftype)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return v, err
}

// bufferActive 对齐 legacy `select count(1) from mem_user_buffer where uid and buftype`==1 的互斥检查。
func (s *Service) bufferActive(ctx context.Context, uid, buftype int) (bool, error) {
	return s.db.Exists(ctx, "select 1 from user_buffers where user_id=? and buftype=? limit 1", uid, buftype)
}

// addCityResources 对齐 utils.php:193（参数顺序 wood,rock,iron,food,gold）。
func (s *Service) addCityResources(ctx context.Context, cid int, wood, rock, iron, food, gold int64) error {
	if cid <= 0 {
		return nil
	}
	_, err := s.db.Exec(ctx, `update city_resources set
		wood=wood+?, rock=rock+?, iron=iron+?, food=food+?, gold=gold+? where city_id=?`,
		wood, rock, iron, food, gold, cid)
	return err
}

// setGoodsAdd 对齐 useShenNongChu 等对 sys_city_res_add 的全服城更新：
//
//	update a,sys_city c set a.goods_xxx_add=25, a.resource_changing=1 where c.uid=uid and a.cid=c.cid
//
// Go 版用 insert...select...on duplicate 保证每个城都有行。
func (s *Service) setGoodsAdd(ctx context.Context, uid int, col string) error {
	_, err := s.db.Exec(ctx,
		"insert into city_res_add (city_id, `"+col+"`, resource_changing) "+
			"select id, 25, 1 from cities where user_id=? "+
			"on duplicate key update `"+col+"`=25, resource_changing=1", uid)
	return err
}

// lastCityID 对齐 useGoods:40 `select lastcid from sys_user`。
func (s *Service) lastCityID(ctx context.Context, uid int) (int, error) {
	v, err := s.db.FetchCellInt64(ctx, "select lastcid from users where id=?", uid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	return int(v), nil
}

// cellFloat 读取单列数值并按浮点解析（对齐 legacy sql_fetch_one_cell 字符串数值化语义）。
func (s *Service) cellFloat(ctx context.Context, query string, args ...any) (float64, error) {
	v, err := s.db.FetchCellString(ctx, query, args...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, nil
	}
	return f, nil
}

// errLegacy 构造与 legacy throw new Exception 等价的错误响应。
func errLegacy(msg string) error {
	return httpx.BadRequest("use_goods_error", msg)
}

var _ = fmt.Sprintf
