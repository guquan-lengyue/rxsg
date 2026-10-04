package city

import (
	"context"
	"database/sql"

	"rxsg/backend/internal/db"
	"rxsg/backend/internal/httpx"
	"rxsg/backend/internal/model"
)

type Service struct {
	db *db.DB
}

func NewService(d *db.DB) *Service { return &Service{db: d} }

// CityDetail 对齐 doGetCityAllInfo（utils.php:846）的返回顺序：
// [sys_city, 基础信息, 建筑, 科技, 州郡]。
type CityDetail struct {
	City      model.City       `json:"city"`
	Base      BaseInfo         `json:"base"`
	Buildings []model.Building `json:"buildings"`
	Technics  []model.Technic  `json:"technics"`
	Province  model.Province   `json:"province"`
}

// BaseInfo 对齐 doGetCityBaseInfo（utils.php:798）。
// 首期省略 isAdult/onLineTime/openIndex/designations/currentYear 等需额外表的分支。
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
	ok, err := s.db.Exists(ctx, "select 1 from sys_city where cid=? and uid=? limit 1", cid, uid)
	if err != nil {
		return err
	}
	if !ok {
		return httpx.Forbidden("not_user_city", "该城池不属于当前用户")
	}
	return nil
}

func (s *Service) ListCities(ctx context.Context, uid int) ([]model.City, error) {
	rows, err := s.db.FetchRows(ctx, "select * from sys_city where uid=?", uid)
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

	cityRow, err := s.db.FetchOne(ctx, "select * from sys_city where cid=?", cid)
	if err == sql.ErrNoRows {
		return nil, httpx.NotFound("no_city_info", "城市不存在")
	}
	if err != nil {
		return nil, err
	}
	city := model.CityFromMap(cityRow)
	if sp, serr := s.specialSoldierID(ctx, cid); serr == nil {
		city.SpacialSid = sp
	}

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
	province, err := s.Province(ctx, cid)
	if err != nil {
		return nil, err
	}

	return &CityDetail{
		City:      city,
		Base:      *base,
		Buildings: buildings,
		Technics:  technics,
		Province:  province,
	}, nil
}

func (s *Service) BaseInfo(ctx context.Context, uid, cid int) (*BaseInfo, error) {
	userRow, err := s.db.FetchOne(ctx, "select * from sys_user where uid=?", uid)
	if err == sql.ErrNoRows {
		return nil, httpx.NotFound("user_not_found", "用户不存在")
	}
	if err != nil {
		return nil, err
	}

	openLottery, _ := s.db.FetchCellInt64(ctx, "select value from mem_state where state=150")

	resRow, err := s.db.FetchOne(ctx, "select * from mem_city_resource where cid=?", cid)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	heroRows, err := s.db.FetchRows(ctx,
		"select * from sys_city_hero h left join mem_hero_blood m on m.hid=h.hid where h.cid=? and h.uid=?", cid, uid)
	if err != nil {
		return nil, err
	}
	heroes := make([]model.Hero, 0, len(heroRows))
	for _, r := range heroRows {
		h := model.HeroFromMap(r)
		h.CurCID = s.heroCurCID(ctx, h)
		heroes = append(heroes, h)
	}

	soldierRows, err := s.db.FetchRows(ctx, "select * from sys_city_soldier where cid=? order by sid", cid)
	if err != nil {
		return nil, err
	}
	troops := make([]model.Soldier, 0, len(soldierRows))
	for _, r := range soldierRows {
		troops = append(troops, model.SoldierFromMap(r))
	}

	defRows, err := s.db.FetchRows(ctx, "select * from sys_city_defence where cid=? order by did", cid)
	if err != nil {
		return nil, err
	}
	defences := make([]model.Defence, 0, len(defRows))
	for _, r := range defRows {
		defences = append(defences, model.DefenceFromMap(r))
	}

	alarmRow, err := s.db.FetchOne(ctx, "select * from sys_alarm where uid=?", uid)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	return &BaseInfo{
		User:        model.UserFromMap(userRow),
		Resource:    model.CityResourceFromMap(resRow),
		Heroes:      heroes,
		Troops:      troops,
		Defences:    defences,
		Alarm:       model.AlarmFromMap(alarmRow),
		OpenLottery: int(openLottery),
	}, nil
}

// heroCurCID 对齐 doGetHeroState（interface.php:173）：state==4（驻守）时取 sys_troops.targetcid。
func (s *Service) heroCurCID(ctx context.Context, h model.Hero) int {
	if h.State != 4 {
		return h.CID
	}
	target, err := s.db.FetchCellInt64(ctx, "select targetcid from sys_troops where hid=? limit 1", h.HID)
	if err != nil {
		return h.CID
	}
	return int(target)
}

// Buildings 对齐 getCityBuildingInfo（utils.php:776-792）。
func (s *Service) Buildings(ctx context.Context, cid int) ([]model.Building, error) {
	rows, err := s.db.FetchRows(ctx,
		"select b.*,c.name as bname,c.description as buildingDescription,l.description as level_description,l.using_people "+
			"from cfg_building c,sys_building b left join cfg_building_level l on l.bid=b.bid and l.`level`=b.`level` "+
			"where b.`cid`=? and c.`bid`=b.`bid`", cid)
	if err != nil {
		return nil, err
	}
	now, err := s.db.Now(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]model.Building, 0, len(rows))
	for _, r := range rows {
		b := model.BuildingFromMap(r)
		b.StateTimeLeft = b.StateEndTime - now
		out = append(out, b)
	}
	return out, nil
}

// Technics 对齐 utils.php:859。
func (s *Service) Technics(ctx context.Context, cid int) ([]model.Technic, error) {
	rows, err := s.db.FetchRows(ctx, "select tid,level from sys_city_technic where cid=?", cid)
	if err != nil {
		return nil, err
	}
	out := make([]model.Technic, 0, len(rows))
	for _, r := range rows {
		out = append(out, model.TechnicFromMap(r))
	}
	return out, nil
}

// Province 对齐 utils.php:861。
func (s *Service) Province(ctx context.Context, cid int) (model.Province, error) {
	row, err := s.db.FetchOne(ctx, "select province,jun from mem_world where wid=?", cid2wid(cid))
	if err == sql.ErrNoRows {
		return model.Province{}, nil
	}
	if err != nil {
		return model.Province{}, err
	}
	return model.ProvinceFromMap(row), nil
}

// specialSoldierID 对齐 getSpacialSoldierId（utils.php:865）。
func (s *Service) specialSoldierID(ctx context.Context, cid int) (int, error) {
	v, err := s.db.FetchCellInt64(ctx, "select sid from cfg_soldier_special_city where cid=? and type<>'4'", cid)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return int(v), nil
}

// Resources 是心跳接口：只读返回 mem_city_resource 当前值。
// TODO(phase2): legacy getCityInfo 会触发 SetCityBaseProduce(utils.php:2145) 与
// UpdateUsersCityResource(utils.php:2329) 写库累积，首期只读不写。
func (s *Service) Resources(ctx context.Context, uid, cid int) (model.CityResource, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return model.CityResource{}, err
	}
	row, err := s.db.FetchOne(ctx, "select * from mem_city_resource where cid=?", cid)
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
	base, err := s.BaseInfo(ctx, uid, cid)
	if err != nil {
		return nil, err
	}
	return base.Defences, nil
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
	row, err := s.db.FetchOne(ctx, "select * from sys_alarm where uid=?", uid)
	if err != nil && err != sql.ErrNoRows {
		return model.Alarm{}, err
	}
	return model.AlarmFromMap(row), nil
}

// cid2wid 对齐 utils.php:663。
func cid2wid(cid int) int {
	y := cid / 1000
	x := cid % 1000
	return (y/10)*10000 + (x/10)*100 + (y%10)*10 + (x%10)
}