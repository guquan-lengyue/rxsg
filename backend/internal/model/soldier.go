package model

// Soldier 对应 sys_city_soldier（utils.php:834）。
type Soldier struct {
	CID   int `json:"cid"`
	SID   int `json:"sid"`
	Count int `json:"count"`
}

func SoldierFromMap(m map[string]any) Soldier {
	return Soldier{CID: Int(m, "cid"), SID: Int(m, "sid"), Count: Int(m, "count")}
}