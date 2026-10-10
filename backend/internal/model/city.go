package model

// City 对应新库 cities 表。
type City struct {
	CID        int    `json:"cid"`
	UID        int    `json:"uid"`
	Name       string `json:"name"`
	IsSpecial  int    `json:"isSpecial"`
	SpacialSid int    `json:"spacialSid"` // 新库无专属兵种表，恒为 0
}

func CityFromMap(m map[string]any) City {
	return City{
		CID:       Int(m, "id"),
		UID:       Int(m, "user_id"),
		Name:      Str(m, "name"),
		IsSpecial: Int(m, "is_special"),
	}
}

// CityResource 对应新库 city_resources 表；未提供的产出/用工字段补 0。
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
	MoraleStable   int   `json:"moraleStable"`
	Tax            int   `json:"tax"`
	Complaint      int   `json:"complaint"`
	HeroFee        int64 `json:"heroFee"`
	Vacation       int   `json:"vacation"`
	Forbidden      int   `json:"forbidden"`
}

func CityResourceFromMap(m map[string]any) CityResource {
	return CityResource{
		CID:          Int(m, "city_id"),
		Wood:         Int64(m, "wood"),
		WoodMax:      Int64(m, "wood_max"),
		Rock:         Int64(m, "rock"),
		RockMax:      Int64(m, "rock_max"),
		Iron:         Int64(m, "iron"),
		IronMax:      Int64(m, "iron_max"),
		Food:         Int64(m, "food"),
		FoodMax:      Int64(m, "food_max"),
		Gold:         Int64(m, "gold"),
		GoldRate:     Int(m, "gold_rate"),
		GoldMax:      Int64(m, "gold_max"),
		People:       Int64(m, "people"),
		PeopleMax:    Int64(m, "people_max"),
		PeopleStable: Int64(m, "people_stable"),
		FoodArmyUse:  Int64(m, "food_army_use"),
		Morale:       Int(m, "morale"),
		MoraleStable: Int(m, "morale_stable"),
		Tax:          Int(m, "tax"),
		Complaint:    Int(m, "complaint"),
		Vacation:     Int(m, "vacation"),
	}
}

// Alarm 对齐 legacy sys_alarm 的顶栏红点字段（前端 api/city.ts::Alarm）。
// 新库 alarms(user_id, task, report)：Task=是否有可领取任务(task)，Mail=未读战报(report)
//（旧库 mail=未读系统邮件；新库无邮件子系统，见 city.alarmOf 说明）。
type Alarm struct {
	UID  int `json:"uid"`
	Task int `json:"task"`
	Mail int `json:"mail"`
}

func AlarmFromMap(m map[string]any) Alarm {
	return Alarm{UID: Int(m, "uid")}
}
