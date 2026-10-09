package battle

// helpers.go —— PHP 纯函数 helper 的 1:1 移植（UtilsExtend.php / OLdBattleCron.php / lang.php）。
// 口径来源（文件:行号）：
//   troop2array UtilsExtend.php:511 / defence2array :528 / check_resource :544 / add_resource :556 /
//   get_city_resource :585 / getArraySub :358 / array2troop :879 / sol2Value :1014 / array2count :1033 /
//   getCrray :1317 / getshootadd :1312 / choseHero :998 / check_city_ground_hero :612 / battleType :1293 /
//   getContentHero :1040 / getContentBattleHero :1058 /
//   getSoldierCounts OLdBattleCron.php:645 / getsoldierarray :656 / getrangbetween :669 / getjtsid :691 /
//   getabletarget :699 /
//   lang.php 模板与数据数组 L60-290、L2034-2131。
// 原版怪癖一律保留并注释；数值统一 float64 模拟 PHP 数值语义（字符串/整数自动转换）。

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"
)

// ── PHP 数值→字符串（拼接语义，precision=14）─────────────────────────────

// phpStr 模拟 PHP 字符串拼接中的 float→string（ini precision=14，即 %.14G）。
// 整数值输出不带小数点（与 PHP 一致）。
func phpStr(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		// PHP: NAN→"NAN"? 实际 nan 拼接为 "nan"（平台相关）；战斗数值不可达，兜底 "0"。
		return "0"
	}
	if v == math.Trunc(v) && math.Abs(v) < 1e15 {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'g', 14, 64)
}

// phpNum 模拟 PHP 字符串→数值（参与运算）；非法/空 → 0（对齐 null/” 语义）。
func phpNum(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		return v
	}
	return 0
}

// phpInt 模拟 (int) 截断。
func phpInt(v float64) int {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < math.MinInt32 {
		return math.MinInt32
	}
	return int(v) // Go int() 对 float64 截断向零，与 PHP (int) 一致
}

// ── PHP rand / mt_rand ──────────────────────────────────────────────────

// phpRand 模拟 PHP rand(a,b)（闭区间）。
func phpRand(a, b int) int {
	if a > b {
		a, b = b, a
	}
	return a + rand.Intn(b-a+1)
}

// phpMtRand 模拟 PHP mt_rand(a,b)（闭区间）。
func phpMtRand(a, b int) int { return phpRand(a, b) }

// phpRandFloat 模拟 PHP rand() 无参（0..getrandmax 2^31-1）——createmapSoldier 等未用，保留。
func phpRandFloat() float64 { return float64(rand.Int63n(1 << 31)) }

// ── 有序 int-keyed map（模拟 PHP 数值键数组：插入序 + ksort）──────────────

type oMap struct {
	keys []int
	m    map[int]float64
}

func newOMap() *oMap { return &oMap{m: map[int]float64{}} }

func (o *oMap) set(k int, v float64) {
	if _, ok := o.m[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.m[k] = v
}

// add 模拟 $arr[$k] = ($arr[$k] ?? 0) + v。
func (o *oMap) add(k int, v float64) { o.set(k, o.get(k)+v) }

func (o *oMap) get(k int) float64 { return o.m[k] }
func (o *oMap) has(k int) bool    { _, ok := o.m[k]; return ok }
func (o *oMap) len() int          { return len(o.m) }

// sortedKeys 模拟 PHP ksort 后的键序。
func (o *oMap) sortedKeys() []int {
	ks := make([]int, len(o.keys))
	copy(ks, o.keys)
	sort.Ints(ks)
	return ks
}

// ── 军队串 "N,sid,count,..." ────────────────────────────────────────────

func splitCSV(s string) []string { return strings.Split(s, ",") }

// at 模拟 PHP $arr[$i]（越界→null→空串）。
func at(p []string, i int) string {
	if i < 0 || i >= len(p) {
		return ""
	}
	return p[i]
}

// troop2Array 对齐 UtilsExtend.php:511 troop2array：cnt>0 才保留，键序=串序。
func troop2Array(s string) *oMap {
	o := newOMap()
	if s == "" || s == "0" {
		return o
	}
	p := splitCSV(s)
	n := phpInt(phpNum(at(p, 0)))
	for i := 0; i < n; i++ {
		sid := phpInt(phpNum(at(p, 2*i+1)))
		cnt := phpNum(at(p, 2*i+2))
		if cnt > 0 {
			o.set(sid, cnt)
		}
	}
	return o
}

// getSoldierCounts 对齐 OLdBattleCron.php:645：兵力总数（PHP 字符串累加=数值加法）。
func getSoldierCounts(s string) float64 {
	p := splitCSV(s)
	n := phpInt(phpNum(at(p, 0)))
	total := 0.0
	for i := 0; i < n; i++ {
		total += phpNum(at(p, 2*i+2))
	}
	return total
}

// array2troop 对齐 UtilsExtend.php:879。def=1 输出 sid,cnt,cnt,（城防三元组）。
// 原版怪癖保留：i==0 时返回字符串 "0"（$soldiers=$i）。
func array2troop(o *oMap, def bool) string {
	i := 0
	var b strings.Builder
	for _, sid := range o.keys {
		cnt := o.m[sid]
		if cnt > 0 {
			if def {
				b.WriteString(phpStr(float64(sid)) + "," + phpStr(cnt) + "," + phpStr(cnt) + ",")
			} else {
				b.WriteString(phpStr(float64(sid)) + "," + phpStr(cnt) + ",")
			}
			i++
		}
	}
	if i != 0 {
		return strconv.Itoa(i) + "," + b.String()
	}
	return "0"
}

// defence2Array 对齐 UtilsExtend.php:528：N,did,oldcnt,cnt 三元组；cnt<0→0。
type defEntry struct{ Cnt, OldCnt float64 }

func defence2Array(s string) ([]int, map[int]defEntry) {
	p := splitCSV(s)
	n := phpInt(phpNum(at(p, 0)))
	var order []int
	m := map[int]defEntry{}
	for i := 0; i < n; i++ {
		sid := phpInt(phpNum(at(p, 3*i+1)))
		oldcnt := phpNum(at(p, 3*i+2))
		cnt := phpNum(at(p, 3*i+3))
		if cnt < 0 {
			cnt = 0
		}
		if _, ok := m[sid]; !ok {
			order = append(order, sid)
		}
		m[sid] = defEntry{Cnt: cnt, OldCnt: oldcnt}
	}
	return order, m
}

// getArraySub 对齐 UtilsExtend.php:358：b 中每个键 a-b（b 独有→负值），结果 ksort。
func getArraySub(a, b *oMap) *oMap {
	r := newOMap()
	for _, k := range b.keys {
		if a.has(k) {
			r.set(k, a.get(k)-b.get(k))
		} else {
			r.set(k, -b.get(k))
		}
	}
	for _, k := range a.keys {
		if !b.has(k) {
			r.set(k, a.get(k))
		}
	}
	// ksort：重建键序
	r.keys = r.sortedKeys()
	return r
}

// sol2Value 对齐 UtilsExtend.php:1014（数组分支）：Σ floor(cnt*soldiervalue[sid]/0.784) 再 floor。
func sol2Value(o *oMap) float64 {
	exp := 0.0
	for _, sid := range o.keys {
		exp += math.Floor(o.m[sid] * soldierValueAt(sid) / 0.784)
	}
	return math.Floor(exp)
}

// array2count 对齐 UtilsExtend.php:1033。
func array2count(o *oMap) float64 {
	t := 0.0
	for _, k := range o.keys {
		t += o.m[k]
	}
	return t
}

// getCrray 对齐 UtilsExtend.php:1317：Σ cnt*carry[sid]*(1+tid/10)。
func getCrray(o *oMap, tid float64) float64 {
	c := 0.0
	for _, sid := range o.keys {
		c += (o.m[sid] * carryAt(sid)) * (1 + tid/10)
	}
	return c
}

// getshootadd 对齐 UtilsExtend.php:1312：1+0.05*科技14等级。
func (s *Service) getshootadd(ctx context.Context, cid int) (float64, error) {
	tec, err := s.techLevel(ctx, cid, 14)
	if err != nil {
		return 0, err
	}
	if tec == 0 {
		tec = 0 // empty→0（PHP 三元）
	}
	return 1 + 0.05*float64(tec), nil
}

// ── 资源串（check_resource / add_resource / get_city_resource）──────────

type res5 struct{ gold, food, wood, rock, iron float64 }

func (r res5) sum() float64 { return r.gold + r.food + r.wood + r.rock + r.iron }

// checkResource 对齐 UtilsExtend.php:544。
// 原版怪癖：'0' 与 '0,0,0,0,0,' 返回字符串本身（非数组）→ 后续 ['gold'] 非法偏移读为 null(0)。
// Go 以 isMap=false 表达，字段按 0 参与运算（与 PHP null 数值语义一致）。
func checkResource(s string) (res5, bool) {
	if s == "0" || s == "0,0,0,0,0," {
		return res5{}, false
	}
	p := splitCSV(s)
	return res5{
		gold: phpNum(at(p, 0)),
		food: phpNum(at(p, 1)),
		wood: phpNum(at(p, 2)),
		rock: phpNum(at(p, 3)),
		iron: phpNum(at(p, 4)),
	}, true
}

// addResource 对齐 UtilsExtend.php:556。返回恒为数组（键 '0' 合计串、'1' 掠夺串，均无尾逗号）。
// 原版怪癖保留：all1+all2>carry 时 x=(carry-all1)/all2 可能除零/超负重全带；carry==0 强制 x=0。
func addResource(r1, r2 string, carry float64) (sumStr, robStr string) {
	a, _ := checkResource(r1)
	b, _ := checkResource(r2)
	all1 := a.sum()
	all2 := b.sum()
	var x float64
	if all1+all2 > carry {
		x = (carry - all1) / all2 // all2==0 时 ±Inf/NaN，与 PHP 一致（后续 floor 处理见下）
	} else if all2 > 0 {
		x = 1
	} else {
		x = 0
	}
	if carry == 0 {
		x = 0
	}
	fl := func(v1, v2 float64) float64 {
		t := v1 + v2*x
		if math.IsNaN(t) {
			return 0 // PHP floor(NAN) 平台兜底
		}
		return math.Floor(t)
	}
	g := fl(a.gold, b.gold)
	f := fl(a.food, b.food)
	w := fl(a.wood, b.wood)
	rk := fl(a.rock, b.rock)
	ir := fl(a.iron, b.iron)
	sumStr = phpStr(g) + "," + phpStr(f) + "," + phpStr(w) + "," + phpStr(rk) + "," + phpStr(ir)
	robStr = phpStr(g-a.gold) + "," + phpStr(f-a.food) + "," + phpStr(w-a.wood) + "," +
		phpStr(rk-a.rock) + "," + phpStr(ir-a.iron)
	return
}

// getCityResource 对齐 UtilsExtend.php:585：city>0 带黄金，否则黄金=0；无尾逗号。
func (s *Service) getCityResource(ctx context.Context, cid int, city float64) (string, bool, error) {
	row, err := s.db.FetchOne(ctx,
		"select gold, food, wood, rock, iron from city_resources where city_id=? limit 1", cid)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	gold := 0.0
	if city > 0 {
		gold = float64(modelInt64(row, "gold"))
	}
	return phpStr(gold) + "," + phpStr(float64(modelInt64(row, "food"))) + "," +
		phpStr(float64(modelInt64(row, "wood"))) + "," + phpStr(float64(modelInt64(row, "rock"))) + "," +
		phpStr(float64(modelInt64(row, "iron"))), true, nil
}

// ── 战斗单位与几何 ──────────────────────────────────────────────────────

// Unit 对应 PHP fightsoldier 元素（$_SESSION['battle'][id]['fightsoldier'] 行）。
// 全 float64 模拟 PHP 松散类型；nil 指针 = PHP unset/null。
type Unit struct {
	SID    float64 `json:"sid"`
	Count  float64 `json:"count"`
	Range  float64 `json:"range"`
	Speed  float64 `json:"speed"`
	GF     float64 `json:"gongjifanwei"` // 攻击范围
	HP     float64 `json:"hp"`
	AP     float64 `json:"ap"`
	DP     float64 `json:"dp"`
	Stype  float64 `json:"stype"`
	Typ    float64 `json:"type"` // 1士兵 2城防 3城墙
	Did    float64 `json:"did"`
	Attack float64 `json:"attack"` // 1攻方 0守方
}

// getSoldierArray 对齐 OLdBattleCron.php:656 getsoldierarray：
// 兵力串 + 位置串（"N,sid,range,..."）→ 单位数组；range 取 pos[2i+2]。
func getSoldierArray(soldiers, posStr string) []*Unit {
	si := splitCSV(soldiers)
	pi := splitCSV(posStr)
	n := phpInt(phpNum(at(si, 0)))
	out := make([]*Unit, 0, n)
	for i := 0; i < n; i++ {
		m := 2 * i
		out = append(out, &Unit{
			SID:   phpNum(at(si, m+1)),
			Count: phpNum(at(si, m+2)),
			Range: phpNum(at(pi, m+2)),
		})
	}
	return out
}

// getRangBetween 对齐 OLdBattleCron.php:669：攻方最小 range、防方最大 range，按自身侧相减。
func getRangBetween(fs []*Unit, fieldrange float64, x int) float64 {
	attack := fieldrange
	resist := 0.0
	for _, v := range fs {
		if v == nil {
			continue
		}
		if v.Attack == 1 && v.Count > 0 && v.Range < attack {
			attack = v.Range
		}
		if v.Attack == 0 && v.Count > 0 && v.Range > resist {
			resist = v.Range
		}
	}
	if fs[x].Attack == 1 {
		return fs[x].Range - resist
	}
	return attack - fs[x].Range
}

// getJTsid 对齐 OLdBattleCron.php:691：首个 type==2 && did==3（箭塔）。
func getJTsid(fs []*Unit) (int, bool) {
	for i, v := range fs {
		if v != nil && v.Typ == 2 && v.Did == 3 {
			return i, true
		}
	}
	return 0, false
}

type idxKV struct {
	idx int
	val float64
}

// getAbleTarget 对齐 OLdBattleCron.php:699。
// 原版怪癖保留：不过滤 count<=0 单位；无匹配时返回空（PHP $rt 未定义）。
// cfgType 对应 PHP `select type from cfg_soldier where sid=...`（查无→null→0）。
func getAbleTarget(fs []*Unit, x int, cfgType func(sid float64) float64) []idxKV {
	var rt []idxKV
	attack := fs[x].Attack
	stype := fs[x].Stype
	var able float64
	if attack == 1 {
		able = fs[x].Range - fs[x].GF
	} else {
		able = fs[x].Range + fs[x].GF
	}
	for i, v := range fs {
		if v == nil {
			continue
		}
		if attack == 1 {
			if stype == 6 || stype == 10 || stype == 12 {
				if v.Attack == 0 && v.Range > able {
					switch {
					case v.Typ == 3:
						rt = append(rt, idxKV{i, v.SID})
					case v.Typ == 2 && v.Did == 3:
						rt = append(rt, idxKV{i, 15})
					case v.Typ == 1:
						rt = append(rt, idxKV{i, cfgType(v.SID)})
					}
				}
			} else {
				if v.Attack == 0 && v.Range > able && v.Typ != 2 {
					val := cfgType(v.SID)
					if v.Typ == 3 {
						val = v.SID
					}
					rt = append(rt, idxKV{i, val})
				}
			}
		} else {
			if v.Attack == 1 && v.Range < able {
				rt = append(rt, idxKV{i, cfgType(v.SID)})
			}
		}
	}
	return rt
}

// arraySearchLoose 模拟 PHP array_search($needle, $arr)（数值松散==；返回首个键）。
func arraySearchLoose(rt []idxKV, needle float64) (int, bool) {
	for _, kv := range rt {
		if kv.val == needle {
			return kv.idx, true
		}
	}
	return 0, false
}

// ── DB 小工具 ───────────────────────────────────────────────────────────

func modelInt64(row map[string]any, k string) int64 {
	switch v := row[k].(type) {
	case int64:
		return v
	case []byte:
		return phpInt64(string(v))
	case string:
		return phpInt64(v)
	case float64:
		return int64(v)
	case nil:
		return 0
	}
	return 0
}

func modelFloat(row map[string]any, k string) float64 {
	switch v := row[k].(type) {
	case int64:
		return float64(v)
	case []byte:
		return phpNum(string(v))
	case string:
		return phpNum(v)
	case float64:
		return v
	case nil:
		return 0
	}
	return 0
}

func phpInt64(s string) int64 {
	s = strings.TrimSpace(s)
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		return int64(v)
	}
	return 0
}

// techLevel 对齐 PHP `select level from sys_city_technic where cid=? and tid=?`（新库 city_technics）。
func (s *Service) techLevel(ctx context.Context, cid, tid int) (int, error) {
	v, err := s.db.FetchCellInt64(ctx, "select level from city_technics where city_id=? and technic_id=? limit 1", cid, tid)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return int(v), nil
}

// choseHero 对齐 UtilsExtend.php:998：state 7→1→8→随机→0。
func (s *Service) choseHero(ctx context.Context, cid int) (int, error) {
	for _, st := range []int{7, 1, 8} {
		v, err := s.db.FetchCellInt64(ctx, "select id from heroes where city_id=? and state=? limit 1", cid, st)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
		if v > 0 {
			return int(v), nil
		}
	}
	v, err := s.db.FetchCellInt64(ctx, "select id from heroes where city_id=? order by rand() limit 1", cid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if v > 0 {
		return int(v), nil
	}
	return 0, nil
}

// checkCityGroundHero 对齐 UtilsExtend.php:612：state in(0,7) 且为本城所属玩家，order by state desc。
// 新库适配：sys_city_hero→heroes（hid→id），城 owner 由 cities.user_id 提供。
func (s *Service) checkCityGroundHero(ctx context.Context, cid int) (hid int, state int, err error) {
	owner, err := s.db.FetchCellInt64(ctx, "select user_id from cities where id=? limit 1", cid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, 0, err
	}
	if owner == 0 {
		return 0, 0, nil
	}
	row, err := s.db.FetchOne(ctx,
		"select id, state from heroes where city_id=? and state in (0,7) and user_id=? order by state desc limit 1",
		cid, owner)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	return int(modelInt64(row, "id")), int(modelInt64(row, "state")), nil
}

// battleType 对齐 UtilsExtend.php:1293。
// 新库适配：field.type>0（城池）→0；task4→1；task7/8/9→4；否则 2。
func battleType(fieldType float64, task int) int {
	if fieldType > 0 {
		return 0
	}
	if task == 4 {
		return 1
	}
	if task == 7 || task == 8 || task == 9 {
		return 4
	}
	return 2
}

// getContentHero 对齐 UtilsExtend.php:1040（cfg_battle_hero 分支不实现→恒查 heroes）。
func (s *Service) getContentHero(ctx context.Context, hid int) (string, error) {
	if hid <= 0 {
		return tplEmpty, nil
	}
	row, err := s.db.FetchOne(ctx, "select name, level, face, sex from heroes where id=? limit 1", hid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sprintf(tplHeroName, "boy_0", "", "", "0"), nil
		}
		return "", err
	}
	sex := "girl"
	if modelInt64(row, "sex") != 0 {
		sex = "boy"
	}
	face := strconv.FormatInt(modelInt64(row, "face"), 10)
	name, _ := row["name"].(string)
	level := strconv.FormatInt(modelInt64(row, "level"), 10)
	return sprintf(tplHeroName, sex+"_"+face, name, name, level), nil
}

// getContentBattleHero 对齐 UtilsExtend.php:1058。
func (s *Service) getContentBattleHero(ctx context.Context, hid int) (string, error) {
	if hid <= 0 {
		return tplEmpty, nil
	}
	row, err := s.db.FetchOne(ctx, "select name, level, face, sex from heroes where id=? limit 1", hid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sprintf(tplHero, "boy_0", "", "", "0"), nil
		}
		return "", err
	}
	sex := "girl"
	if modelInt64(row, "sex") != 0 {
		sex = "boy"
	}
	face := strconv.FormatInt(modelInt64(row, "face"), 10)
	name, _ := row["name"].(string)
	level := strconv.FormatInt(modelInt64(row, "level"), 10)
	return sprintf(tplHero, sex+"_"+face, name, name, level), nil
}

// ── PHP sprintf('%s') 简化实现（战斗模板仅用 %s）────────────────────────

// sprintf 按序替换 %s（模板中的 %% 不出现）。
func sprintf(tpl string, args ...string) string {
	var b strings.Builder
	ai := 0
	for i := 0; i < len(tpl); i++ {
		if tpl[i] == '%' && i+1 < len(tpl) && tpl[i+1] == 's' {
			if ai < len(args) {
				b.WriteString(args[ai])
				ai++
			}
			i++
			continue
		}
		b.WriteByte(tpl[i])
	}
	return b.String()
}

// sprintfF 支持 %s 与 %.2f 等少量数值占位（模板中 %s 为主，数值参数经 phpStr 传入）。
// ── lang.php 数据数组 ───────────────────────────────────────────────────

// soldierValueAt 对齐 $GLOBALS['soldier']['soldiervalue']（lang.php:227-234）。
func soldierValueAt(sid int) float64 {
	if sid < 0 || sid >= len(soldierValueArr) {
		return 0
	}
	return soldierValueArr[sid]
}

func usePeopleAt(sid int) float64 {
	if sid < 0 || sid >= len(usePeopleArr) {
		return 0
	}
	return usePeopleArr[sid]
}

func carryAt(sid int) float64 {
	if sid < 0 || sid >= len(carryArr) {
		return 0
	}
	return carryArr[sid]
}

func foodUseAt(sid int) float64 {
	if sid < 0 || sid >= len(foodUseArr) {
		return 0
	}
	return foodUseArr[sid]
}

// lang.php:226 $GLOBALS['soldier']['usepeople']
var usePeopleArr = []float64{0, 1, 1, 1, 1, 1, 2, 4, 3, 6, 5, 10, 8, 1, 1, 1, 2, 3, 1, 1, 1, 2, 3, 1, 1, 1, 2, 3, 1, 1, 1, 2, 3, 1, 1, 1, 1, 1, 2, 3, 6, 4, 5, 20, 8, 8, 10, 2, 6, 4, 2, 1, 1, 1, 2, 3, 6, 5, 10, 5, 1, 1, 1, 2, 3, 6, 5, 10, 5, 1, 1, 1, 2, 3, 6, 5, 10, 5, 1, 1, 1, 2, 3, 6, 5, 10, 5, 1, 1, 1, 2, 3, 6, 5, 10, 5}

// lang.php:227-234 $GLOBALS['soldier']['soldiervalue']
var soldierValueArr = []float64{0, 23, 31, 70, 90, 135, 140, 298, 285, 875, 1000, 1375, 2900, 31, 90, 135, 140, 285, 26, 89, 127, 128, 263, 31, 90, 135, 140, 285, 31, 90, 135, 140, 285,
	23, 31, 70, 90, 135, 140, 298, 285, 875, 1000, 1375, 2900,
	285, 2900, 90, 135, 140, 298,
	70, 90, 135, 140, 298, 285, 1000, 1375, 2900,
	70, 90, 135, 140, 298, 285, 1000, 1375, 2900,
	70, 90, 135, 140, 298, 285, 1000, 1375, 2900,
	70, 90, 135, 140, 298, 285, 1000, 1375, 2900,
	70, 90, 135, 140, 298, 285, 1000, 1375, 2900}

// lang.php:235 $GLOBALS['soldier']['carry']
var carryArr = []float64{0, 200, 20, 5, 40, 30, 25, 100, 80, 5000, 35, 45, 75, 20, 40, 30, 25, 100, 35, 50, 40, 30, 150, 20, 40, 30, 25, 100, 20, 40, 30, 25, 100, 200, 20, 5, 40, 30, 25, 100, 80, 5000, 35, 45, 75, 100, 60, 50, 80, 45, 35, 15, 120, 90, 75, 300, 240, 105, 135, 225, 10, 200, 155, 125, 500, 400, 175, 225, 375, 37, 300, 225, 187, 750, 600, 262, 337, 562, 45, 450, 315, 405, 720, 900, 315, 540, 675, 75, 750, 525, 675, 1200, 1500, 525, 900, 1125}

// lang.php:236 $GLOBALS['soldier']['fooduse']
var foodUseArr = []float64{0, 2, 3, 5, 6, 7, 9, 18, 35, 10, 50, 100, 250, 3, 6, 8, 9, 20, 2, 5, 6, 8, 15, 3, 6, 8, 9, 20, 3, 6, 8, 9, 20, 2, 3, 5, 6, 7, 9, 18, 35, 10, 50, 100, 250, 50, 100, 250, 5, 6, 7, 9, 18, 35, 50, 100, 250, 5, 6, 7, 9, 18, 35, 50, 100, 250, 5, 6, 7, 9, 18, 35, 100, 250}

// sidTypeLang 对齐 lang.php:267 $GLOBALS['sid']['type']（battleAdd 的 sidtype 用此映射，非 cfg_soldier.type——原版怪癖）。
var sidTypeLang = map[int]int{
	1: 1, 2: 2, 3: 3, 4: 4, 5: 5, 6: 6, 7: 7, 8: 8, 9: 9, 10: 10, 11: 11, 12: 12,
	13: 2, 14: 4, 15: 5, 16: 6, 17: 7, 18: 2, 19: 4, 20: 5, 21: 6, 22: 7,
	45: 45, 46: 46, 47: 47, 48: 48, 49: 49, 50: 50,
}

// battlePosLang 对齐 lang.php:268 $GLOBALS['battle']['pos']（battleAdd 阵位串）。
var battlePosLang = []string{"", "100", "50", "0"}

// patrolReportSoldier 对齐 lang.php:2035-2131 $GLOBALS['battle']['patrol_report_soldier']。
var patrolReportSoldier = map[int]string{
	1: "民夫", 2: "义兵", 3: "斥候", 4: "长枪兵", 5: "刀盾兵", 6: "弓箭兵", 7: "轻骑兵", 8: "铁骑兵",
	9: "辎重车", 10: "床弩", 11: "冲车", 12: "投石车", 13: "流民", 14: "匪兵", 15: "强盗", 16: "山贼",
	17: "马贼", 18: "黄巾众", 19: "黄巾军", 20: "黄巾精兵", 21: "黄巾弓手", 22: "黄巾头目", 23: "十常侍众",
	24: "十常侍军", 25: "十常侍精兵", 26: "十常侍弓手", 27: "十常侍头目", 28: "董卓众", 29: "董卓军",
	30: "董卓精兵", 31: "董卓弓手", 32: "董卓爪牙", 33: "宿卫兵", 34: "禁卫军", 35: "斥侯骑", 36: "虎贲军",
	37: "羽林军", 38: "强弩兵", 39: "武卫骑", 40: "宿武卫骑", 41: "运输车", 42: "连弩车", 43: "破城车",
	44: "霹雳车", 45: "西凉铁骑", 46: "南蛮象兵", 47: "青州兵", 48: "虎豹骑", 49: "突骑兵", 50: "藤甲兵",
	51: "羌族哨兵", 52: "羌族勇士", 53: "羌族猛士", 54: "羌族弓弩手", 55: "羌族骑士", 56: "羌族小头目",
	57: "羌族强弩手", 58: "羌族攻城车", 59: "羌族抛石车", 60: "南蛮野人", 61: "南蛮勇士", 62: "南蛮刀斧手",
	63: "南蛮弓箭手", 64: "南蛮骑兵", 65: "南蛮铁骑兵", 66: "南蛮连弩手", 67: "南蛮冲撞车", 68: "南蛮投石车",
	69: "山越密探", 70: "山越枪兵", 71: "山越刀盾兵", 72: "山越神射手", 73: "山越骑兵", 74: "山越小头目",
	75: "山越弩车", 76: "山越冲车", 77: "山越投石车", 78: "乌丸探子", 79: "乌丸枪兵", 80: "乌丸刀兵",
	81: "乌丸弩手", 82: "乌丸轻骑", 83: "乌丸骑兵", 84: "乌丸连弩车", 85: "乌丸冲车", 86: "乌丸投石车",
	87: "匈奴斥候", 88: "匈奴枪兵", 89: "匈奴精兵", 90: "匈奴射手", 91: "匈奴骑兵", 92: "匈奴重骑兵",
	93: "匈奴弩车", 94: "匈奴冲车", 95: "匈奴投石车",
}

// patrolReportDefence 对齐 lang.php:275 $GLOBALS['battle']['patrol_report_defence']。
var patrolReportDefence = []string{"", "陷阱", "拒马", "箭塔", "滚木", "擂石"}

// reportTypeLang 对齐 lang.php:264 $GLOBALS['report']['type']。
var reportTypeLang = []string{"运输", "派遣", "侦查", "掠夺", "占领", "防守", "护家", "战场出征", "战场派遣", "战场攻击"}

// title2Row 对齐 lang.php:239 $GLOBALS['report']['title_2row']。
var title2Row = []string{
	"对方出城迎战。<br/>", "目标发现山贼，发生战斗。<br/>", "发生战斗。<br/>", "对方未发觉。<br/>",
	"派遣完成。<br/>", "运输完成。<br/>", "派遣失败。<br/>", "运输失败。<br/>",
	"斥候被对方发现，发生战斗。<br/>", "我方未迎战。", "我方迎战。",
}

// jueweiLang 对齐 lang.php:277 $GLOBALS['battle']['juewei']。
var jueweiLang = []string{"平民", "公士", "上造", "簪袅", "不更", "大夫", "官大夫", "公大夫", "公乘", "五大夫",
	"左庶长", "右庶长", "左更", "中更", "右更", "少上造", "大上造", "驷车庶长", "大庶长", "关内侯", "列侯", "王"}

// citytypeLang 对齐 lang.php:279 $GLOBALS['battle']['citytype']。
var citytypeLang = []string{"城池", "县城", "郡城", "州城", "都城"}

// invadeResultLang 对齐 lang.php:282 $GLOBALS['battle']['invade_result']。
var invadeResultLang = []string{"<font color=\"red\">未达成</font>", "<font color=\"#00DB00\">达成</font>",
	"&nbsp;条件不足，无法占领城池。", "&nbsp;条件达成，成功占领城池。"}

// battleResultLang 对齐 lang.php:274 $GLOBALS['battle']['result']。
var battleResultLang = []string{"胜利", "失败", "平局"}

// 战报模板常量见 lang.go（由 lang.php 逐字节提取，勿手改）。
