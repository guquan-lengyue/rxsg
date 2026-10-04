package model

// User 对应 sys_user（字段取自 server/game/Login.php 与 utils.php 的读写）。
type User struct {
	UID       int    `json:"uid"`
	Passport  string `json:"passport"`
	Passtype  string `json:"passtype"`
	Name      string `json:"name"`
	Group     int    `json:"group"`
	State     int    `json:"state"`
	Money     int64  `json:"money"`
	Honour    int    `json:"honour"`
	Nobility  int    `json:"nobility"`
	LastCID   int    `json:"lastcid"`
	RegTime   int64  `json:"regtime"`
	OfficePos int    `json:"officepos"`
	UnionID   int    `json:"union_id"`
	Rank      int    `json:"rank"`
}

func UserFromMap(m map[string]any) User {
	return User{
		UID:       Int(m, "uid"),
		Passport:  Str(m, "passport"),
		Passtype:  Str(m, "passtype"),
		Name:      Str(m, "name"),
		Group:     Int(m, "group"),
		State:     Int(m, "state"),
		Money:     Int64(m, "money"),
		Honour:    Int(m, "honour"),
		Nobility:  Int(m, "nobility"),
		LastCID:   Int(m, "lastcid"),
		RegTime:   Int64(m, "regtime"),
		OfficePos: Int(m, "officepos"),
		UnionID:   Int(m, "union_id"),
		Rank:      Int(m, "rank"),
	}
}