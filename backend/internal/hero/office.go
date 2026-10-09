package hero

import (
	"context"
	"database/sql"
	"errors"

	"rxsg/backend/internal/model"
)

// office.go 1:1 复刻 legacy server/game/OfficeFunc.php：
//   setCityChief(30) / doSetCityChief(123) / doSetCityGeneral(188) /
//   doSetCityCounsellor(210) / heroIsInTroop(102) / checkHeroSelf(112)。
//
// 原版流程（setCityChief:30）：
//  1. checkCityOwner；
//  2. 校验遍：param 每项必须是 array（否则 set_chief_fail_2）、hid>0 时要求
//     该城存在 state∈{0,1,7,8} 的武将（否则 set_chief_fail）；
//  3. 卸任遍：读 oldtype=select state from heroes where hid=$hid（legacy 只按 hid，
//     不校验 cid——怪癖保留），oldtype==1/7/8 → doSetCity*(uid,cid,0)；
//     cheiftype>0 才入 sets；
//  4. 应用遍：按 newtype 1/7/8 调 doSetCity*(uid,cid,hid)；
//  5. 返回 getOfficeInfo —— 新前端以武将列表刷新，Go 版返回 Info。
//
// 原版怪癖保留：
//   - doSetCityChief 卸任旧城守 `update heroes set state=0 where id=oldChief` 无 cid 条件；
//   - checkHeroSelf 三列互斥（if/else if）——同将兼任多职时只清一列（原版如此）；
//   - getOfficeInfo 无 office 建筑（bid=11）时 throw no_office_built，lang 原文为 "铁锭"
//     （原版翻译错位，逐字保留）。
//
// 降级：cfg_book/sys_user_book 技能表无 → skill_*_add 清零后不再回填（恒 0）；
//   completeTask(85)/completeTaskWithTaskid(328/330) 属 M8，未接线。

// officeBuildingID 对齐 ID_BUILDING_OFFICE（bloodcommon.swc 枚举：官署=11）。
const officeBuildingID = 11

// msgNoOfficeBuilt 对齐 lang.php:1132 getOfficeInfo.no_office_built（原文 "铁锭"，原版错位保留）。
const msgNoOfficeBuilt = "铁锭"

// SetChief 对齐 setCityChief。sets 每项为 [hid, cheiftype]（cheiftype∈{0,1,7,8}）。
func (s *Service) SetChief(ctx context.Context, uid, cid int, sets [][2]int) (*Info, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return nil, err
	}
	// 1. 校验遍（setCityChief:36-47）。
	for _, p := range sets {
		hid, cheiftype := p[0], p[1]
		_ = cheiftype
		if hid > 0 {
			ok, err := s.db.Exists(ctx,
				"select 1 from heroes where city_id=? and id=? and state in (0,1,7,8) limit 1", cid, hid)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, errHero(msgSetChiefFail)
			}
		}
	}
	// 2. 卸任遍（setCityChief:67-86）：oldtype 只按 hid 查询（不校验 cid——原版怪癖）。
	type officeSet struct {
		hid, cheiftype, oldtype int
	}
	var toApply []officeSet
	for _, p := range sets {
		hid, cheiftype := p[0], p[1]
		oldtype, err := s.cellInt64Zero(ctx, "select state from heroes where id=? limit 1", hid)
		if err != nil {
			return nil, err
		}
		switch int(oldtype) {
		case StateChief:
			if err := s.doSetCityChief(ctx, uid, cid, 0); err != nil {
				return nil, err
			}
		case StateGeneral:
			if err := s.doSetCityGeneral(ctx, uid, cid, 0); err != nil {
				return nil, err
			}
		case StateCounsellor:
			if err := s.doSetCityCounsellor(ctx, uid, cid, 0); err != nil {
				return nil, err
			}
		}
		if cheiftype > 0 {
			toApply = append(toApply, officeSet{hid: hid, cheiftype: cheiftype, oldtype: int(oldtype)})
		}
	}
	// 3. 应用遍（setCityChief:87-97）。
	for _, st := range toApply {
		switch st.cheiftype {
		case StateChief:
			if err := s.doSetCityChief(ctx, uid, cid, st.hid); err != nil {
				return nil, err
			}
		case StateGeneral:
			if err := s.doSetCityGeneral(ctx, uid, cid, st.hid); err != nil {
				return nil, err
			}
		case StateCounsellor:
			if err := s.doSetCityCounsellor(ctx, uid, cid, st.hid); err != nil {
				return nil, err
			}
		}
	}
	// 4. legacy 返回 getOfficeInfo（官署建筑信息）；新前端以武将列表刷新 → 返回 Info。
	if err := s.officeExists(ctx, cid); err != nil {
		return nil, err
	}
	return s.Info(ctx, uid, cid)
}

// officeExists 对齐 getOfficeInfo:20——无官署建筑（bid=11）时 throw no_office_built。
func (s *Service) officeExists(ctx context.Context, cid int) error {
	ok, err := s.db.Exists(ctx,
		"select 1 from buildings where city_id=? and building_id=? limit 1", cid, officeBuildingID)
	if err != nil {
		return err
	}
	if !ok {
		return errHero(msgNoOfficeBuilt)
	}
	return nil
}

// heroIsInTroop 对齐 OfficeFunc.php:102——troops 表存在该武将且 user_id!=0。
func (s *Service) heroIsInTroop(ctx context.Context, hid int) (bool, error) {
	if hid == 0 {
		return false, nil
	}
	return s.db.Exists(ctx, "select 1 from troops where hero_id=? and user_id<>0 limit 1", hid)
}

// checkHeroSelf 对齐 OfficeFunc.php:112——该武将若占三职之一则清对应列（if/else if 互斥，原版如此）。
func (s *Service) checkHeroSelf(ctx context.Context, cid, hid int) error {
	if hid == 0 {
		return nil
	}
	city, err := s.db.FetchOne(ctx,
		"select chief_hero_id, general_hero_id, counsellor_hero_id from cities where id=?", cid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	switch {
	case model.Int(city, "chief_hero_id") == hid:
		_, err = s.db.Exec(ctx, "update cities set chief_hero_id=0 where id=?", cid)
	case model.Int(city, "general_hero_id") == hid:
		_, err = s.db.Exec(ctx, "update cities set general_hero_id=0 where id=?", cid)
	case model.Int(city, "counsellor_hero_id") == hid:
		_, err = s.db.Exec(ctx, "update cities set counsellor_hero_id=0 where id=?", cid)
	}
	return err
}

// doSetCityChief 对齐 OfficeFunc.php:123（设置城守）。
func (s *Service) doSetCityChief(ctx context.Context, uid, cid, hid int) error {
	if err := s.checkHeroSelf(ctx, cid, hid); err != nil {
		return err
	}
	oldChief, err := s.cellInt64Zero(ctx, "select chief_hero_id from cities where id=? limit 1", cid)
	if err != nil {
		return err
	}
	busy, err := s.heroIsInTroop(ctx, int(oldChief))
	if err != nil {
		return err
	}
	if busy {
		return errHero(msgSetChiefBusy)
	}
	if oldChief > 0 {
		// legacy 无 cid 条件（怪癖保留）。
		if _, err := s.db.Exec(ctx, "update heroes set state=0 where id=?", oldChief); err != nil {
			return err
		}
	}
	if hid > 0 {
		if _, err := s.db.Exec(ctx, "update heroes set state=1 where id=?", hid); err != nil {
			return err
		}
		if err := s.syncChiefLoyalty(ctx, cid, hid); err != nil {
			return err
		}
		// updateCityChiefResAdd → updateCityResourceAdd：mem_world 无表 → ownercid=cid。
		if err := s.markResChanging(ctx, cid); err != nil {
			return err
		}
		// completeTask(85) 属 M8，未接线。
	} else {
		if _, err := s.db.Exec(ctx, "update city_resources set chief_loyalty=0 where city_id=?", cid); err != nil {
			return err
		}
		if err := s.markResChanging(ctx, cid); err != nil {
			return err
		}
	}
	// 将领技能：cfg_book/sys_user_book 无表 → 清零后不回填（恒 0）。
	if _, err := s.db.Exec(ctx, `update city_res_add set skill_gold_add=0,skill_food_add=0,
		skill_wood_add=0,skill_rock_add=0,skill_iron_add=0 where city_id=?`, cid); err != nil {
		return err
	}
	if hid != 0 {
		if err := s.markResChanging(ctx, cid); err != nil {
			return err
		}
	}
	_, err = s.db.Exec(ctx, "update cities set chief_hero_id=? where id=?", hid, cid)
	return err
}

// doSetCityGeneral 对齐 OfficeFunc.php:188（设置主将）。
func (s *Service) doSetCityGeneral(ctx context.Context, uid, cid, hid int) error {
	if err := s.checkHeroSelf(ctx, cid, hid); err != nil {
		return err
	}
	oldChief, err := s.cellInt64Zero(ctx, "select general_hero_id from cities where id=? limit 1", cid)
	if err != nil {
		return err
	}
	busy, err := s.heroIsInTroop(ctx, int(oldChief))
	if err != nil {
		return err
	}
	if busy {
		return errHero(msgSetChiefBusy)
	}
	if oldChief > 0 {
		if _, err := s.db.Exec(ctx, "update heroes set state=0 where id=?", oldChief); err != nil {
			return err
		}
	}
	if hid > 0 {
		if _, err := s.db.Exec(ctx, "update heroes set state=7 where id=?", hid); err != nil {
			return err
		}
		if err := s.syncChiefLoyalty(ctx, cid, hid); err != nil {
			return err
		}
	}
	if _, err := s.db.Exec(ctx, "update cities set general_hero_id=? where id=?", hid, cid); err != nil {
		return err
	}
	// completeTaskWithTaskid(328) 属 M8，未接线。
	return nil
}

// doSetCityCounsellor 对齐 OfficeFunc.php:210（设置军师）。
func (s *Service) doSetCityCounsellor(ctx context.Context, uid, cid, hid int) error {
	if err := s.checkHeroSelf(ctx, cid, hid); err != nil {
		return err
	}
	oldChief, err := s.cellInt64Zero(ctx, "select counsellor_hero_id from cities where id=? limit 1", cid)
	if err != nil {
		return err
	}
	busy, err := s.heroIsInTroop(ctx, int(oldChief))
	if err != nil {
		return err
	}
	if busy {
		return errHero(msgSetChiefBusy)
	}
	if oldChief > 0 {
		if _, err := s.db.Exec(ctx, "update heroes set state=0 where id=?", oldChief); err != nil {
			return err
		}
	}
	if hid > 0 {
		if _, err := s.db.Exec(ctx, "update heroes set state=8 where id=?", hid); err != nil {
			return err
		}
		if err := s.syncChiefLoyalty(ctx, cid, hid); err != nil {
			return err
		}
	}
	if _, err := s.db.Exec(ctx, "update cities set counsellor_hero_id=? where id=?", hid, cid); err != nil {
		return err
	}
	// completeTaskWithTaskid(330) 属 M8，未接线。
	return nil
}

// syncChiefLoyalty 对齐 legacy `update mem_city_resource m,sys_city_hero h
// set m.chief_loyalty=h.loyalty where m.cid=$cid and h.hid=$hid`。
func (s *Service) syncChiefLoyalty(ctx context.Context, cid, hid int) error {
	loyalty, err := s.cellInt64Zero(ctx, "select loyalty from heroes where id=? limit 1", hid)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, "update city_resources set chief_loyalty=? where city_id=?", loyalty, cid)
	return err
}
