package hero

import (
	"context"
	"strconv"

	"rxsg/backend/internal/model"
)

// office_info.go —— R11-3 官署面板聚合读取端点。
//
// 对齐 legacy OfficeFunc.php：
//   getOfficeInfo(18)            官署建筑（无 → throw no_office_built，lang 原文 "铁锭"，错位保留）
//   doGetOfficeValidPosition(12) 官署空位 = 官署等级 − 城内将领数
//   doGetCityHero(6)             城内将领列表（复用 hero.Info）
//   doGetBuildingInfo:262-269    爵位（getBufferNobility(1534)，推恩令 buftype 16/18）
//
// 说明：任命/卸任写操作（setCityChief:30 全链）已在 M3 实现（office.go Service.SetChief）。
// 本端点只做面板所需的只读聚合，不改既有行为。

// OfficeInfo 官署面板信息（GET /cities/:cid/office）。
type OfficeInfo struct {
	OfficeLevel      int         `json:"office_level"`         // 官署等级（getOfficeInfo，最高级官署建筑）
	ValidPosition    int         `json:"valid_position"`       // 官署空位（doGetOfficeValidPosition）
	Nobility         int         `json:"nobility"`             // 爵位（含推恩令）
	ChiefHeroID      int         `json:"chief_hero_id"`        // 城守
	GeneralHeroID    int         `json:"general_hero_id"`      // 主将
	CounsellorHeroID int         `json:"counsellor_hero_id"`   // 军师
	Heroes           []HeroState `json:"heroes"`               // 城内将领
}

// OfficeInfo 读取官署面板聚合信息。
func (s *Service) OfficeInfo(ctx context.Context, uid, cid int) (*OfficeInfo, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return nil, err
	}
	// getOfficeInfo:20-25：无官署建筑（bid=11）→ throw no_office_built。
	ok, err := s.db.Exists(ctx,
		"select 1 from buildings where city_id=? and building_id=? limit 1", cid, officeBuildingID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errHero(msgNoOfficeBuilt)
	}
	officeLevel, err := s.cellInt64Zero(ctx,
		"select level from buildings where city_id=? and building_id=? order by level desc limit 1", cid, officeBuildingID)
	if err != nil {
		return nil, err
	}
	// doGetOfficeValidPosition:14-16：官署等级 − 城内将领数。
	heroCount, err := s.cellInt64Zero(ctx, "select count(*) from heroes where user_id=? and city_id=?", uid, cid)
	if err != nil {
		return nil, err
	}
	// 爵位：getBufferNobility（推恩令）。
	nobility, err := s.bufferNobility(ctx, uid)
	if err != nil {
		return nil, err
	}
	city, err := s.db.FetchOne(ctx, "select chief_hero_id, general_hero_id, counsellor_hero_id from cities where id=?", cid)
	if err != nil {
		return nil, err
	}
	info, err := s.Info(ctx, uid, cid)
	if err != nil {
		return nil, err
	}
	return &OfficeInfo{
		OfficeLevel:      int(officeLevel),
		ValidPosition:    int(officeLevel) - int(heroCount),
		Nobility:         nobility,
		ChiefHeroID:      model.Int(city, "chief_hero_id"),
		GeneralHeroID:    model.Int(city, "general_hero_id"),
		CounsellorHeroID: model.Int(city, "counsellor_hero_id"),
		Heroes:           info.Heroes,
	}, nil
}

// bufferNobility 对齐 utils.php:1534 getBufferNobility：推恩令 buftype∈{16,18} 提升爵位（封顶 19/18）。
func (s *Service) bufferNobility(ctx context.Context, uid int) (int, error) {
	nobilityStr, err := s.db.FetchCellString(ctx, "select nobility from users where id=? limit 1", uid)
	if err != nil {
		return 0, err
	}
	real, _ := strconv.ParseFloat(nobilityStr, 64)
	bufparam, err := s.cellInt64Zero(ctx,
		"select bufparam from user_buffers where user_id=? and (buftype=16 or buftype=18) order by bufparam desc limit 1", uid)
	if err != nil {
		return 0, err
	}
	if bufparam != 0 {
		n := int64(real) + bufparam
		if bufparam == 5 && n > 19 {
			n = 19
		}
		if bufparam == 2 && n > 18 {
			n = 18
		}
		if float64(n) > real {
			return int(n), nil
		}
	}
	return int(real), nil
}
