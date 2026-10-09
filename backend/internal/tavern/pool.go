package tavern

import (
	"context"
	"database/sql"
	"errors"
	"math"

	"rxsg/backend/internal/model"
)

// pool.go 复刻 HotelFunc.php 招募池刷新与将领生成：
//   doGetRecruitHero(246) / regenerate 块逻辑 / generateRecruitHero(177) /
//   generateHeroName(39) / generateName(32) / isActHero(ActFunc.php:48)。
//
// 原版怪癖 1:1 保留：
//   - last_reset_recruit 无行时：insert 置 now，但变量按 empty 分支归 0 → 下次刷新块计算从 0 起；
//   - blockdelta 只删最旧的 delta 个（order by id limit $blockdelta），补齐至 level 个；
//   - 招贤榜（last_reset_recruit==0）→ rate=活动rate；否则未用榜保底 5%；
//     每日活动将领上限 countLimitPerDay=1 依赖 log_act（无表 → sum 恒 0，不拦截）；
//   - mt_srand(mt_rand()) 重播种不影响分布，Go 版省略；
//   - generateRecruitHero 内 all_base 用 rand()（非 mt_rand）——原版混用，Go 统一 math/rand。

// doGetRecruitHero 对齐 HotelFunc.php:246。
func (s *Service) doGetRecruitHero(ctx context.Context, uid, cid, level int) ([]Recruit, error) {
	recruits := []Recruit{}
	if level <= 0 {
		return recruits, nil
	}

	lastReset, err := s.cellInt64Zero(ctx, "select last_reset_recruit from city_schedule where city_id=?", cid)
	if err != nil {
		return nil, err
	}
	if lastReset == 0 {
		// insert ... on duplicate key update last_reset_recruit=unix_timestamp()；变量保持 0（原版语义）。
		if _, err := s.db.Exec(ctx,
			"insert into city_schedule (city_id, last_reset_recruit) values (?,unix_timestamp()) on duplicate key update last_reset_recruit=unix_timestamp()",
			cid); err != nil {
			return nil, err
		}
		lastReset = 0
	}
	now, err := s.db.Now(ctx)
	if err != nil {
		return nil, err
	}
	blocksize := float64(recruitBlock/gameSpeedRate) / float64(level)
	lastBlock := math.Floor(float64(lastReset+8*3600) / blocksize)
	currBlock := math.Floor(float64(now+8*3600) / blocksize)
	blockdelta := int(currBlock - lastBlock)

	if blockdelta > 0 {
		// 删除最旧的 blockdelta 个（order by id limit）。
		olds, err := s.db.FetchRows(ctx, "select id from recruit_heroes where city_id=? order by id limit ?", cid, blockdelta)
		if err != nil {
			return nil, err
		}
		for _, o := range olds {
			if _, err := s.db.Exec(ctx, "delete from recruit_heroes where id=?", model.Int(o, "id")); err != nil {
				return nil, err
			}
		}
		heroCount, err := s.cellInt64Zero(ctx, "select count(*) from recruit_heroes where city_id=?", cid)
		if err != nil {
			return nil, err
		}

		// 活动（cfg_act type=2001）无表 → tresult 恒 false → 走纯 generateRecruitHero 补齐分支。
		for i := int(heroCount); i < level; i++ {
			if err := s.generateRecruitHero(ctx, cid, level); err != nil {
				return nil, err
			}
		}
		if _, err := s.db.Exec(ctx, "update city_schedule set last_reset_recruit=unix_timestamp() where city_id=?", cid); err != nil {
			return nil, err
		}
	}

	rows, err := s.db.FetchRows(ctx, "select * from recruit_heroes where city_id=? order by id desc", cid)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		recruits = append(recruits, Recruit{
			ID:          model.Int(r, "id"),
			Name:        model.Str(r, "name"),
			Sex:         model.Int(r, "sex"),
			Face:        model.Int(r, "face"),
			Level:       model.Int(r, "level"),
			AffairsBase: model.Int(r, "affairs_base"),
			BraveryBase: model.Int(r, "bravery_base"),
			WisdomBase:  model.Int(r, "wisdom_base"),
			CommandBase: model.Int(r, "command_base"),
			AffairsAdd:  model.Int(r, "affairs_add"),
			BraveryAdd:  model.Int(r, "bravery_add"),
			WisdomAdd:   model.Int(r, "wisdom_add"),
			CommandAdd:  model.Int(r, "command_add"),
			Loyalty:     model.Int(r, "loyalty"),
			GoldNeed:    model.Int64(r, "gold_need"),
			HeroType:    model.Int(r, "hero_type"),
			IsActHero:   isActHero(model.Int(r, "hero_type")),
		})
	}
	return recruits, nil
}

// generateRecruitHero 对齐 HotelFunc.php:177。
func (s *Service) generateRecruitHero(ctx context.Context, cid, level int) error {
	// 性别：1/10 概率女。
	sex := 1
	if randInclusive(0, 9) == 0 {
		sex = 0
	}
	name, err := s.generateHeroName(ctx, sex)
	if err != nil {
		return err
	}
	face := randInclusive(1001, 1070)
	if sex == 0 {
		face = randInclusive(100, 145)
	}
	// 等级：<11 级客栈 → 1..level*5；≥11 → 50% 1..39 / 50% 40..50。
	hlevel := 0
	if level < 11 {
		hlevel = randInclusive(1, level*5)
	} else if randInclusive(0, 1) == 0 {
		hlevel = randInclusive(1, 39)
	} else {
		hlevel = randInclusive(40, 50)
	}

	affairsRate := randInclusive(300, 900)
	braveryRate := randInclusive(300, 900)
	wisdomRate := randInclusive(300, 900)
	allRate := affairsRate + braveryRate + wisdomRate

	total, ok, err := s.levelTotalExp(ctx, hlevel)
	if err != nil {
		return err
	}
	if !ok {
		return errTavern(msgNoLevelData)
	}

	allBase := randInclusive(50, 170)
	commandBase := 0
	if allBase > 110 && allBase <= 150 {
		commandBase = randInclusive(1, 5)
	} else if allBase > 150 && allBase <= 170 {
		commandBase = randInclusive(6, 10)
	}
	affairsBase := int(math.Floor(float64(allBase) * float64(affairsRate) / float64(allRate)))
	braveryBase := int(math.Floor(float64(allBase) * float64(braveryRate) / float64(allRate)))
	wisdomBase := int(math.Floor(float64(allBase) * float64(wisdomRate) / float64(allRate)))

	affairsAdd := int(math.Round(float64(hlevel) * float64(affairsRate) / float64(allRate)))
	braveryAdd := int(math.Round(float64(hlevel) * float64(braveryRate) / float64(allRate)))
	wisdomAdd := hlevel - affairsAdd - braveryAdd

	loyalty := 70
	goldNeed := recruitGoldNeed(hlevel, affairsBase, affairsAdd, braveryBase, braveryAdd, wisdomBase, wisdomAdd)

	// 怪癖：legacy 此 insert 不含 command_add/herotype 列 → 默认 0。
	_, err = s.db.Exec(ctx, `insert into recruit_heroes
		(name, sex, face, city_id, level, exp, affairs_base, bravery_base, wisdom_base,
		 command_base, affairs_add, bravery_add, wisdom_add, loyalty, gold_need, gen_time)
		values (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,unix_timestamp())`,
		name, sex, face, cid, hlevel, total,
		affairsBase, braveryBase, wisdomBase, commandBase,
		affairsAdd, braveryAdd, wisdomAdd, loyalty, goldNeed)
	return err
}

// recruitGoldNeed 对齐 HotelFunc.php:230（×50 黄金价式）。
func recruitGoldNeed(hlevel, ab, aa, bb, ba, wb, wa int) int64 {
	ex := 0
	if v := ab + aa - 90; v > 0 {
		ex += v
	}
	if v := bb + ba - 90; v > 0 {
		ex += v
	}
	if v := wb + wa - 90; v > 0 {
		ex += v
	}
	return int64((hlevel*20 + ex*50) * 50)
}

// generateHeroName 对齐 HotelFunc.php:39（姓+名，mem_cfg_* → cfg_names kind 1/2/3）。
func (s *Service) generateHeroName(ctx context.Context, sex int) (string, error) {
	first, err := s.generateName(ctx, 1)
	if err != nil {
		return "", err
	}
	kind := 2
	if sex == 0 {
		kind = 3
	}
	given, err := s.generateName(ctx, kind)
	if err != nil {
		return "", err
	}
	return first + given, nil
}

// generateName 对齐 HotelFunc.php:32。
func (s *Service) generateName(ctx context.Context, kind int) (string, error) {
	cnt, err := s.cellInt64Zero(ctx, "select count(*) from cfg_names where kind=?", kind)
	if err != nil {
		return "", err
	}
	if cnt == 0 {
		return "", nil
	}
	v, err := s.db.FetchCellString(ctx, "select `name` from cfg_names where kind=? and id=? limit 1", kind, randInclusive(1, int(cnt)))
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// isActHero 对齐 ActFunc.php:48。
func isActHero(heroType int) bool {
	return heroType > 10 && heroType != 100 && heroType < 20000
}
