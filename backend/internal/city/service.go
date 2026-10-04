package city

import (
	"context"
	"database/sql"

	"rxsg/backend/internal/building"
	"rxsg/backend/internal/db"
	"rxsg/backend/internal/httpx"
	"rxsg/backend/internal/model"
)

type Service struct {
	db  *db.DB
	bld *building.Service
}

func NewService(d *db.DB, bld *building.Service) *Service { return &Service{db: d, bld: bld} }

// CityDetail 对齐 doGetCityAllInfo（utils.php:846）的返回顺序：
// [城市, 基础信息, 建筑, 科技, 州郡]。
type CityDetail struct {
	City      model.City       `json:"city"`
	Base      BaseInfo         `json:"base"`
	Buildings []model.Building `json:"buildings"`
	Technics  []model.Technic  `json:"technics"`
	Province  model.Province   `json:"province"`
}

// BaseInfo 对齐 doGetCityBaseInfo（utils.php:798）。
type BaseInfo struct {
	User        model.User         `json:"user"`
	Resource    model.CityResource `json:"resource"`
	Heroes      []model.Hero       `json:"heroes"`
	Troops      []model.Soldier    `json:"troops"`
	Defences    []model.Defence    `json:"defences"`
	Alarm       model.Alarm        `json:"alarm"`
	OpenLottery int                `json:"openLottery"`
}

// ensureOwner 对齐 checkCityOwner（utils.php:211）。
func (s *Service) ensureOwner(ctx context.Context, uid, cid int) error {
	ok, err := s.db.Exists(ctx, "select 1 from cities where id=? and user_id=? limit 1", cid, uid)
	if err != nil {
		return err
	}
	if !ok {
		return httpx.Forbidden("not_user_city", "该城池不属于当前用户")
	}
	return nil
}

func (s *Service) ListCities(ctx context.Context, uid int) ([]model.City, error) {
	rows, err := s.db.FetchRows(ctx, "select * from cities where user_id=?", uid)
	if err != nil {
		return nil, err
	}
	out := make([]model.City, 0, len(rows))
	for _, r := range rows {
		out = append(out, model.CityFromMap(r))
	}
	return out, nil
}

func (s *Service) GetCityDetail(ctx context.Context, uid, cid int) (*CityDetail, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return nil, err
	}

	cityRow, err := s.db.FetchOne(ctx, "select * from cities where id=?", cid)
	if err == sql.ErrNoRows {
		return nil, httpx.NotFound("no_city_info", "城市不存在")
	}
	if err != nil {
		return nil, err
	}
	city := model.CityFromMap(cityRow)

	base, err := s.BaseInfo(ctx, uid, cid)
	if err != nil {
		return nil, err
	}
	buildings, err := s.Buildings(ctx, cid)
	if err != nil {
		return nil, err
	}
	technics, err := s.Technics(ctx, cid)
	if err != nil {
		return nil, err
	}

	return &CityDetail{
		City:      city,
		Base:      *base,
		Buildings: buildings,
		Technics:  technics,
		Province:  model.Province{},
	}, nil
}

func (s *Service) BaseInfo(ctx context.Context, uid, cid int) (*BaseInfo, error) {
	userRow, err := s.db.FetchOne(ctx, "select * from users where id=?", uid)
	if err == sql.ErrNoRows {
		return nil, httpx.NotFound("user_not_found", "用户不存在")
	}
	if err != nil {
		return nil, err
	}

	resRow, err := s.db.FetchOne(ctx, "select * from city_resources where city_id=?", cid)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	heroRows, err := s.db.FetchRows(ctx, "select * from heroes where city_id=? and user_id=?", cid, uid)
	if err != nil {
		return nil, err
	}
	heroes := make([]model.Hero, 0, len(heroRows))
	for _, r := range heroRows {
		h := model.HeroFromMap(r)
		h.CurCID = s.heroCurCID(ctx, h)
		heroes = append(heroes, h)
	}

	soldierRows, err := s.db.FetchRows(ctx, "select * from city_soldiers where city_id=? order by soldier_id", cid)
	if err != nil {
		return nil, err
	}
	troops := make([]model.Soldier, 0, len(soldierRows))
	for _, r := range soldierRows {
		troops = append(troops, model.SoldierFromMap(r))
	}

	return &BaseInfo{
		User:     model.UserFromMap(userRow),
		Resource: model.CityResourceFromMap(resRow),
		Heroes:   heroes,
		Troops:   troops,
		Defences: []model.Defence{},
		Alarm:    model.Alarm{},
	}, nil
}

// heroCurCID 对齐 doGetHeroState（interface.php:173）：state==4（驻守）时取 troops.target_id。
func (s *Service) heroCurCID(ctx context.Context, h model.Hero) int {
	if h.State != 4 {
		return h.CID
	}
	target, err := s.db.FetchCellInt64(ctx, "select target_id from troops where hero_id=? limit 1", h.HID)
	if err != nil {
		return h.CID
	}
	return int(target)
}

// Buildings 委托 building.Service：含惰性结算与联表建筑名。
func (s *Service) Buildings(ctx context.Context, cid int) ([]model.Building, error) {
	return s.bld.List(ctx, cid)
}

// Technics 对齐 utils.php:859。
func (s *Service) Technics(ctx context.Context, cid int) ([]model.Technic, error) {
	rows, err := s.db.FetchRows(ctx, "select technic_id,level from city_technics where city_id=?", cid)
	if err != nil {
		return nil, err
	}
	out := make([]model.Technic, 0, len(rows))
	for _, r := range rows {
		out = append(out, model.TechnicFromMap(r))
	}
	return out, nil
}

// Resources 是心跳接口：只读返回 city_resources 当前值。
// TODO(phase2): 资源按时间累积写库。
func (s *Service) Resources(ctx context.Context, uid, cid int) (model.CityResource, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return model.CityResource{}, err
	}
	row, err := s.db.FetchOne(ctx, "select * from city_resources where city_id=?", cid)
	if err == sql.ErrNoRows {
		return model.CityResource{}, httpx.NotFound("no_city_info", "城市资源不存在")
	}
	if err != nil {
		return model.CityResource{}, err
	}
	return model.CityResourceFromMap(row), nil
}

func (s *Service) GetBuildings(ctx context.Context, uid, cid int) ([]model.Building, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return nil, err
	}
	return s.Buildings(ctx, cid)
}

func (s *Service) GetTechnics(ctx context.Context, uid, cid int) ([]model.Technic, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return nil, err
	}
	return s.Technics(ctx, cid)
}

func (s *Service) GetTroops(ctx context.Context, uid, cid int) ([]model.Soldier, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return nil, err
	}
	base, err := s.BaseInfo(ctx, uid, cid)
	if err != nil {
		return nil, err
	}
	return base.Troops, nil
}

func (s *Service) GetDefences(ctx context.Context, uid, cid int) ([]model.Defence, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return nil, err
	}
	return []model.Defence{}, nil
}

func (s *Service) GetHeroes(ctx context.Context, uid, cid int) ([]model.Hero, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return nil, err
	}
	base, err := s.BaseInfo(ctx, uid, cid)
	if err != nil {
		return nil, err
	}
	return base.Heroes, nil
}

func (s *Service) GetAlarms(ctx context.Context, uid, cid int) (model.Alarm, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return model.Alarm{}, err
	}
	return model.Alarm{}, nil
}