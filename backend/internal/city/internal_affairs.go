package city

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strconv"

	"rxsg/backend/internal/game"
	"rxsg/backend/internal/httpx"
	"rxsg/backend/internal/model"
)

// cellFloat 读取单列数值并按浮点解析（对齐 legacy sql_fetch_one_cell 返回字符串后数值比较的语义，
// MySQL 表达式 food-people_max*10.0 等返回 decimal，不能直接 Scan 进整型）。
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
		return 0, nil // 对齐 PHP：非数值字符串数值化为 0
	}
	return f, nil
}

// internal_affairs.go 1:1 复刻 legacy server/game/CityFunc.php 的城市内政：
//   changeTax(268) / levyResource(345) / pacifyPeople(406) / getCityProduct(74)
// 数值公式与 lang.php 文案逐条对齐；任务触发（completeTask）属 M8 批次，此处不接线。
// 与 legacy 的差异（新库缺表，按既有模式省略）：
//   - mem_world 世界战乱状态恒为 0（仅保留 sys_troops 战乱判定）；
//   - mem_hero_buffer/mem_user_buffer 符/buff 表无，hufu 恒 1、文曲星符与道具加成恒 0；
//   - cfg_book/sys_user_book 技能书表无，skill_add 恒 [0]。

// levyCooldownSecs / pacifyCooldownSecs：legacy 硬编码 900 秒冷却（CityFunc.php:351/415）。
const (
	levyCooldownSecs   = 900
	pacifyCooldownSecs = 900
)

// makeTimeLeft 对齐 utils.php:63 MakeTimeLeft（lang.php:1720-1722 单位文案）。
func makeTimeLeft(seconds int64) string {
	h := seconds / 3600
	m := (seconds - h*3600) / 60
	s := seconds % 60
	switch {
	case h > 0:
		return fmt.Sprintf("%d小时%d分钟%d秒", h, m, s)
	case m > 0:
		return fmt.Sprintf("%d分钟%d秒", m, s)
	default:
		return fmt.Sprintf("%d秒", s)
	}
}

// addHeroExp 对齐 utils.php:1190：expadd=floor(exp*HERO_EXP_RATE)，herotype!=1000。
func (s *Service) addHeroExp(ctx context.Context, hid int, exp float64) error {
	if hid <= 0 {
		return nil
	}
	expAdd := int64(math.Floor(exp * game.HeroExpRate))
	if expAdd <= 0 {
		return nil
	}
	_, err := s.db.Exec(ctx, "update heroes set exp=exp+? where id=? and hero_type<>1000", expAdd, hid)
	return err
}

// scheduleValue 读取 city_schedule 单列；行缺失时返回 0（对齐 legacy NULL→PHP 数值化 0）。
func (s *Service) scheduleValue(ctx context.Context, cid int, col string) (int64, error) {
	v, err := s.db.FetchCellInt64(ctx, "select `"+col+"` from city_schedule where city_id=?", cid)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return v, err
}

// upsertSchedule 对齐 legacy insert ... on duplicate key update。
func (s *Service) upsertSchedule(ctx context.Context, cid int, col string, now int64) error {
	_, err := s.db.Exec(ctx,
		"insert into city_schedule (city_id, `"+col+"`) values (?,?) on duplicate key update `"+col+"`=?",
		cid, now, now)
	return err
}

// ChangeTax 对齐 changeTax（CityFunc.php:268-291）。
func (s *Service) ChangeTax(ctx context.Context, uid, cid, newtax int) (model.CityResource, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return model.CityResource{}, err
	}
	if newtax < 0 || newtax > 100 {
		return model.CityResource{}, httpx.BadRequest("waigua_invalid", "数据异常")
	}
	if _, err := s.db.Exec(ctx, "update city_resources set tax=? where city_id=?", newtax, cid); err != nil {
		return model.CityResource{}, err
	}
	if _, err := s.db.Exec(ctx,
		"update city_resources set morale_stable=GREATEST(0,LEAST(100-tax-complaint,100)) where city_id=?", cid); err != nil {
		return model.CityResource{}, err
	}
	return s.Resources(ctx, uid, cid)
}

// LevyResource 对齐 levyResource（CityFunc.php:345-405）。resid: 0金/1粮/2木/3石/4铁。
func (s *Service) LevyResource(ctx context.Context, uid, cid, resid int) (string, model.CityResource, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return "", model.CityResource{}, err
	}
	now, err := s.db.Now(ctx)
	if err != nil {
		return "", model.CityResource{}, err
	}
	last, err := s.scheduleValue(ctx, cid, "last_levy_resource")
	if err != nil {
		return "", model.CityResource{}, err
	}
	delta := now - last
	if delta <= levyCooldownSecs {
		return "", model.CityResource{}, httpx.BadRequest("levy_time_limit",
			fmt.Sprintf("%s后才能征收物资。", makeTimeLeft(levyCooldownSecs-delta)))
	}

	row, err := s.db.FetchOne(ctx, "select people,morale,people_max from city_resources where city_id=?", cid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", model.CityResource{}, httpx.NotFound("no_city_info", "城市资源不存在")
		}
		return "", model.CityResource{}, err
	}
	people := float64(model.Int64(row, "people"))
	morale := model.Int(row, "morale")
	if morale <= 20 {
		return "", model.CityResource{}, httpx.BadRequest("not_enough_morale", "民心低于20，不能征收物资。")
	}

	// 征收量 = people × SPEED × 全局速率 × 0.1（浮点入库由 MySQL 舍入，与 legacy 拼接 SQL 一致）。
	type levyDef struct {
		name string
		col  string
		rate float64
	}
	defs := []levyDef{
		{"黄金", "gold", game.GlobalGoldRate},
		{"粮食", "food", game.GlobalFoodRate},
		{"木材", "wood", game.GlobalWoodRate},
		{"石料", "rock", game.GlobalRockRate},
		{"铁锭", "iron", game.GlobalIronRate},
	}
	desc := ""
	if resid >= 0 && resid < len(defs) {
		d := defs[resid]
		amount := people * float64(game.SpeedRate) * d.rate * 0.1
		if _, err := s.db.Exec(ctx, "update city_resources set "+d.col+"="+d.col+"+? where city_id=?", amount, cid); err != nil {
			return "", model.CityResource{}, err
		}
		desc = d.name + fmt.Sprintf("%.0f", math.Floor(amount))
	}
	if _, err := s.db.Exec(ctx, "update city_resources set morale=GREATEST(0,morale-20) where city_id=?", cid); err != nil {
		return "", model.CityResource{}, err
	}
	if _, err := s.db.Exec(ctx, "update city_resources set people_stable=people_max*morale*0.01 where city_id=?", cid); err != nil {
		return "", model.CityResource{}, err
	}
	if err := s.upsertSchedule(ctx, cid, "last_levy_resource", now); err != nil {
		return "", model.CityResource{}, err
	}
	res, err := s.Resources(ctx, uid, cid)
	if err != nil {
		return "", model.CityResource{}, err
	}
	return fmt.Sprintf("成功征收%s,民心降20。", desc), res, nil
}

// inBattle 对齐 pacifyPeople:423 战乱判定（mem_world 无表恒 0；sys_troops→troops，
// 新库行军状态 0=出征中，视为来袭）。
func (s *Service) inBattle(ctx context.Context, cid int) (bool, error) {
	return s.db.Exists(ctx, "select 1 from troops where target_id=? and state=0 limit 1", cid)
}

// PacifyPeople 对齐 pacifyPeople（CityFunc.php:406-557）。action: 0赈灾/1祈福/2祭天/3增丁。
func (s *Service) PacifyPeople(ctx context.Context, uid, cid, action int) (string, model.CityResource, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return "", model.CityResource{}, err
	}

	var message string
	var res model.CityResource
	err := s.lk.WithUserLock(ctx, uid, "pacify", func(ctx context.Context) error {
		now, err := s.db.Now(ctx)
		if err != nil {
			return err
		}
		last, err := s.scheduleValue(ctx, cid, "last_pacify_people")
		if err != nil {
			return err
		}
		delta := now - last
		if delta <= pacifyCooldownSecs {
			return httpx.BadRequest("pacify_time_limit",
				fmt.Sprintf("%s后才能安抚百姓。", makeTimeLeft(pacifyCooldownSecs-delta)))
		}
		battle, err := s.inBattle(ctx, cid)
		if err != nil {
			return err
		}
		if battle {
			return httpx.BadRequest("city_in_battle", "本城目前处于战乱状态，暂时不能安抚！")
		}

		chiefhid, err := s.db.FetchCellInt64(ctx, "select chief_hero_id from cities where id=?", cid)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		peopleMax, err := s.db.FetchCellInt64(ctx, "select people_max from city_resources where city_id=?", cid)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		spd := float64(game.SpeedRate)

		switch action {
		case 0: // 赈灾：耗粮=人口上限×SPEED，民心+5，民怨-15
			d, err := s.cellFloat(ctx, "select food-people_max*? from city_resources where city_id=?", spd, cid)
			if err != nil {
				return err
			}
			if d < 0 {
				return httpx.BadRequest("no_enough_food", "本城的粮食不够。")
			}
			if _, err := s.db.Exec(ctx, `update city_resources set
				food=GREATEST(0,food-people_max*?),
				morale=LEAST(100,morale+5),
				complaint=GREATEST(0,complaint-15),
				morale_stable=GREATEST(0,LEAST(100-tax-complaint,100)) where city_id=?`, spd, cid); err != nil {
				return err
			}
			if err := s.addHeroExp(ctx, int(chiefhid), float64(peopleMax)*spd*game.FoodPrice); err != nil {
				return err
			}
		case 1: // 祈福：耗金=人口上限×SPEED，民心+25，民怨-5
			d, err := s.db.FetchCellInt64(ctx, "select gold-people_max*? from city_resources where city_id=?", spd, cid)
			if err != nil {
				return err
			}
			if d < 0 {
				return httpx.BadRequest("no_enough_gold", "本城的黄金不够。")
			}
			if _, err := s.db.Exec(ctx, `update city_resources set
				gold=GREATEST(0,gold-people_max*?),
				morale=LEAST(100,morale+25),
				complaint=GREATEST(0,complaint-5),
				morale_stable=GREATEST(0,LEAST(100-tax-complaint,100)) where city_id=?`, spd, cid); err != nil {
				return err
			}
			if err := s.addHeroExp(ctx, int(chiefhid), float64(peopleMax)*spd); err != nil {
				return err
			}
		case 2: // 祭天：耗粮=人口上限×SPEED、耗金=10%×SPEED；天灾推迟 1 天+rand(0,3天)；10% 天赐
			fd, err := s.db.FetchCellInt64(ctx, "select food-people_max*? from city_resources where city_id=?", spd, cid)
			if err != nil {
				return err
			}
			gd, err := s.db.FetchCellInt64(ctx, "select gold-people_max*0.1*? from city_resources where city_id=?", spd, cid)
			if err != nil {
				return err
			}
			if fd < 0 {
				return httpx.BadRequest("no_enough_food", "本城的粮食不够。")
			}
			if gd < 0 {
				return httpx.BadRequest("no_enough_gold", "本城的黄金不够。")
			}
			nbe, err := s.scheduleValue(ctx, cid, "next_bad_event")
			if err != nil {
				return err
			}
			// legacy: nb - (nb+8*3600)%86400 + 86400 + rand(0,259200)
			nbe = nbe - (nbe+8*3600)%86400 + 86400 + rand.Int63n(259201)
			if _, err := s.db.Exec(ctx, "update city_schedule set next_bad_event=? where city_id=?", nbe, cid); err != nil {
				return err
			}
			if rand.Intn(10) == 0 {
				if _, err := s.db.Exec(ctx, "update city_schedule set next_good_event=? where city_id=?", now, cid); err != nil {
					return err
				}
			}
			if _, err := s.db.Exec(ctx, `update city_resources set
				food=GREATEST(0,food-people_max*?),
				gold=GREATEST(0,gold-people_max*0.1*?) where city_id=?`, spd, spd, cid); err != nil {
				return err
			}
			if err := s.addHeroExp(ctx, int(chiefhid), float64(peopleMax)*spd*(game.FoodPrice+1)); err != nil {
				return err
			}
		case 3: // 增丁：耗粮=人口上限×5×SPEED，人口+人口上限×5%×SPEED（不超上限）
			fd, err := s.db.FetchCellInt64(ctx, "select food-people_max*5*? from city_resources where city_id=?", spd, cid)
			if err != nil {
				return err
			}
			if fd < 0 {
				return httpx.BadRequest("no_enough_food", "本城的粮食不够。")
			}
			if _, err := s.db.Exec(ctx, `update city_resources set
				food=food-people_max*5*?,
				people=LEAST(people_max,people+FLOOR(people_max*?*0.05)) where city_id=?`, spd, spd, cid); err != nil {
				return err
			}
			if err := s.addHeroExp(ctx, int(chiefhid), float64(peopleMax)*5*spd*game.FoodPrice); err != nil {
				return err
			}
		}

		if _, err := s.db.Exec(ctx, "update city_resources set people_stable=people_max*morale*0.01 where city_id=?", cid); err != nil {
			return err
		}
		if err := s.upsertSchedule(ctx, cid, "last_pacify_people", now); err != nil {
			return err
		}
		message = "成功安抚百姓"
		res, err = s.Resources(ctx, uid, cid)
		return err
	})
	if err != nil {
		if err.Error() == "lock_busy" {
			return "", model.CityResource{}, httpx.BadRequest("server_busy", "服务器忙，请稍后再进行操作。")
		}
		return "", model.CityResource{}, err
	}
	return message, res, nil
}

// ProductInfo 对齐 getCityProduct（CityFunc.php:74-234）返回的产出明细。
type ProductInfo struct {
	FoodRate int `json:"foodRate"`
	WoodRate int `json:"woodRate"`
	RockRate int `json:"rockRate"`
	IronRate int `json:"ironRate"`

	FoodPeople int64 `json:"foodPeople"`
	WoodPeople int64 `json:"woodPeople"`
	RockPeople int64 `json:"rockPeople"`
	IronPeople int64 `json:"ironPeople"`

	FoodBase float64 `json:"foodBase"`
	WoodBase float64 `json:"woodBase"`
	RockBase float64 `json:"rockBase"`
	IronBase float64 `json:"ironBase"`

	FoodTechnic int `json:"foodTechnic"`
	WoodTechnic int `json:"woodTechnic"`
	RockTechnic int `json:"rockTechnic"`
	IronTechnic int `json:"ironTechnic"`

	FoodArmyUse int64   `json:"foodArmyUse"`
	ChiefAdd    float64 `json:"chiefAdd"`

	SkillGold int `json:"skillGold"`
	SkillFood int `json:"skillFood"`
	SkillWood int `json:"skillWood"`
	SkillRock int `json:"skillRock"`
	SkillIron int `json:"skillIron"`
}

// GetCityProduct 对齐 getCityProduct：只读计算城市产出（劳力/基础/科技/城守加成）。
func (s *Service) GetCityProduct(ctx context.Context, uid, cid int) (*ProductInfo, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return nil, err
	}
	// 保证 city_res_add 行存在（legacy:77-81）。
	if _, err := s.db.Exec(ctx, "insert ignore into city_res_add (city_id) values (?)", cid); err != nil {
		return nil, err
	}
	addRow, err := s.db.FetchOne(ctx, "select * from city_res_add where city_id=?", cid)
	if err != nil {
		return nil, err
	}

	out := &ProductInfo{
		FoodRate: model.Int(addRow, "food_rate"),
		WoodRate: model.Int(addRow, "wood_rate"),
		RockRate: model.Int(addRow, "rock_rate"),
		IronRate: model.Int(addRow, "iron_rate"),

		SkillGold: model.Int(addRow, "skill_gold_add"),
		SkillFood: model.Int(addRow, "skill_food_add"),
		SkillWood: model.Int(addRow, "skill_wood_add"),
		SkillRock: model.Int(addRow, "skill_rock_add"),
		SkillIron: model.Int(addRow, "skill_iron_add"),
	}

	// 劳力需求：Σ using_people（按建筑等级），bid 2-5 对应农/木/石/铁。
	peopleOf := func(bid int) (int64, error) {
		v, err := s.db.FetchCellInt64(ctx, `select coalesce(sum(l.using_people),0)
			from buildings b join cfg_building_levels l on b.building_id=l.bid and b.level=l.level
			where b.city_id=? and b.building_id=?`, cid, bid)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return v, err
	}
	if out.FoodPeople, err = peopleOf(game.BidFarmland); err != nil {
		return nil, err
	}
	if out.WoodPeople, err = peopleOf(game.BidWood); err != nil {
		return nil, err
	}
	if out.RockPeople, err = peopleOf(game.BidRock); err != nil {
		return nil, err
	}
	if out.IronPeople, err = peopleOf(game.BidIron); err != nil {
		return nil, err
	}
	spd := float64(game.SpeedRate)
	out.FoodBase = game.GlobalFoodRate * float64(out.FoodPeople) * spd
	out.WoodBase = game.GlobalWoodRate * float64(out.WoodPeople) * spd
	out.RockBase = game.GlobalRockRate * float64(out.RockPeople) * spd
	out.IronBase = game.GlobalIronRate * float64(out.IronPeople) * spd

	// 科技加成：level×10（legacy:93-96）。
	techOf := func(tid int) (int, error) {
		v, err := s.db.FetchCellInt64(ctx, "select level*10 from city_technics where technic_id=? and city_id=?", tid, cid)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return int(v), err
	}
	if out.FoodTechnic, err = techOf(game.TechnicFood); err != nil {
		return nil, err
	}
	if out.WoodTechnic, err = techOf(game.TechnicWood); err != nil {
		return nil, err
	}
	if out.RockTechnic, err = techOf(game.TechnicRock); err != nil {
		return nil, err
	}
	if out.IronTechnic, err = techOf(game.TechnicIron); err != nil {
		return nil, err
	}

	// 军粮消耗（legacy:182 读 mem_city_resource.food_army_use）。
	fau, err := s.db.FetchCellInt64(ctx, "select food_army_use from city_resources where city_id=?", cid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	out.FoodArmyUse = fau

	// 城守加成（legacy:101-126）。
	chiefhid, err := s.db.FetchCellInt64(ctx, "select chief_hero_id from cities where id=?", cid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if chiefhid > 0 {
		h, err := s.db.FetchOne(ctx, "select * from heroes where id=? and city_id=?", chiefhid, cid)
		if err == nil {
			chiefAdd := float64(model.Int(h, "affairs_base") + model.Int(h, "affairs_add") + model.Int(h, "affairs_add_on"))
			heroCommand := float64(model.Int(h, "level") + model.Int(h, "command_base") + model.Int(h, "command_add_on"))
			cityPeopleMax, err := s.db.FetchCellInt64(ctx, "select people_max from city_resources where city_id=?", cid)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return nil, err
			}
			hufu := 1.0 // mem_hero_buffer buftype=1 无表，恒 1
			leaderTech, err := s.db.FetchCellInt64(ctx, "select level from city_technics where city_id=? and technic_id=?", cid, game.TechnicLeader)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return nil, err
			}
			peoplerate := heroCommand * 10.0 * (100*hufu + float64(leaderTech)*10) / float64(cityPeopleMax+1)
			if peoplerate > 1.0 {
				peoplerate = 1.0
			}
			chiefAdd *= peoplerate
			// 文曲星符（buftype=2）无表，×1.25 省略。
			out.ChiefAdd = chiefAdd
		}
	}
	return out, nil
}
