package model

// Hero 对应新库 heroes 表；force/energy 等新库未提供的字段补 0。
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
	CurCID      int    `json:"curCid"`
}

func HeroFromMap(m map[string]any) Hero {
	return Hero{
		HID:         Int(m, "id"),
		UID:         Int(m, "user_id"),
		CID:         Int(m, "city_id"),
		Name:        Str(m, "name"),
		Sex:         Int(m, "sex"),
		Face:        Int(m, "face"),
		State:       Int(m, "state"),
		Level:       Int(m, "level"),
		HeroType:    Int(m, "hero_type"),
		CommandBase: Int(m, "command_base"),
		AffairsBase: Int(m, "affairs_base"),
		BraveryBase: Int(m, "bravery_base"),
		WisdomBase:  Int(m, "wisdom_base"),
		Loyalty:     Int(m, "loyalty"),
	}
}