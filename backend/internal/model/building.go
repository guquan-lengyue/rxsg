package model

// Building 对应新库 buildings 表（xy 为标签如 "a1"）；
// bname 由联表 cfg_buildings.name 提供。
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
	x, y := ParseXY(Str(m, "xy"))
	return Building{
		CID:            Int(m, "city_id"),
		BID:            Int(m, "building_id"),
		X:              x,
		Y:              y,
		Level:          Int(m, "level"),
		State:          Int(m, "state"),
		StateStartTime: Int64(m, "state_start_at"),
		StateEndTime:   Int64(m, "state_end_at"),
		Name:           Str(m, "bname"),
	}
}

// ParseXY 把建筑标签 "a1" 解析为 0-based 坐标：字母→列(x)，数字→行(y)。
// 非法输入返回 (0,0)。
func ParseXY(xy string) (int, int) {
	if len(xy) < 2 {
		return 0, 0
	}
	col := xy[0]
	row := xy[1]
	if col < 'a' || col > 'z' || row < '1' || row > '9' {
		return 0, 0
	}
	return int(col - 'a'), int(row - '1')
}