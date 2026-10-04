package model

// Soldier 对应新库 city_soldiers 表。
type Soldier struct {
	CID   int `json:"cid"`
	SID   int `json:"sid"`
	Count int `json:"count"`
}

func SoldierFromMap(m map[string]any) Soldier {
	return Soldier{CID: Int(m, "city_id"), SID: Int(m, "soldier_id"), Count: Int(m, "count")}
}