package economy

// workshop.go 1:1 复刻 server/game/WorkShop.php（工匠作坊宝珠货架）。
// 表映射：sys_user_workshop→user_workshops、bid15（legacy 工匠作坊）→新 bid14。
// 原版怪癖保留：
//   - refreshWorkShop 中 now-time<0（time 在未来）时强制收费 20 五铢钱。
//   - buyWorkShopGood 礼金判断为严格大于（gift==price 也报"您的礼金不够！"）。
//   - 购买后重建 gidStr 时，被购项若是末对，则前一对的尾逗号残留。

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	"rxsg/backend/internal/game"
)

// checkCityExist 对齐 utils.php:215。
func (s *Service) checkCityExist(ctx context.Context, uid, cid int) error {
	ok, err := s.db.Exists(ctx, "select user_id from cities where id=? and user_id=? limit 1", cid, uid)
	if err != nil {
		return err
	}
	if !ok {
		return errLegacy("没有该城池信息。") // checkCityExist.no_city_info
	}
	return nil
}

// workshopRow 取（无则建）user_workshops 行。
func (s *Service) workshopRow(ctx context.Context, uid int) (map[string]any, error) {
	row, err := s.db.FetchOne(ctx, "select * from user_workshops where user_id=?", uid)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := s.db.Exec(ctx, "insert into user_workshops (user_id) values (?)", uid); err != nil {
			return nil, err
		}
		return s.db.FetchOne(ctx, "select * from user_workshops where user_id=?", uid)
	}
	return row, err
}

// loadInitWorkShopInfo 对齐 WorkShop.php:5。
// 返回 [leavTime] 或 [leavTime, goodCnt, (cfg_goods行, price)×n]。
func (s *Service) loadInitWorkShopInfo(ctx context.Context, uid, curCid int) ([]any, error) {
	if err := s.checkCityExist(ctx, uid, curCid); err != nil {
		return nil, err
	}
	row, err := s.workshopRow(ctx, uid)
	if err != nil {
		return nil, err
	}
	ret := []any{}
	lastTime := modelInt64(row, "time")
	leavTime := int64(0)
	if lastTime > 0 {
		endTime := lastTime + game.WorkshopCooldown
		now, err := s.db.Now(ctx)
		if err != nil {
			return nil, err
		}
		if endTime-now > 0 {
			leavTime = endTime - now
		}
	}
	ret = append(ret, leavTime)
	gidStr := ""
	if v, ok := row["gidstr"]; ok && v != nil {
		if b, isB := v.([]byte); isB {
			gidStr = string(b)
		} else if st, isS := v.(string); isS {
			gidStr = st
		}
	}
	if gidStr == "" {
		return ret, nil
	}
	goodArr := strings.Split(gidStr, ",")
	goodCnt := goodArr[0]
	ret = append(ret, goodCnt)
	for i := 1; i < len(goodArr); i += 2 {
		gid := goodArr[i]
		price := ""
		if i+1 < len(goodArr) {
			price = goodArr[i+1]
		}
		good, err := s.db.FetchOne(ctx, "select * from cfg_goods where gid=?", gid)
		if errors.Is(err, sql.ErrNoRows) {
			good = map[string]any{}
		} else if err != nil {
			return nil, err
		}
		ret = append(ret, good, price)
	}
	return ret, nil
}

// LoadInitWorkShopInfo 对外入口。
func (s *Service) LoadInitWorkShopInfo(ctx context.Context, uid, curCid int) ([]any, error) {
	return s.loadInitWorkShopInfo(ctx, uid, curCid)
}

// getKey 对齐 WorkShop.php:155（累加 >= compareValue → key，按插入序）。
func getKey(compareValue int, order []int, vals map[int]int) int {
	sum := 0
	index := 0
	for _, key := range order {
		sum += vals[key]
		if sum >= compareValue {
			index = key
			break
		}
	}
	return index
}

var (
	wsFreeRateOrder = []int{1, 2, 3, 4}
	wsFreeRate      = map[int]int{1: 45, 2: 30, 3: 20, 4: 5}
	wsWuzhuRate     = map[int]int{1: 40, 2: 30, 3: 24, 4: 5, 5: 1}
	wsLevelToCount  = map[int]int{1: 1, 2: 2, 3: 3, 4: 4, 5: 5, 6: 7, 7: 9, 8: 11, 9: 13, 10: 14}
	wsGoodTypeRate  = map[int]int{0: 5, 1: 15, 2: 5, 3: 15, 4: 25, 6: 10, 7: 25}
	wsLevelToPrice  = map[int]int{1: 10, 2: 30, 3: 60, 4: 90, 5: 120}
)

// wsGoodTypeOrder 保持 goodTypeRateArr 的插入序（0,1,2,3,4,6,7）。
var wsGoodTypeOrder = []int{0, 1, 2, 3, 4, 6, 7}

// RefreshWorkShop 对齐 WorkShop.php:46。
func (s *Service) RefreshWorkShop(ctx context.Context, uid, curCid, isNeedWuzhu int) ([]any, error) {
	if err := s.checkCityExist(ctx, uid, curCid); err != nil {
		return nil, err
	}
	if isNeedWuzhu != 0 && isNeedWuzhu != 1 {
		return nil, errLegacy("数据异常！")
	}
	row, err := s.workshopRow(ctx, uid)
	if err != nil {
		return nil, err
	}
	if modelInt(row, "count") >= game.WorkshopRefreshLimit {
		return nil, errLegacy("今天立即刷新次数达到上限，请明天再来")
	}
	needWuzhuCount := 0
	if isNeedWuzhu == 1 {
		needWuzhuCount = 20
	}
	now, err := s.db.Now(ctx)
	if err != nil {
		return nil, err
	}
	if now-modelInt64(row, "time") < 0 { // 原版怪癖：time 在未来 → 强制收费
		needWuzhuCount = 20
	}
	if needWuzhuCount > 0 {
		wuzhu, err := s.goodsCount(ctx, uid, game.GidWuzhuqian)
		if err != nil {
			return nil, err
		}
		if wuzhu < int64(needWuzhuCount) {
			return nil, errLegacy("您的五铢钱不够！")
		}
	}
	workShopLevel, err := s.cellInt(ctx, "select level from buildings where city_id=? and building_id=? limit 1", curCid, game.BidWorkshop)
	if err != nil {
		return nil, err
	}
	if workShopLevel == 0 { // legacy empty()：无行或 0 级
		return nil, errLegacy("当前城池没有工匠作坊！！！")
	}
	if needWuzhuCount > 0 {
		if err := s.addGoods(ctx, uid, game.GidWuzhuqian, -int64(needWuzhuCount), 1212); err != nil {
			return nil, err
		}
	}
	needRefreshCount := wsLevelToCount[int(workShopLevel)]
	resultStr := fmt.Sprintf("%d,", needRefreshCount)
	for i := 0; i < needRefreshCount; i++ {
		levelRate := rand.Intn(100) + 1 // mt_rand(1,100)
		var resultLevel int
		if isNeedWuzhu == 1 {
			resultLevel = getKey(levelRate, []int{1, 2, 3, 4, 5}, wsWuzhuRate)
		} else {
			resultLevel = getKey(levelRate, wsFreeRateOrder, wsFreeRate)
		}
		typeRate := rand.Intn(100) + 1
		attid := getKey(typeRate, wsGoodTypeOrder, wsGoodTypeRate)
		newGid := 300 + 10*attid + (resultLevel - 1)
		newPrice := wsLevelToPrice[resultLevel]
		if i == needRefreshCount-1 {
			resultStr += fmt.Sprintf("%d,%d", newGid, newPrice)
		} else {
			resultStr += fmt.Sprintf("%d,%d,", newGid, newPrice)
		}
	}
	newcount := modelInt(row, "count") + 1
	if _, err := s.db.Exec(ctx,
		"update user_workshops set `count`=?, gidstr=?, time=? where user_id=?",
		newcount, resultStr, now, uid); err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(ctx,
		"insert into log_workshop_freshes (user_id, gidstr, cost, time) values (?,?,?,unix_timestamp())",
		uid, resultStr, needWuzhuCount); err != nil {
		return nil, err
	}
	return s.loadInitWorkShopInfo(ctx, uid, curCid)
}

// BuyWorkShopGood 对齐 WorkShop.php:171。返回 ["购买成功！", loadInit]。
func (s *Service) BuyWorkShopGood(ctx context.Context, uid, curCid, gid int) ([]any, error) {
	if err := s.checkCityExist(ctx, uid, curCid); err != nil {
		return nil, err
	}
	gidStr, err := s.db.FetchCellString(ctx, "select gidstr from user_workshops where user_id=?", uid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if gidStr == "" {
		return nil, errLegacy("您的工匠作坊没有数据！")
	}
	gidArr := strings.Split(gidStr, ",")
	gCount, _ := strconv.Atoi(gidArr[0])
	gidArr = gidArr[1:]
	resultPrice := 0
	resultGidStr := ""
	reclyCount := gCount * 2
	for i := 0; i < reclyCount && i < len(gidArr); i += 2 {
		curGid, _ := strconv.Atoi(gidArr[i])
		price := 0
		if i+1 < len(gidArr) {
			price, _ = strconv.Atoi(gidArr[i+1])
		}
		if curGid == gid {
			resultPrice = price
			continue
		}
		if i < reclyCount-1 {
			resultGidStr += fmt.Sprintf("%d,%d,", curGid, price)
		} else {
			resultGidStr += fmt.Sprintf("%d,%d", curGid, price)
		}
	}
	if resultPrice > 0 {
		gift, err := s.cellInt(ctx, "select gift from users where id=? limit 1", uid)
		if err != nil {
			return nil, err
		}
		if gift <= int64(resultPrice) { // 严格大于（原版 sql_check gift>$resultPrice）
			return nil, errLegacy("您的礼金不够！")
		}
		if err := s.addGoods(ctx, uid, gid, 1, 1212); err != nil {
			return nil, err
		}
		if err := s.addGoods(ctx, uid, 0, -int64(resultPrice), 1212); err != nil {
			return nil, err
		}
		resultGidStr = fmt.Sprintf("%d,", gCount-1) + resultGidStr
		if _, err := s.db.Exec(ctx, "update user_workshops set gidstr=? where user_id=?", resultGidStr, uid); err != nil {
			return nil, err
		}
	} else {
		return nil, errLegacy("数据异常！")
	}
	init, err := s.loadInitWorkShopInfo(ctx, uid, curCid)
	if err != nil {
		return nil, err
	}
	return []any{"购买成功！", init}, nil
}
