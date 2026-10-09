package economy

// service.go M6 经济批次：市场（MarketFunc.php）/仓库（StoreFunc.php）/
// 工匠作坊（WorkShop.php）/商城（ShopFunc.php）/宝物出售（GoodsFunc.sellGoods）/
// 产出结算（utils.SetCityBaseProduce+UpdateUsersCityResource）/
// 交易与自动运输惰性结算（ReportCron.HandleTrade+HandleAutoTrans）。
// 1:1 复刻原版逻辑与文案，原版 bug/怪癖保留并在各处注释。
// 通用降级（新库缺表/外部服务，按任务约定）：
//   - mem_world/troops 野地 → refreshFoodArmyUsers 仅计城内驻军口粮。
//   - sys_user_book/cfg_book → 将领技能 skill_add 恒 [0]；mem_hero_buffer → hufu 恒 1、无文曲星。
//   - sys_union → 交易列表 unionname 恒空串（M8 社交不实现）。
//   - completeTask/logUserAction 成就任务（M8）：logUserAction 落库，completeTask 省略。
//   - checkAndDoShopAct/addChibiGoods/addMenghuoGood（M9/赤壁）→ 恒 false/普通道具路径。
//   - mem_city_trade 镜像（cron 缓存）合并省略。

import (
	"context"
	"database/sql"
	"errors"
	"strconv"

	"rxsg/backend/internal/building"
	"rxsg/backend/internal/db"
	"rxsg/backend/internal/httpx"
	"rxsg/backend/internal/lock"
)

type Service struct {
	db  *db.DB
	lk  *lock.Locker
	bld *building.Service
}

func NewService(d *db.DB, bld *building.Service) *Service {
	return &Service{db: d, lk: lock.New(d.DB), bld: bld}
}

// WithUserLock 包裹写操作（对齐 legacy lockUser/unlockUser 区间；
// 注意 legacy lockUser 恒返回 true——文件锁失效的原版行为，Go 用 GET_LOCK 保证数据一致性）。
func (s *Service) WithUserLock(ctx context.Context, uid int, key string, fn func(context.Context) error) error {
	err := s.lk.WithUserLock(ctx, uid, key, fn)
	if err != nil {
		if err.Error() == "lock_busy" {
			return errLegacy("服务器忙，请稍后再进行操作。") // pacifyPeople.server_busy
		}
		return err
	}
	return nil
}

// errLegacy 构造与 legacy throw new Exception 等价的错误响应。
// 原版"购买成功/出售成功/修改比例成功"等成功提示同样经由 throw 返回，1:1 保留。
func errLegacy(msg string) error {
	return httpx.BadRequest("economy_error", msg)
}

// cellFloat 读取单列数值并按浮点解析（对齐 legacy sql_fetch_one_cell 数值化语义）。
func (s *Service) cellFloat(ctx context.Context, query string, args ...any) (float64, error) {
	v, err := s.db.FetchCellString(ctx, query, args...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, nil
	}
	return f, nil
}

// cellInt 读取单列整数值（无行返回 0，对齐 legacy empty→0）。
func (s *Service) cellInt(ctx context.Context, query string, args ...any) (int64, error) {
	v, err := s.db.FetchCellInt64(ctx, query, args...)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return v, err
}

// cityNamePosition 对齐 utils.php:223 getCityNamePosition：name+"("+cid%1000+","+floor(cid/1000)+")"。
func (s *Service) cityNamePosition(ctx context.Context, cid int) (string, error) {
	name, err := s.db.FetchCellString(ctx, "select name from cities where id=?", cid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	return name + "(" + strconv.Itoa(cid%1000) + "," + strconv.Itoa(cid/1000) + ")", nil
}
