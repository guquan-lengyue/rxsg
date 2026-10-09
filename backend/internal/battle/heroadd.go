package battle

// heroadd.go —— 将领战斗加成，1:1 对照 getUsersHeroBattleAdd（OLdBattleCron.php:531-633）。

import (
	"context"
	"database/sql"
	"errors"
	"math"
)

// heroAdd 对齐 getUsersHeroBattleAdd 的 $ret 数组（仅引擎用到的键）。
type heroAdd struct {
	command      float64 // 统率
	bravery      float64
	wisdom       float64
	attack       float64 // 攻击加成
	defence      float64 // 防御加成
	blood        float64
	plund        float64 // 掠夺加成
	shoot        float64
	gongji       float64 // 攻击科技
	fangyu       float64 // 防御科技
	xingjunspeed float64 // 行军速度
	jiayuspeed   float64 // 驾驭速度
	herospeed    float64 // 名将额外速度
	heroshoot    float64 // 名将抛射
	heroattack   float64
	herodefence  float64
	heroblood    float64
}

// heroBattleAdd 对齐 getUsersHeroBattleAdd($hid,$cid,$battletype)。
// 新栈差异（已声明）：
//   - sys_city_hero → heroes；cfg_battle_hero（战场将）无表 → battletype>=4 仍走 heroes（本栈不实现战场战）。
//   - mem_hero_buffer（buftype 1/2/3/4 符）无表 → goods_command/blood/att/def 恒 0（与 armor/city 模块一致）。
func (s *Service) heroBattleAdd(ctx context.Context, hid, cid, battletype int) (*heroAdd, error) {
	ret := &heroAdd{}
	var hero map[string]any
	if hid != 0 {
		row, err := s.db.FetchOne(ctx, "select * from heroes where id=? limit 1", hid)
		if errors.Is(err, sql.ErrNoRows) {
			hero = map[string]any{}
		} else if err != nil {
			return nil, err
		} else {
			hero = row
		}
	} else {
		hero = map[string]any{}
	}

	mjact := 0.0     // 名将攻击加成（bravery_base）
	mjdef := 0.0     // 名将防御加成（wisdom_base）
	mjblood := 0.0   // 名将生命加成（affairs_base）
	myspeed := 0.0   // 名将速度加成（speed_add_on 的 10% 向上取整）
	hrange := 0.0    // 名将抛射加成（command_base）
	// legacy 以 `hid<1027` 判定名将（1..1026 为预置名将行）。新库无预置名将行，
	// 以 heroes.npc_id（名将卡 NPC id，见 0007 迁移）>0 等价映射。
	if hid > 0 && modelInt64(hero, "npc_id") > 0 {
		mjact = modelFloat(hero, "bravery_base")
		mjdef = modelFloat(hero, "wisdom_base")
		mjblood = modelFloat(hero, "affairs_base")
		myspeed = math.Ceil(modelFloat(hero, "speed_add_on") * 0.1)
		hrange = modelFloat(hero, "command_base")
	}

	// 科技等级（sys_city_technic → city_technics）。
	tid := func(t int) (float64, error) {
		v, err := s.techLevel(ctx, cid, t)
		if err != nil {
			return 0, err
		}
		return float64(v), nil
	}
	tecCommand, err := tid(6)
	if err != nil {
		return nil, err
	}
	tecAttc, err := tid(9)
	if err != nil {
		return nil, err
	}
	tecDef, err := tid(10)
	if err != nil {
		return nil, err
	}
	tecSpeed, err := tid(12)
	if err != nil {
		return nil, err
	}
	tecJiayu, err := tid(13)
	if err != nil {
		return nil, err
	}
	tecShoot, err := tid(14)
	if err != nil {
		return nil, err
	}
	tecBlood, err := tid(16)
	if err != nil {
		return nil, err
	}
	tecPlund, err := tid(20)
	if err != nil {
		return nil, err
	}

	// 符/buff 加成恒 0（mem_hero_buffer 无表）。
	goodsCommand, goodsBlood, goodsAtt, goodsDef := 0.0, 0.0, 0.0, 0.0

	// 原版行 615-631（逐行 1:1）。
	ret.command = (modelFloat(hero, "command_base")+modelFloat(hero, "level"))*((1+goodsCommand)+0.1*tecCommand) + modelFloat(hero, "command_add_on")
	ret.bravery = (modelFloat(hero, "bravery_base") + modelFloat(hero, "bravery_add") + modelFloat(hero, "bravery_add_on")) + mjact*(1+0.05*tecAttc)
	ret.wisdom = (modelFloat(hero, "wisdom_base") + modelFloat(hero, "wisdom_add") + modelFloat(hero, "wisdom_add_on")) + mjdef*(1+0.05*tecDef)
	ret.attack = (modelFloat(hero, "bravery_base")+modelFloat(hero, "bravery_add"))*(1+goodsAtt) + modelFloat(hero, "bravery_add_on") + modelFloat(hero, "attack_add_on")/10
	ret.defence = (modelFloat(hero, "wisdom_base")+modelFloat(hero, "wisdom_add"))*(1+goodsDef) + modelFloat(hero, "wisdom_add_on") + modelFloat(hero, "defence_add_on")/10
	ret.blood = ((modelFloat(hero, "affairs_add") + modelFloat(hero, "affairs_add_on") + modelFloat(hero, "affairs_base")) / 500 + goodsBlood) + 0.05*tecBlood
	ret.plund = 1 + 0.03*tecPlund
	ret.shoot = 0.05 * tecShoot
	ret.gongji = 0.05 * tecAttc
	ret.fangyu = 0.05 * tecDef
	ret.xingjunspeed = 0.1 * tecSpeed
	ret.jiayuspeed = tecJiayu * 0.05
	ret.herospeed = myspeed + modelFloat(hero, "speed_add_on")
	ret.heroshoot = hrange * (1 + 0.05*tecShoot)
	ret.heroattack = mjact * (1 + 0.05*tecAttc)
	ret.herodefence = mjdef * (1 + 0.05*tecDef)
	ret.heroblood = mjblood * (1 + 0.05*tecBlood)
	return ret, nil
}
