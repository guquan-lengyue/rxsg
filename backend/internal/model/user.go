package model

// User 对应新库 users 表；JSON 字段沿用前端既有契约。
type User struct {
	UID       int    `json:"uid"`
	Passport  string `json:"passport"`
	Passtype  string `json:"passtype"`
	Name      string `json:"name"`
	Group     int    `json:"group"`
	State     int    `json:"state"`
	Money     int64  `json:"money"`
	Honour    int    `json:"honour"`
	Nobility  string `json:"nobility"`
	LastCID   int    `json:"lastcid"`
	RegTime   int64  `json:"regtime"`
	OfficePos int    `json:"officepos"`
	UnionID   int    `json:"union_id"`
	Rank      int    `json:"rank"`
}

func UserFromMap(m map[string]any) User {
	return User{
		UID:      Int(m, "id"),
		Passport: Str(m, "passport"),
		Passtype: "my",
		Name:     Str(m, "nickname"),
		State:    Int(m, "state"),
		Money:    Int64(m, "money"),
		Honour:   Int(m, "honour"),
		Nobility: Str(m, "nobility"),
		RegTime:  Int64(m, "created_at"),
	}
}