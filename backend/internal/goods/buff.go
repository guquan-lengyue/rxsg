package goods

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"rxsg/backend/internal/model"
)

// makeTimeLeft 对齐 utils.php:63 MakeTimeLeft（与 city 包同实现）。
func makeTimeLeft(seconds int64) string {
	h := seconds / 3600
	m := (seconds - h*3600) / 60
	s := seconds % 60
	switch {
	case h > 0:
		return fmt.Sprintf("%d小时%d分钟%d秒", h, m, s)
	case m > 0:
		return fmt.Sprintf("%d分钟%d秒", m, s)
	default:
		return fmt.Sprintf("%d秒", s)
	}
}

// buff.go 复刻 GoodsFunc.php 中"有效期提示框"类道具（ret[]=2 分支）：
//   useShenNongChu(1782)/useLuBanFu(1806)/useKaiShanCui(1830)/useXuanTieLu(1855)
//   useShuiLiBian(2049)/useYaoYiLinAll(2258)/useMuBingLing(2282)/useQiuXianZhao(2293)
//   useJunLingZhuang(2304)/useShangDuiQiYue(2234)/useQingCangLing(2039)/useKaoGongJi(2558)
//   useXunChaLin(2377)/useXianZhenZhaoGu(1879)/useBaGuaZhenTu(1938)/useQingNangShu(1972)
//   UseMianZhanPai(1686)
// 数值与 mem_user_buffer buftype 逐一对齐；completeTaskWithTaskid/logUserAction/成就属 M8，未接线。

// prodToolDef 四产具 + 税吏鞭的公共形态：buftype、res_add 列、额外资源量。
type prodToolDef struct {
	gid       int
	bufType   int
	goodsCol  string // city_res_add 的 goods_*_add 列
	delay     int64  // 单份时长
	advDelay  int64  // 高级版单份时长（86400*7）
	basicGid  int    // 基础版 gid（额外 500）
	advGid    int    // 高级版 gid（额外 5000）
	extraFn   func(cid int64, extra int64) error
	validText string
}

// useProdTool 对齐 useShenNongChu 等四函数同构逻辑：
//
//	delay=基础86400/高级86400*7，×useCount；数量不足报"当前物品不足"；
//	全服城 goods_*_add=25 + resource_changing=1；buftype 叠加 endtime；
//	reduceGoods(useCount)；额外资源 500/5000 进 lastcid 城。
func (s *Service) useProdTool(ctx context.Context, uid int, d prodToolDef, gid, useCount int) (int64, string, error) {
	delay := d.delay
	if gid == d.advGid {
		delay = d.advDelay
	}
	delay = delay * int64(useCount)

	goodCnt, err := s.goodsCount(ctx, uid, gid)
	if err != nil {
		return 0, "", err
	}
	if goodCnt < int64(useCount) {
		return 0, "", errLegacy("当前物品不足")
	}

	if err := s.setGoodsAdd(ctx, uid, d.goodsCol); err != nil {
		return 0, "", err
	}
	endtime, err := s.addUserBuffer(ctx, uid, d.bufType, delay, delay)
	if err != nil {
		return 0, "", err
	}
	if err := s.ReduceGoods(ctx, uid, gid, int64(useCount), 0); err != nil {
		return 0, "", err
	}
	cid, err := s.lastCityID(ctx, uid)
	if err != nil {
		return 0, "", err
	}
	extra := int64(500)
	if gid == d.advGid {
		extra = 5000
	}
	if err := d.extraFn(int64(cid), extra); err != nil {
		return 0, "", err
	}
	return endtime, d.validText, nil
}

// useShuiLiBian 对齐 2049：checkGoods 失败报 "not_enough_goods$gid"；
// 全服城 gold_rate=125（legacy 写 mem_city_resource，Go 库为 city_resources）；buftype=15。
func (s *Service) useShuiLiBian(ctx context.Context, uid, gid int) (int64, string, error) {
	delay := int64(86400)
	if gid == 55 {
		delay = 86400 * 7
	}
	ok, err := s.checkGoods(ctx, uid, gid)
	if err != nil {
		return 0, "", err
	}
	if !ok {
		return 0, "", errLegacy(fmt.Sprintf("not_enough_goods%d", gid))
	}
	if _, err := s.db.Exec(ctx,
		"update city_resources m join cities c on m.city_id=c.id set m.gold_rate=125 where c.user_id=?", uid); err != nil {
		return 0, "", err
	}
	endtime, err := s.addUserBuffer(ctx, uid, 15, delay, delay)
	if err != nil {
		return 0, "", err
	}
	if err := s.ReduceGoods(ctx, uid, gid, 1, 0); err != nil {
		return 0, "", err
	}
	cid, err := s.lastCityID(ctx, uid)
	if err != nil {
		return 0, "", err
	}
	gold := int64(500)
	if gid == 55 {
		gold = 5000
	}
	if err := s.addCityResources(ctx, cid, 0, 0, 0, 0, gold); err != nil {
		return 0, "", err
	}
	return endtime, "“税吏鞭”有效时间截止到", nil
}

// useYaoYiLinAll 对齐 2258：insert 用 259200×useCount、on-duplicate 固定 +259200（原版 bug，1:1 保留）。
func (s *Service) useYaoYiLinAll(ctx context.Context, uid, gid, useCount int) (int64, string, error) {
	bufType := 11
	text := "“徭役令”有效期截止到"
	if gid == 166 {
		bufType = 166
		text = "“高级徭役令”有效期截止到"
	}
	goodCnt, err := s.goodsCount(ctx, uid, gid)
	if err != nil {
		return 0, "", err
	}
	if goodCnt < int64(useCount) {
		return 0, "", errLegacy("当前物品不足")
	}
	endtime, err := s.addUserBuffer(ctx, uid, bufType, int64(259200*useCount), 259200)
	if err != nil {
		return 0, "", err
	}
	if err := s.ReduceGoods(ctx, uid, gid, int64(useCount), 0); err != nil {
		return 0, "", err
	}
	return endtime, text, nil
}

// useSimpleBuffer 对齐"checkGoods→buftype 叠加→reduceGoods 1"的简单 buff 道具：
//
//	useMuBingLing(10321,+86400)/useQiuXianZhao(22,+604800)/useJunLingZhuang(19,+86400)
//	useShangDuiQiYue(17,+259200)/useQingCangLing(10,+604800)/useKaoGongJi(12/13/14,+86400)
//	useXunChaLin(100,+259200×3)/useZhaoAnLing(10333,+86400×useCount, update 固定 +86400 原版 bug)
//
// notEnoughMsg 为 checkGoods 失败时的报错（"not_enough_goods$gid" 或 no_this_good）。
func (s *Service) useSimpleBuffer(ctx context.Context, uid, gid, bufType int, delay int64, updateDelay int64, notEnoughMsg, text string) (int64, string, error) {
	ok, err := s.checkGoods(ctx, uid, gid)
	if err != nil {
		return 0, "", err
	}
	if !ok {
		return 0, "", errLegacy(notEnoughMsg)
	}
	endtime, err := s.addUserBuffer(ctx, uid, bufType, delay, updateDelay)
	if err != nil {
		return 0, "", err
	}
	if err := s.ReduceGoods(ctx, uid, gid, 1, 0); err != nil {
		return 0, "", err
	}
	return endtime, text, nil
}

// useBaGuaZhenTu 对齐 1938：delay 基础86400/高级×7/强 86400/24，×useCount；与 oldtype 互斥。
// useXianZhenZhaoGu(1879) 与其同构但 legacy 不乘 useCount（reduceGoods 固定 1），单独处理。
func (s *Service) useZhenTu(ctx context.Context, uid, gid, useCount int) (int64, string, error) {
	delay := int64(86400)
	oldType := 10085
	bufType := 6
	text := "八卦阵图有效期截止到"
	if gid == 49 {
		delay = 86400 * 7
		text = "八卦阵图有效期截止到"
	}
	if gid == 10085 {
		oldType = 6
		bufType = 10085
		delay = 86400 / 24
		text = "强八卦阵图有效期截止到"
	}
	delay = delay * int64(useCount)

	goodCnt, err := s.goodsCount(ctx, uid, gid)
	if err != nil {
		return 0, "", err
	}
	if goodCnt < int64(useCount) {
		return 0, "", errLegacy("当前物品不足")
	}
	active, err := s.bufferActive(ctx, uid, oldType)
	if err != nil {
		return 0, "", err
	}
	if active {
		return 0, "", errLegacy("八卦阵图和强八卦阵图不能同时使用。")
	}
	endtime, err := s.addUserBuffer(ctx, uid, bufType, delay, delay)
	if err != nil {
		return 0, "", err
	}
	if err := s.ReduceGoods(ctx, uid, gid, int64(useCount), 0); err != nil {
		return 0, "", err
	}
	return endtime, text, nil
}

// useZhaoGu 对齐 useXianZhenZhaoGu:1879（legacy 分发只传 uid/gid，不传 useCount；固定消耗 1 份）。
func (s *Service) useZhaoGu(ctx context.Context, uid, gid int) (int64, string, error) {
	delay := int64(86400)
	oldType := 10084
	bufType := 5
	goodsName := "陷阵战鼓"
	text := "陷阵战鼓有效期截止到"
	if gid == 48 {
		delay = 86400 * 7
		goodsName = "高级陷阵战鼓"
	}
	if gid == 10084 {
		oldType = 5
		bufType = 10084
		delay = 86400 / 24
		goodsName = "强陷阵战鼓"
		text = "强陷阵战鼓有效期截止到"
	}
	ok, err := s.checkGoods(ctx, uid, gid)
	if err != nil {
		return 0, "", err
	}
	if !ok {
		return 0, "", errLegacy(fmt.Sprintf("你没有%s，不能使用。", goodsName))
	}
	active, err := s.bufferActive(ctx, uid, oldType)
	if err != nil {
		return 0, "", err
	}
	if active {
		return 0, "", errLegacy("陷阵战鼓和强陷阵战鼓不能同时使用。")
	}
	endtime, err := s.addUserBuffer(ctx, uid, bufType, delay, delay)
	if err != nil {
		return 0, "", err
	}
	if err := s.ReduceGoods(ctx, uid, gid, 1, 0); err != nil {
		return 0, "", err
	}
	return endtime, text, nil
}

// useQingNangShu 对齐 1972：25→buftype9/86400、165→buftype165/86400、10083→buftype10083/3600；
// 与 oldtype 互斥；time×useCount。
func (s *Service) useQingNangShu(ctx context.Context, uid, gid, useCount int) (int64, string, error) {
	oldType := 10083
	bufType := 9
	timeSec := int64(86400)
	text := "“青囊书”有效期截止到"
	if gid == 165 {
		bufType = 165
		text = "“高级青囊书”有效期截止到"
	}
	if gid == 10083 {
		oldType = 9
		bufType = 10083
		timeSec = 3600
	}
	goodCnt, err := s.goodsCount(ctx, uid, gid)
	if err != nil {
		return 0, "", err
	}
	if goodCnt < int64(useCount) {
		return 0, "", errLegacy("当前物品不足")
	}
	timeSec = timeSec * int64(useCount)
	active, err := s.bufferActive(ctx, uid, oldType)
	if err != nil {
		return 0, "", err
	}
	if active {
		return 0, "", errLegacy("青囊书和上级青囊书不能同时使用。")
	}
	endtime, err := s.addUserBuffer(ctx, uid, bufType, timeSec, timeSec)
	if err != nil {
		return 0, "", err
	}
	if err := s.ReduceGoods(ctx, uid, gid, int64(useCount), 0); err != nil {
		return 0, "", err
	}
	return endtime, text, nil
}

// UseMianZhanPai 对齐 1686（fromIndex=1 即 useGoods gid==12 入口；休战 fromIndex==3 分支属 M7 结算，未触发）。
// 依赖：users.state=2（免战）、buftype7 免战时长、buftype8 免战冷却、user_action_log 连用计数。
// mem_state state=197（服务器类型）无表→serverType 恒 0，走 sys_user_action_log 分支（与 legacy 默认服一致）。
func (s *Service) UseMianZhanPai(ctx context.Context, uid int) (int64, string, error) {
	cooling, err := s.db.FetchOne(ctx,
		"select endtime, endtime-unix_timestamp() as lefttime from user_buffers where user_id=? and buftype=8", uid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, "", err
	}
	if err == nil && model.Int64(cooling, "endtime") != 0 { // 正处于免战冷却时期
		delta := model.Int64(cooling, "lefttime")
		return 0, "", errLegacy(fmt.Sprintf("免战结束3小时后才可以再次使用，还需%s ！", makeTimeLeft(delta)))
	}

	leftTime, err := s.bufferEnd(ctx, uid, 7)
	if err != nil {
		return 0, "", err
	}

	// 休假冷却 buffer(913)——M9 休假未实现，恒无。
	var usecount int64
	if leftTime > 0 {
		row, err := s.db.FetchOne(ctx,
			"select `count`, id from user_action_log where user_id=? and action='usemianzhan' limit 1", uid)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return 0, "", err
		}
		if err == nil {
			usecount = model.Int64(row, "count")
			if usecount >= 5 {
				return 0, "", errLegacy("已经使用了5次免战，需要等待冷却时间结束后才可以继续使用")
			}
		}
	} else {
		if _, err := s.db.Exec(ctx,
			"update user_action_log set `count`=0 where user_id=? and action='usemianzhan'", uid); err != nil {
			return 0, "", err
		}
	}

	// 连续使用需要的物品个数是连续次数加 1（legacy:1738-1746）
	if usecount > 1 {
		usecount = 1
	}
	reduceCount := usecount + 1
	ok, err := s.checkGoodsCount(ctx, uid, 12, reduceCount)
	if err != nil {
		return 0, "", err
	}
	if !ok {
		return 0, "", errLegacy(fmt.Sprintf("免战牌不够，需要%s个免战牌", strconv.FormatInt(reduceCount, 10)))
	}
	if err := s.ReduceGoods(ctx, uid, 12, reduceCount, 0); err != nil {
		return 0, "", err
	}

	if _, err := s.db.Exec(ctx, "update users set state=2 where id=?", uid); err != nil {
		return 0, "", err
	}
	usetime := int64(12 * 3600)
	endtime, err := s.addUserBuffer(ctx, uid, 7, usetime, usetime)
	if err != nil {
		return 0, "", err
	}

	newCount := usecount + 1
	mark := "使用免战牌"
	cnt, err := s.db.FetchCellInt64(ctx,
		"select count(1) from user_action_log where user_id=? and action='usemianzhan'", uid)
	if err != nil {
		return 0, "", err
	}
	if cnt == 0 {
		if _, err := s.db.Exec(ctx,
			"insert into user_action_log (user_id, action, `count`, `time`, mark) values (?, 'usemianzhan', ?, unix_timestamp(), ?)",
			uid, newCount, mark); err != nil {
			return 0, "", err
		}
	} else {
		if _, err := s.db.Exec(ctx,
			"update user_action_log set `count`=`count`+1, `time`=unix_timestamp() where user_id=? and action='usemianzhan'",
			uid); err != nil {
			return 0, "", err
		}
	}
	return endtime, "免战牌有效期截止到", nil
}
