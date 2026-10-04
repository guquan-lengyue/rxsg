package model

// Defence 对应 sys_city_defence（utils.php:836）。
type Defence struct {
	CID   int `json:"cid"`
	DID   int `json:"did"`
	Count int `json:"count"`
}

func DefenceFromMap(m map[string]any) Defence {
	return Defence{CID: Int(m, "cid"), DID: Int(m, "did"), Count: Int(m, "count")}
}