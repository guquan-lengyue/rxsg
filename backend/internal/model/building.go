package model

// Building 对应 getCityBuildingInfo 的联表结果（utils.php:785）。
// 别名映射：bname / buildingDescription / level_description / using_people。
type Building struct {
	CID              int    `json:"cid"`
	BID              int    `json:"bid"`
	X                int    `json:"x"`
	Y                int    `json:"y"`
	Level            int    `json:"level"`
	State            int    `json:"state"`
	StateStartTime   int64  `json:"state_starttime"`
	StateEndTime     int64  `json:"state_endtime"`
	StateTimeLeft    int64  `json:"state_timeleft"`
	Name             string `json:"bname"`
	Description      string `json:"buildingDescription"`
	LevelDescription string `json:"level_description"`
	UsingPeople      int    `json:"using_people"`
}

func BuildingFromMap(m map[string]any) Building {
	return Building{
		CID:              Int(m, "cid"),
		BID:              Int(m, "bid"),
		X:                Int(m, "x"),
		Y:                Int(m, "y"),
		Level:            Int(m, "level"),
		State:            Int(m, "state"),
		StateStartTime:   Int64(m, "state_starttime"),
		StateEndTime:     Int64(m, "state_endtime"),
		StateTimeLeft:    Int64(m, "state_timeleft"),
		Name:             Str(m, "bname"),
		Description:      Str(m, "buildingDescription"),
		LevelDescription: Str(m, "level_description"),
		UsingPeople:      Int(m, "using_people"),
	}
}