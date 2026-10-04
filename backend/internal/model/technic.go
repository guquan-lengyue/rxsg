package model

// Technic 对应新库 city_technics 表。
type Technic struct {
	TID   int `json:"tid"`
	Level int `json:"level"`
}

func TechnicFromMap(m map[string]any) Technic {
	return Technic{TID: Int(m, "technic_id"), Level: Int(m, "level")}
}

// Province 新库无世界地图表，恒为空。
type Province struct {
	Province string `json:"province"`
	Jun      string `json:"jun"`
}