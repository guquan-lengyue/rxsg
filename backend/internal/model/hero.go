package model

// Hero 对应 sys_city_hero h left join mem_hero_blood m（utils.php:832）。
type Hero struct {
	HID         int    `json:"hid"`
	UID         int    `json:"uid"`
	CID         int    `json:"cid"`
	Name        string `json:"name"`
	Sex         int    `json:"sex"`
	Face        int    `json:"face"`
	State       int    `json:"state"`
	Level       int    `json:"level"`
	HeroType    int    `json:"herotype"`
	CommandBase int    `json:"command_base"`
	AffairsBase int    `json:"affairs_base"`
	BraveryBase int    `json:"bravery_base"`
	WisdomBase  int    `json:"wisdom_base"`
	Loyalty     int    `json:"loyalty"`
	Force       int    `json:"force"`
	ForceMax    int    `json:"force_max"`
	Energy      int    `json:"energy"`
	EnergyMax   int    `json:"energy_max"`
	CurCID      int    `json:"curCid"` // 对齐 doGetHeroState：state==4 时取 sys_troops.targetcid
}

func HeroFromMap(m map[string]any) Hero {
	return Hero{
		HID:         Int(m, "hid"),
		UID:         Int(m, "uid"),
		CID:         Int(m, "cid"),
		Name:        Str(m, "name"),
		Sex:         Int(m, "sex"),
		Face:        Int(m, "face"),
		State:       Int(m, "state"),
		Level:       Int(m, "level"),
		HeroType:    Int(m, "herotype"),
		CommandBase: Int(m, "command_base"),
		AffairsBase: Int(m, "affairs_base"),
		BraveryBase: Int(m, "bravery_base"),
		WisdomBase:  Int(m, "wisdom_base"),
		Loyalty:     Int(m, "loyalty"),
		Force:       Int(m, "force"),
		ForceMax:    Int(m, "force_max"),
		Energy:      Int(m, "energy"),
		EnergyMax:   Int(m, "energy_max"),
	}
}