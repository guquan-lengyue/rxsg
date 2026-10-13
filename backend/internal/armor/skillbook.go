package armor

import (
	"context"
)

// skillbook.go 1:1 复刻 legacy HeroSkillFunc.php「铁匠铺-技能书」页签所需只读链路
// （原版 tab i18n key `BlackSmith_jinengshu`，客户端命令 `blacksmith/loadSkillBook_OpenTab`）：
//   loadUserValidSkillBook(5) / AddSkillBookShelf(21) / loadUserSkillBook(63) / loadSkillBook_OpenTab(360)。
//
// 表映射：sys_user_book → user_books（uid→user_id）、sys_user_bookShelfNum → user_book_shelf_num、
//   cfg_book → cfg_book（同名）、sys_goods → user_goods、cfg_goods → cfg_goods。
//
// 【数据缺口】原库 cfg_book（技能书配置）dump 已丢失、仓库无权威数据源 →
//   cfg_book 为合成种子（见 migrations/0023 头注「非原版精确还原」）；user_books 为新建空表。
// 【未移植】武将技能体系（sys_hero_skill / cfg_skill_rate）及技能书穿戴/合成/领悟
//   （setHeroBook:147 / moveHeroBook:220 / mergeBook:279 / resetHeroSkill:98 / getHeroSkill:71）
//   依赖未移植的技能系统，本轮不接线；本文件仅实现页签只读 + 扩展书架。
//
// 怪癖 1:1 保留：
//   - loadSkillBook_OpenTab:364 count 子查询按 `bid = a.skill`（而非 a.id）统计，且带 level=a.level
//     与 hid=0 条件（原版如此，逐字保留）。
//   - loadUserValidSkillBook:8 join 按 `a.bid=b.skill`（其余函数多按 `a.bid=b.id`；原版不一致，逐字保留）。
//   - AddSkillBookShelf:35 首次插入 num=110、之后 on duplicate key update num=num+10（非 +10→110 语义）。
//   - loadUserValidSkillBook 书架数 empty→100（0 或无行均回退 100）。

const (
	// add_bookShelfNum.no_enough_YuanBao（lang.php:2431）
	msgAddShelfNoMoney = "您的元宝不足，无法扩展技能书栏位"
	// add_bookShelfNum.armor_column_full（lang.php:2432）
	msgAddShelfFull = "你的技能书栏位已经最大，不能再扩展"
)

// LoadSkillBookOpenTab 对齐 HeroSkillFunc.php:360 loadSkillBook_OpenTab。
// 返回 [cfg_book 全表（附当前页签已拥有数量 count）, gid=10667 的 cfg_goods 行（附 count）]。
func (s *Service) LoadSkillBookOpenTab(ctx context.Context, uid int) ([]any, error) {
	books, err := s.db.FetchRows(ctx, `select *,
		(select count(1) from user_books where user_id=? and bid = a.skill and level=a.level and hid=0) as `+"`count`"+`
		from cfg_book a`, uid)
	if err != nil {
		return nil, err
	}
	goods, err := s.fetchOneOrNil(ctx, `select c.*, g.`+"`count`"+` from cfg_goods c
		left join user_goods g on g.gid=c.gid and g.user_id=? where c.gid=10667 limit 1`, uid)
	if err != nil {
		return nil, err
	}
	return []any{books, goods}, nil
}

// LoadUserSkillBook 对齐 HeroSkillFunc.php:63 loadUserSkillBook。
// 入参 skill（legacy $mBid，即技能 id，用于 b.skill 过滤）；返回 [已拥有该技能的技能书行]。
func (s *Service) LoadUserSkillBook(ctx context.Context, uid, skill int) ([]any, error) {
	rows, err := s.db.FetchRows(ctx, `select a.*, b.name, b.skill, b.attr_type, b.attr, b.description
		from user_books a left join cfg_book b on a.bid=b.id and a.level=b.level
		where b.skill=? and a.user_id=? and a.hid=0`, skill, uid)
	if err != nil {
		return nil, err
	}
	return []any{rows}, nil
}

// LoadUserValidSkillBook 对齐 HeroSkillFunc.php:5 loadUserValidSkillBook。
// 返回 [书架数量（默认 100）, 未穿戴的技能书行（附 xid=user_books.id）]。
func (s *Service) LoadUserValidSkillBook(ctx context.Context, uid int) ([]any, error) {
	num, err := cellInt(ctx, s.db, "select num from user_book_shelf_num where user_id=? limit 1", uid)
	if err != nil {
		return nil, err
	}
	shelfNum := 100 // legacy empty($bookShelfNum) → 100（0 或无行均回退）
	if num != 0 {
		shelfNum = int(num)
	}
	rows, err := s.db.FetchRows(ctx, `select a.*, a.id as xid, b.name, b.skill, b.attr_type, b.attr, b.description
		from user_books a left join cfg_book b on a.bid=b.skill and a.level=b.level
		where a.user_id=? and a.hid=0`, uid)
	if err != nil {
		return nil, err
	}
	return []any{shelfNum, rows}, nil
}

// AddSkillBookShelf 对齐 HeroSkillFunc.php:21 AddSkillBookShelf（元宝 -50 扩展技能书栏 +10）。
// 返回 [扩展后的书架数量]。
func (s *Service) AddSkillBookShelf(ctx context.Context, uid int) ([]any, error) {
	money, err := cellInt(ctx, s.db, "select money from users where id=? limit 1", uid)
	if err != nil {
		return nil, err
	}
	if money < 50 {
		return nil, errf(msgAddShelfNoMoney)
	}
	cur, err := cellInt(ctx, s.db, "select num from user_book_shelf_num where user_id=? limit 1", uid)
	if err != nil {
		return nil, err
	}
	if cur >= 500 {
		return nil, errf(msgAddShelfFull)
	}
	if err := s.addMoney(ctx, uid, -50, 31); err != nil { // 31=增加技能书栏扣除的元宝
		return nil, err
	}
	if _, err := s.db.Exec(ctx, `insert into user_book_shelf_num (user_id, num) values (?,110)
		on duplicate key update num=num+10`, uid); err != nil {
		return nil, err
	}
	newNum, err := cellInt(ctx, s.db, "select num from user_book_shelf_num where user_id=? limit 1", uid)
	if err != nil {
		return nil, err
	}
	return []any{newNum}, nil
}
