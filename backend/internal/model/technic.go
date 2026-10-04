package model

// Technic 对应 sys_city_technic 的 {tid,level}（utils.php:859）。
type Technic struct {
	TID   int `json:"tid"`
	Level int `json:"level"`
}

func TechnicFromMap(m map[string]any) Technic {
	return Technic{TID: Int(m, "tid"), Level: Int(m, "level")}
}

// Province 对应 mem_world 的 {province,jun}（utils.php:861）。
type Province struct {
	Province string `json:"province"`
	Jun      string `json:"jun"`
}

func ProvinceFromMap(m map[string]any) Province {
	return Province{Province: Str(m, "province"), Jun: Str(m, "jun")}
}