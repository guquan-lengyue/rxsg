package model

// City 对应 sys_city（utils.php:849 select *）。
type City struct {
	CID        int    `json:"cid"`
	UID        int    `json:"uid"`
	Name       string `json:"name"`
	IsSpecial  int    `json:"isSpecial"`
	SpacialSid int    `json:"spacialSid"` // 由 getSpacialSoldierId(cid) 附加（utils.php:851）
}

func CityFromMap(m map[string]any) City {
	return City{
		CID:       Int(m, "cid"),
		UID:       Int(m, "uid"),
		Name:      Str(m, "name"),
		IsSpecial: Int(m, "is_special"),
	}
}

// CityResource 对应 mem_city_resource（utils.php:828）。
type CityResource struct {
	CID            int   `json:"cid"`
	Wood           int64 `json:"wood"`
	WoodAdd        int64 `json:"woodAdd"`
	WoodMax        int64 `json:"woodMax"`
	Rock           int64 `json:"rock"`
	RockAdd        int64 `json:"rockAdd"`
	RockMax        int64 `json:"rockMax"`
	Iron           int64 `json:"iron"`
	IronAdd        int64 `json:"ironAdd"`
	IronMax        int64 `json:"ironMax"`
	Food           int64 `json:"food"`
	FoodAdd        int64 `json:"foodAdd"`
	FoodMax        int64 `json:"foodMax"`
	FoodArmyUse    int64 `json:"foodArmyUse"`
	Gold           int64 `json:"gold"`
	GoldRate       int   `json:"goldRate"`
	GoldMax        int64 `json:"goldMax"`
	People         int64 `json:"people"`
	PeopleMax      int64 `json:"peopleMax"`
	PeopleStable   int64 `json:"peopleStable"`
	PeopleWorking  int64 `json:"peopleWorking"`
	PeopleBuilding int64 `json:"peopleBuilding"`
	Morale         int   `json:"morale"`
	Tax            int   `json:"tax"`
	Complaint      int   `json:"complaint"`
	HeroFee        int64 `json:"heroFee"`
	Vacation       int   `json:"vacation"`
	Forbidden      int   `json:"forbidden"`
}

func CityResourceFromMap(m map[string]any) CityResource {
	return CityResource{
		CID:            Int(m, "cid"),
		Wood:           Int64(m, "wood"),
		WoodAdd:        Int64(m, "wood_add"),
		WoodMax:        Int64(m, "wood_max"),
		Rock:           Int64(m, "rock"),
		RockAdd:        Int64(m, "rock_add"),
		RockMax:        Int64(m, "rock_max"),
		Iron:           Int64(m, "iron"),
		IronAdd:        Int64(m, "iron_add"),
		IronMax:        Int64(m, "iron_max"),
		Food:           Int64(m, "food"),
		FoodAdd:        Int64(m, "food_add"),
		FoodMax:        Int64(m, "food_max"),
		FoodArmyUse:    Int64(m, "food_army_use"),
		Gold:           Int64(m, "gold"),
		GoldRate:       Int(m, "gold_rate"),
		GoldMax:        Int64(m, "gold_max"),
		People:         Int64(m, "people"),
		PeopleMax:      Int64(m, "people_max"),
		PeopleStable:   Int64(m, "people_stable"),
		PeopleWorking:  Int64(m, "people_working"),
		PeopleBuilding: Int64(m, "people_building"),
		Morale:         Int(m, "morale"),
		Tax:            Int(m, "tax"),
		Complaint:      Int(m, "complaint"),
		HeroFee:        Int64(m, "hero_fee"),
		Vacation:       Int(m, "vacation"),
		Forbidden:      Int(m, "forbidden"),
	}
}

// Alarm 对应 sys_alarm（utils.php:838）。
type Alarm struct {
	UID  int `json:"uid"`
	Task int `json:"task"`
	Mail int `json:"mail"`
}

func AlarmFromMap(m map[string]any) Alarm {
	return Alarm{UID: Int(m, "uid"), Task: Int(m, "task"), Mail: Int(m, "mail")}
}