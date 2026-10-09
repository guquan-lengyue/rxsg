package goods

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"strconv"

	"rxsg/backend/internal/db"
	"rxsg/backend/internal/lock"
	"rxsg/backend/internal/model"
)

// service.go 1:1 复刻 legacy server/game/GoodsFunc.php useGoods(28-630) 分发器
// 与 loadUserGoods(8)。分支顺序、ret[] 结构、报错文案逐条对齐 PHP；
// 原版 bug/怪癖（徭役令 on-duplicate 只 +259200、0706 八进制=454、10308 第5占位文案错写"第10名"、
// lang 键缺失→空串、dispatch 不路由 10083/10084）全部保留。
// 礼盒/掉落类实现见 boxes.go，功能类实现见 misc.go，buff 类见 buff.go。

type Service struct {
	db *db.DB
	lk *lock.Locker
}

func NewService(d *db.DB) *Service {
	return &Service{db: d, lk: lock.New(d.DB)}
}

// UseResult 对齐 legacy $ret 数组：[gid, mode, ...payload]。
// mode: 0 消息框 / 1 宝物列表 / 2 有效期提示 / 3 开出装备 / 4 高级推恩 / 5 推恩 / 6 答题。
type UseResult struct {
	GID      int              `json:"gid"`
	Mode     int              `json:"mode"`
	Message  string           `json:"message,omitempty"`
	Endtime  int64            `json:"endtime,omitempty"`
	Nobility int              `json:"nobility,omitempty"`
	Left     int64            `json:"left,omitempty"`
	Hid      int              `json:"hid,omitempty"`
	Goods    []map[string]any `json:"goods,omitempty"`
}

// LoadUserGoods 对齐 loadUserGoods（GoodsFunc.php:8）：
// select * from sys_goods join cfg_goods where uid and count>0 order by group,gid。
func (s *Service) LoadUserGoods(ctx context.Context, uid int) ([]map[string]any, error) {
	return s.db.FetchRows(ctx, `select g.gid, g.count, f.name, f.group_id, f.position, f.value
		from user_goods g join cfg_goods f on g.gid=f.gid
		where g.user_id=? and g.count>0 order by f.group_id, g.gid`, uid)
}

// UseGoods 对齐 useGoods($uid,$param)。hid 仅武魂段（110000<gid<160000）使用。
func (s *Service) UseGoods(ctx context.Context, uid, gid, hid, useCount int) (*UseResult, error) {
	// 入口校验（GoodsFunc.php:30-36）。
	if gid > 110000 && gid < 160000 {
		useCount = 1
	}
	if useCount < 1 {
		useCount = 1
	}
	if useCount >= 100 {
		return nil, errLegacy("最高次数为99次！")
	}

	cnt, err := s.goodsCount(ctx, uid, gid)
	if err != nil {
		return nil, err
	}
	cid, err := s.lastCityID(ctx, uid)
	if err != nil {
		return nil, err
	}
	if cnt == 0 { // legacy empty($cnt)
		if gid > 110000 && gid < 160000 {
			return nil, errLegacy("你使用的武魂数量为0，请合成后再使用。")
		}
		if notEnoughGIDs(gid) {
			return nil, errLegacy(fmt.Sprintf("not_enough_goods%d", gid))
		}
		return nil, errLegacy("你拥有的该道具数量为0，请去商城购买后再使用。")
	}

	var out *UseResult
	err = s.lk.WithUserLock(ctx, uid, "usegoods", func(ctx context.Context) error {
		var e error
		out, e = s.dispatch(ctx, uid, cid, hid, gid, useCount)
		return e
	})
	if err != nil {
		if err.Error() == "lock_busy" {
			return nil, legacyBusy()
		}
		return nil, err
	}
	return out, nil
}

func legacyBusy() error {
	return errLegacy("服务器忙，请稍后再进行操作。")
}

// notEnoughGIDs 对齐 GoodsFunc.php:47-56 的入口报错集合。
func notEnoughGIDs(gid int) bool {
	switch gid {
	case 2, 44, 3, 45, 4, 46, 5, 47, 54, 55, 57, 25, 6, 48, 7, 49, 60, 61, 62,
		58, 140, 56, 142, 124, 117, 120, 15, 133, 134, 154, 164, 166, 165:
		return true
	}
	return false
}

// expiredGIDs 对齐 GoodsFunc.php:542 过期集合。
func expiredGIDs(gid int) bool {
	switch gid {
	case 10014, 10015, 10083, 10270, 10279, 10282, 10254:
		return true
	}
	return gid >= 10035 && gid <= 10056
}

// dispatch 按 PHP if-else 顺序逐分支实现。
func (s *Service) dispatch(ctx context.Context, uid, cid, hid, gid, useCount int) (*UseResult, error) {
	switch {
	case gid == 1:
		return nil, errLegacy("传音符是在世界频道聊天的时候使用。")

	case gid == 2 || gid == 44 || gid == 3 || gid == 45 || gid == 4 || gid == 46 || gid == 5 || gid == 47:
		endtime, text, err := s.useProdTool(ctx, uid, s.prodToolFor(ctx, gid), gid, useCount)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 2, Message: text, Endtime: endtime}, nil

	case gid == 6 || gid == 48: // 注：10084 不在此列（dispatch 不路由，落入默认分支，原版如此）
		endtime, text, err := s.useZhaoGu(ctx, uid, gid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 2, Message: text, Endtime: endtime}, nil

	case gid == 161501:
		endtime, err := s.userXianDiZhaoShu(ctx, uid, gid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 2, Message: "献帝诏书有效期截止到", Endtime: endtime}, nil

	case gid == 7 || gid == 49 || gid == 10085:
		endtime, text, err := s.useZhenTu(ctx, uid, gid, useCount)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 2, Message: text, Endtime: endtime}, nil

	case gid == 8:
		return nil, errLegacy("墨家残卷")
	case gid == 9:
		return nil, errLegacy("墨家图纸")
	case gid == 10:
		return nil, errLegacy("墨家典籍")

	case gid == 12:
		endtime, text, err := s.UseMianZhanPai(ctx, uid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 2, Message: text, Endtime: endtime}, nil

	case gid == 13:
		return nil, errLegacy("锦囊")

	case gid == -100:
		n, err := s.useaddyuanbao(ctx, uid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 0, Message: fmt.Sprintf("恭喜你获得%d元宝", n)}, nil

	case gid == 15:
		if err := s.useMenzhulin(ctx, uid, cid); err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 0, Message: "“盟主令”使用成功，联盟成员人数上限提升为150。"}, nil

	case gid == 22:
		return nil, errLegacy("洗髓丹是在给将领洗点的时候使用。")
	case gid == 23:
		return nil, errLegacy("“招贤榜”在客栈的“招贤纳士”处使用。")

	case gid == 16 || gid == 19:
		list, err := s.useCopperBox(ctx, uid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 1, Goods: list}, nil
	case gid == 17 || gid == 20:
		list, err := s.useSilverBox(ctx, uid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 1, Goods: list}, nil
	case gid == 18 || gid == 21:
		list, err := s.useGoldBox(ctx, uid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 1, Goods: list}, nil

	case gid == 25:
		endtime, text, err := s.useQingNangShu(ctx, uid, gid, useCount)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 2, Message: text, Endtime: endtime}, nil
	case gid == 165:
		endtime, text, err := s.useQingNangShu(ctx, uid, gid, useCount)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 2, Message: text, Endtime: endtime}, nil

	case gid >= 41 && gid <= 43:
		list, err := s.openGemBox(ctx, uid, gid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 1, Goods: list}, nil

	case gid == 10428 || gid == 10487:
		list, err := s.openXuanBinBox(ctx, uid, gid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 1, Goods: list}, nil

	case gid > 40000 && gid < 50000:
		msg, err := s.addSkillBook(ctx, uid, gid, useCount)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 0, Message: msg}, nil

	case gid == 50:
		list, err := s.useOldWoodBox(ctx, uid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 1, Goods: list}, nil

	case gid == 51:
		endtime, text, err := s.useSimpleBuffer(ctx, uid, 51, 10, 604800, 604800,
			"你拥有的该道具数量为0，请去商城购买后再使用。", "“清仓令”有效期截止到")
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 2, Message: text, Endtime: endtime}, nil

	case gid == 52:
		return nil, errLegacy("墨家密笈")

	case gid == 54 || gid == 55:
		endtime, text, err := s.useShuiLiBian(ctx, uid, gid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 2, Message: text, Endtime: endtime}, nil

	case gid == 56:
		endtime, text, err := s.useYaoYiLinAll(ctx, uid, 56, 1)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 2, Message: text, Endtime: endtime}, nil
	case gid == 166:
		endtime, text, err := s.useYaoYiLinAll(ctx, uid, 166, useCount)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 2, Message: text, Endtime: endtime}, nil

	case gid == 10321:
		// lang MuBingLing_valid_date 键缺失 → 空串（原版如此，保留）。
		endtime, text, err := s.useSimpleBuffer(ctx, uid, 10321, 10321, 86400, 86400,
			"not_enough_goods10321", "")
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 2, Message: text, Endtime: endtime}, nil

	case gid == 19061 || gid == 19062 || gid == 19063:
		if err := s.getOutTroops(ctx, uid, cid, gid); err != nil {
			return nil, err
		}
		text := map[int]string{19061: "南蛮战书使用成功", 19062: "山越战书使用成功", 19063: "匈奴战书使用成功"}[gid]
		return &UseResult{GID: gid, Mode: 0, Message: text}, nil

	case gid == 10931:
		msg, err := s.useWangZheBingFu(ctx, uid, cid, gid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 0, Message: msg}, nil

	case (gid >= 10932 && gid <= 10937) || gid == 10996 || (gid >= 11021 && gid <= 11022) || gid == 11078:
		msg, err := s.changeCityMap(ctx, uid, cid, gid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 0, Message: msg}, nil

	case gid >= 10957 && gid <= 10959:
		msg, err := s.useAddUserHeroExpBook(ctx, uid, cid, gid, useCount)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 0, Message: msg}, nil

	case gid == 133:
		endtime, err := s.useJunLingZhuangAll(ctx, uid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 2, Message: "“军令状”有效期截止到", Endtime: endtime}, nil

	case gid == 117:
		nobility, left, err := s.useGaojiTuiEnLing(ctx, uid, gid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 4, Nobility: nobility, Left: left}, nil

	case gid == 124:
		nobility, left, err := s.useTuiEnLing(ctx, uid, gid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 5, Nobility: nobility, Left: left}, nil

	case gid == 119:
		list, err := s.useTreasureBox(ctx, uid, gid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 1, Goods: list}, nil

	case gid == 120:
		endtime, text, err := s.useSimpleBuffer(ctx, uid, 120, 17, 259200, 259200,
			"not_enough_goods120", "“商队契约”有效期截止到")
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 2, Message: text, Endtime: endtime}, nil

	case gid == 57:
		msg, err := s.useDianMinLin(ctx, uid, cid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 0, Message: msg}, nil

	case gid == 139:
		msg, err := s.useTaiPingYaoShu(ctx, uid, cid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 0, Message: msg}, nil

	case gid == 164: // 沙场令：sys_sc_user 无表→legacy sql_query 静默失败，仅文案+扣道具（保留）
		if err := s.ReduceGoods(ctx, uid, 164, 1, 0); err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 0, Message: "沙场次数清0成功"}, nil

	case gid == 142:
		endtime, err := s.useXunChaLin(ctx, uid, cid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 2, Message: "使用成功，“巡查令”使用期限截止到", Endtime: endtime}, nil

	case gid == 58:
		msg, err := s.useAnMingGaoShi(ctx, uid, cid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 0, Message: msg}, nil

	case gid == 59:
		return nil, errLegacy("“军旗”在军队出征的时候使用。")

	case gid == 60 || gid == 61 || gid == 62:
		endtime, _, err := s.useKaoGongJi(ctx, uid, gid)
		if err != nil {
			return nil, err
		}
		name, err := s.goodsName(ctx, gid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 2, Message: fmt.Sprintf("“%s”有效时间截止到", name), Endtime: endtime}, nil

	case gid == 63:
		return nil, errLegacy("“韩信三篇”是在对招兵队列加速的时候使用。")
	case gid == 64:
		return nil, errLegacy("“备城门”在对城防建造队列加速的时候使用。")

	case gid == 85 || gid == 86:
		msg, err := s.openGoldBar(ctx, uid, cid, gid, useCount)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 0, Message: msg}, nil

	case gid >= 87 && gid <= 94:
		msg, err := s.openResBox(ctx, uid, cid, gid, useCount)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 0, Message: msg}, nil

	case gid == 134:
		msg, err := s.useSheMianWenShu(ctx, uid, gid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 0, Message: msg}, nil

	case gid == 158:
		msg, err := s.useShiShiWenShu(ctx, uid, gid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 0, Message: msg}, nil

	case gid == 138:
		msg, err := s.useQingZhanShu(ctx, uid, gid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 0, Message: msg}, nil

	case gid == 95 || gid == 96 || gid == 97 || (gid >= 101 && gid <= 112):
		list, err := s.useArmorBox(ctx, uid, gid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 3, Goods: list}, nil

	case gid == 145 || gid == 146:
		n, err := s.addArmorShelf(ctx, uid, gid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 0, Message: fmt.Sprintf("你的装备栏增加到%d个", n)}, nil

	case gid > 110000 && gid < 160000:
		msg, err := s.useWuHun(ctx, uid, hid, gid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 0, Message: msg, Hid: hid}, nil

	case gid == 10013:
		msg, err := s.useResourcePackage(ctx, uid, gid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 0, Message: msg}, nil

	case gid == 10017:
		list, err := s.useKeyChain(ctx, uid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 1, Goods: list}, nil

	case gid == 121 || gid == 122:
		msg, err := s.useXiuJiaFu(ctx, uid, cid, gid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 0, Message: msg}, nil

	case (gid >= 50101 && gid <= 50110) || (gid >= 1000001 && gid <= 1000010):
		needLevel := gid - 50100
		if gid >= 1000001 {
			needLevel = gid - 1000000
		}
		govLevel, err := s.governmentLevel(ctx, uid)
		if err != nil {
			return nil, err
		}
		if govLevel < int64(needLevel) {
			name, err := s.goodsName(ctx, gid)
			if err != nil {
				return nil, err
			}
			return nil, errLegacy(fmt.Sprintf("你的官府等级不足%d级，不能打开%s。", needLevel, name))
		}
		list, err := s.openDynamicBox(ctx, uid, gid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 1, Goods: list}, nil

	case gid >= 50000 && gid <= 60000:
		list, err := s.openDynamicBox(ctx, uid, gid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 1, Goods: list}, nil

	case gid == 147:
		list, err := s.useShenMiChuanYinFu(ctx, uid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 6, Goods: list}, nil

	case gid == 150:
		list, err := s.useDaTiLiBao(ctx, uid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 1, Goods: list}, nil

	case gid == 153:
		list, err := s.openBaoshiBox(ctx, uid, gid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 1, Goods: list}, nil

	case gid == 200:
		list, err := s.openBaozhuhe(ctx, uid, gid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 1, Goods: list}, nil

	case gid == 154:
		endtime, text, err := s.useSimpleBuffer(ctx, uid, 154, 22, 604800, 604800,
			"not_enough_goods154", "“求贤诏”有效时间截止到")
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 2, Message: text, Endtime: endtime}, nil

	case gid == 156:
		msg, err := s.openHeroBoxTaskreward(ctx, uid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 0, Message: msg}, nil
	case gid == 10215:
		msg, err := s.openHeroBoxActreward(ctx, uid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 0, Message: msg}, nil
	case gid == 11227:
		msg, err := s.openHeroBoxTongling(ctx, uid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 0, Message: msg}, nil
	case gid == 11228:
		msg, err := s.openHeroBoxShiwei(ctx, uid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 0, Message: msg}, nil
	case gid == 12051:
		msg, err := s.openHeroBoxJiajiang(ctx, uid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 0, Message: msg}, nil

	case gid == 10158:
		endtime, err := s.useAdvancedConstructionPlan(ctx, uid, cid, gid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 2, Message: "“高级建筑图纸”有效时间截止到", Endtime: endtime}, nil

	case gid == 250:
		msg, err := s.giveMeHeroCard(ctx, uid, gid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 0, Message: msg}, nil

	case gid > 210000 && gid < 250000:
		msg, err := s.generateHero4Card(ctx, uid, cid, gid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 0, Message: msg}, nil

	case gid == 251 || gid == 252 || gid == 253 || gid == 254:
		msg, err := s.useArmyOrder(ctx, uid, cid, gid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 0, Message: msg}, nil

	case gid == 14:
		if err := s.useMenZhuMiZhao(ctx, uid); err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 0, Message: "“盟主密诏”使用成功，恭喜您已经成为新的盟主！"}, nil

	case (gid > 10000 && gid <= 10012) || gid == 10072 || gid == 10016 || (gid >= 10018 && gid <= 10024):
		list, err := s.useHuodongGoods(ctx, uid, gid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 1, Goods: list}, nil

	case expiredGIDs(gid):
		return nil, errLegacy("这个道具已过期")

	case gid == 10303 || gid == 10304:
		list, err := s.openEggBox(ctx, uid, gid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 1, Goods: list}, nil

	case gid == 10308 || gid == 10417:
		return nil, errLegacy(s.rankMsg(ctx, uid, gid))

	case gid == 10314:
		if err := s.AddGoods(ctx, uid, 10314, -1, 6); err != nil {
			return nil, err
		}
		if isLucky(30, 100, 1) {
			if err := s.AddGoods(ctx, uid, 10312, 1, 6); err != nil {
				return nil, err
			}
			// sys_inform 广播无表→省略（原版为全服滚动公告）。
			row, err := s.db.FetchOne(ctx, "select *,1 as `count` from cfg_goods where gid=10312")
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return nil, err
			}
			arr := []map[string]any{row}
			return &UseResult{GID: gid, Mode: 1, Goods: arr}, nil
		}
		return nil, errLegacy("恭喜您被万圣节恶魔整蛊了，这个面具是假的！")

	case gid == 10333:
		endtime, err := s.useZhaoAnLing(ctx, uid, gid, useCount)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 2, Message: "“诏安令”有效期截止到", Endtime: endtime}, nil

	case gid == 1015:
		list, err := s.openSuiPianHe(ctx, uid, gid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 1, Goods: list}, nil

	case gid == 10816:
		if err := s.userWangZheZhiCheng(ctx, uid, gid, cid); err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 0, Message: "成功使用道具"}, nil

	case gid == 1016:
		list, err := s.openBazhuHe(ctx, uid, gid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 1, Goods: list}, nil

	case gid >= 200000 && gid <= 210000:
		list, err := s.openCombineArmorBox(ctx, uid, gid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 1, Goods: list}, nil

	case gid >= 8887 && gid <= 8892:
		var msg string
		var err error
		if gid < 8891 {
			msg, err = s.openlongyuanArmorBox(ctx, uid, gid)
		} else {
			msg, err = s.openlongyuantieArmorBox(ctx, uid, gid)
		}
		if err != nil {
			return nil, err
		}
		return nil, errLegacy(msg) // legacy 此处为 throw（ret[]=1 已入数组但被异常丢弃）

	case gid == 8893 || gid == 8894:
		msg, err := s.openunionandbattleBox(ctx, uid, gid)
		if err != nil {
			return nil, err
		}
		return nil, errLegacy(msg)

	default: // 默认打开道具，一般为活动道具（GoodsFunc.php:621-624）
		list, err := s.openDefaultBox(ctx, uid, cid, gid)
		if err != nil {
			return nil, err
		}
		return &UseResult{GID: gid, Mode: 1, Goods: list}, nil
	}
}

// goodsName 对齐 select name from cfg_goods where gid=...。
func (s *Service) goodsName(ctx context.Context, gid int) (string, error) {
	v, err := s.db.FetchCellString(ctx, "select name from cfg_goods where gid=?", gid)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// governmentLevel 对齐 select max(b.level) from sys_building b,sys_city c where b.cid=c.cid and c.uid=? and b.bid=6。
func (s *Service) governmentLevel(ctx context.Context, uid int) (int64, error) {
	v, err := s.db.FetchCellInt64(ctx, `select coalesce(max(b.level),0) from buildings b join cities c on b.city_id=c.id
		where c.user_id=? and b.building_id=6`, uid)
	return v, err
}

// useJunLingZhuangAll 对齐 useJunLingZhuang(2304)：buftype=19、bufparam=3、+86400。
func (s *Service) useJunLingZhuangAll(ctx context.Context, uid int) (int64, error) {
	ok, err := s.checkGoods(ctx, uid, 133)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, errLegacy("not_enough_goods133")
	}
	if err := s.addUserBufferParam(ctx, uid, 19, 3, 86400, 86400); err != nil {
		return 0, err
	}
	if err := s.ReduceGoods(ctx, uid, 133, 1, 0); err != nil {
		return 0, err
	}
	return s.bufferEnd(ctx, uid, 19)
}

// addUserBufferParam 同 addUserBuffer，但新行写入 bufparam（军令状/巡查令/推恩令等 legacy insert 带 bufparam）。
func (s *Service) addUserBufferParam(ctx context.Context, uid, buftype, bufparam int, insertDelay, updateDelay int64) error {
	_, err := s.db.Exec(ctx,
		"insert into user_buffers (user_id, buftype, bufparam, endtime) values (?,?,?,unix_timestamp()+?) "+
			"on duplicate key update endtime=endtime+?",
		uid, buftype, bufparam, insertDelay, updateDelay)
	return err
}

// rankMsg 对齐 GoodsFunc.php:549-574（10308/10417 收藏排行）。
// users.prestige 列缺失 → 并列按声望的平手判定省略；order by count desc, prestige desc 退化为 count desc。
// 第 5 个占位文案错写"第10名"（原版 bug，1:1 保留）。
func (s *Service) rankMsg(ctx context.Context, uid, gid int) string {
	dash := "--"
	myCount := int64(0)
	if v, err := s.db.FetchCellInt64(ctx, "select `count` from user_goods where gid=? and `count`>0 and user_id=?", gid, uid); err == nil {
		myCount = v
	}
	myRank := ""
	if myCount > 0 {
		if v, err := s.db.FetchCellInt64(ctx, "select count(1)+1 from user_goods where gid=? and `count`>?", gid, myCount); err == nil {
			myRank = strconv.FormatInt(v, 10)
		}
	}
	rows, err := s.db.FetchRows(ctx, "select `count` from user_goods where gid=? and `count`>0 order by `count` desc limit 100", gid)
	if err != nil {
		rows = nil
	}
	at := func(i int) string {
		if i < len(rows) {
			if c := model.Int64(rows[i], "count"); c != 0 {
				return strconv.FormatInt(c, 10)
			}
		}
		return dash
	}
	// legacy 对 $my_count 赋 '--' 但未参与 rankmsg 拼接（1:1 保留，无输出）。
	if myRank == "" {
		myRank = dash
	}
	return fmt.Sprintf("我当前排名：%s;\n第10名拥有个数：%s;\n第20名拥有个数：%s;\n第50名拥有个数：%s;第10名拥有个数：%s;",
		myRank, at(9), at(19), at(49), at(99))
}

// isLucky 对齐 ActFunc.php:36 isLucky($luckyResult,$maxResult,$minResult=1)：mt_rand(min,max)>=? 反向：lucky>=rand。
func isLucky(lucky, maxResult, minResult int) bool {
	r := rand.Intn(maxResult-minResult+1) + minResult
	return lucky >= r
}

// openDefaultBox 对齐 GoodsFunc.php:3983（srctype=0 分支）。
// cfg_box_open_condition/cfg_box_details 无表 → 走 legacy 空结果路径 func_not_in_use。
func (s *Service) openDefaultBox(ctx context.Context, uid, cid, gid int) ([]map[string]any, error) {
	_ = cid
	ok, err := s.checkGoods(ctx, uid, gid)
	if err != nil {
		return nil, err
	}
	if !ok {
		name, err := s.goodsName(ctx, gid)
		if err != nil {
			return nil, err
		}
		return nil, errLegacy(fmt.Sprintf("你没有%s，不能使用。", name))
	}
	// cfg_box_open_condition 无行（缺表）→ 继续；cfg_box_details 无行 → func_not_in_use。
	return nil, errLegacy("此功能尚未开放。")
}

// prodToolFor 返回四产具定义（buff.go 的 useProdTool 使用）。
// extraFn 对齐 addCityResources($cid, wood, rock, iron, food, gold) 的资源位（基础 500 / 高级 5000）。
func (s *Service) prodToolFor(ctx context.Context, gid int) prodToolDef {
	switch gid {
	case 3, 45: // 鲁班斧 → 木材
		return prodToolDef{bufType: 2, goodsCol: "goods_wood_add", delay: 86400, advDelay: 86400 * 7,
			basicGid: 3, advGid: 45, validText: "鲁班斧有效期截止到",
			extraFn: func(cid int64, extra int64) error { return s.addCityResources(ctx, int(cid), extra, 0, 0, 0, 0) }}
	case 4, 46: // 开山锤 → 石料
		return prodToolDef{bufType: 3, goodsCol: "goods_rock_add", delay: 86400, advDelay: 86400 * 7,
			basicGid: 4, advGid: 46, validText: "开山锤有效期截止到",
			extraFn: func(cid int64, extra int64) error { return s.addCityResources(ctx, int(cid), 0, extra, 0, 0, 0) }}
	case 5, 47: // 玄铁炉 → 铁锭
		return prodToolDef{bufType: 4, goodsCol: "goods_iron_add", delay: 86400, advDelay: 86400 * 7,
			basicGid: 5, advGid: 47, validText: "玄铁炉有效期截止到",
			extraFn: func(cid int64, extra int64) error { return s.addCityResources(ctx, int(cid), 0, 0, extra, 0, 0) }}
	default: // 2/44 神农锄 → 粮食
		return prodToolDef{bufType: 1, goodsCol: "goods_food_add", delay: 86400, advDelay: 86400 * 7,
			basicGid: 2, advGid: 44, validText: "神农锄有效期截止到",
			extraFn: func(cid int64, extra int64) error { return s.addCityResources(ctx, int(cid), 0, 0, 0, extra, 0) }}
	}
}
