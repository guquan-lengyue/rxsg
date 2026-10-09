package testutil

// 真库测试支撑（用户决策：全部连 config.yaml 指向的远程 rxsg_test）。
// 每个用例创建唯一测试账号 + 专属城池 + 配套资源/武将，t.Cleanup 删除，互不干扰。

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"rxsg/backend/internal/config"
	"rxsg/backend/internal/db"
)

// NewTestDB 加载 backend/config.yaml 并连接真库。
func NewTestDB(t *testing.T) *db.DB {
	t.Helper()
	cfgPath := ConfigPath(t)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load config %s: %v", cfgPath, err)
	}
	d, err := db.Open(cfg.DB)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

// ConfigPath 从测试工作目录向上找 config.yaml（backend/config.yaml）。
func ConfigPath(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		p := filepath.Join(dir, "config.yaml")
		if _, err := os.Stat(p); err == nil {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("config.yaml not found from test working dir")
	return ""
}

// Fixture 一套隔离测试数据。
type Fixture struct {
	UID  int
	CID  int
	HID1 int // 城守（affairs 高）
	HID2 int
	DB   *db.DB
}

// SetupCity 创建唯一账号、城池 cid、资源行（people_max=5000, morale=100）、两名武将。
// 数值与 0004 演示城一致，便于对照公式手算结果。
func SetupCity(t *testing.T, d *db.DB) *Fixture {
	t.Helper()
	ctx := context.Background()

	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		t.Fatal(err)
	}
	suffix := hex.EncodeToString(buf)
	passport := fmt.Sprintf("t_%s", suffix)
	hash, err := bcrypt.GenerateFromPassword([]byte("test123456"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}

	uid, err := d.Insert(ctx, "insert into users (passport, password_hash, nickname, state) values (?,?,?,0)",
		passport, string(hash), "测试"+suffix)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	cid, err := d.Insert(ctx,
		"insert into cities (user_id, name, is_special, type, general_hero_id, chief_hero_id, counsellor_hero_id) values (?,?,0,0,0,0,0)",
		uid, "测试城"+suffix)
	if err != nil {
		t.Fatalf("insert city: %v", err)
	}

	if _, err := d.Exec(ctx, `insert into city_resources
		(city_id, wood, wood_max, rock, rock_max, iron, iron_max, food, food_max, gold, gold_max,
		 people, people_max, morale, tax, complaint, vacation)
		values (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		cid, 100000, 500000, 100000, 500000, 100000, 500000, 100000, 500000, 10000, 100000,
		1000, 5000, 100, 0, 0, 0); err != nil {
		t.Fatalf("insert city_resources: %v", err)
	}

	// state=0（空闲）——对齐 legacy sys_city_hero.state 值集 {0,1,4,7,8,10,11}；
	// 旧版简化的 state=3 在 PHP 中不存在，goods/hero 包的 isHeroInCity 判定依赖 0。
	hid1, err := d.Insert(ctx, `insert into heroes
		(user_id, city_id, name, sex, face, state, level, hero_type,
		 command_base, affairs_base, bravery_base, wisdom_base, affairs_add, affairs_add_on, command_add_on, exp)
		values (?,?,?,1,95,0,10,1,80,40,90,60,0,0,0,0)`, uid, cid, "城守"+suffix)
	if err != nil {
		t.Fatalf("insert chief hero: %v", err)
	}
	hid2, err := d.Insert(ctx, `insert into heroes
		(user_id, city_id, name, sex, face, state, level, hero_type,
		 command_base, affairs_base, bravery_base, wisdom_base, affairs_add, affairs_add_on, command_add_on, exp)
		values (?,?,?,1,90,0,10,2,60,95,30,100,0,0,0,0)`, uid, cid, "太守"+suffix)
	if err != nil {
		t.Fatalf("insert hero2: %v", err)
	}
	if _, err := d.Exec(ctx, "update cities set chief_hero_id=?, general_hero_id=? where id=?", hid1, hid1, cid); err != nil {
		t.Fatalf("assign chief: %v", err)
	}
	// 对齐 legacy 进城记 lastcid（goods 包 addCityResources 的目标城）
	if _, err := d.Exec(ctx, "update users set lastcid=? where id=?", cid, uid); err != nil {
		t.Fatalf("set lastcid: %v", err)
	}

	f := &Fixture{UID: int(uid), CID: int(cid), HID1: int(hid1), HID2: int(hid2), DB: d}
	t.Cleanup(f.Cleanup)
	return f
}

// Cleanup 删除本 Fixture 写入的所有行。
func (f *Fixture) Cleanup() {
	d := f.DB
	if d == nil {
		return
	}
	ctx := context.Background()
	deletes := []struct {
		q   string
		uid bool // true=按 UID，false=按 CID
	}{
		// 装备：先删按 hid 关联的表（在删 heroes 前）
		{"delete from hero_armors where hid in (select id from heroes where city_id=?)", false},
		{"delete from hero_attributes where hid in (select id from heroes where city_id=?)", false},
		{"delete from user_armors where user_id=?", true},
		{"delete from things where user_id=?", true},
		{"delete from log_things where user_id=?", true},
		{"delete from log_armor_strong where user_id=?", true},
		{"delete from log_armor_combine where user_id=?", true},
		{"delete from log_selled_armor where user_id=?", true},
		{"delete from log_armor where user_id=?", true},
		{"delete from hero_blood where hero_id in (select id from heroes where city_id=?)", false},
		{"delete from recruit_heroes where city_id=?", false},
		{"delete from hero_exprs where city_id=?", false},
		{"delete from hero_base_add where hero_id in (select id from heroes where city_id=?)", false},
		{"delete from log_goods where user_id=?", true},
		{"delete from log_money where user_id=?", true},
		{"delete from user_goods where user_id=?", true},
		// M6 经济
		{"delete from city_merchants where city_id=?", false},
		{"delete from city_autotrans where user_id=?", true},
		{"delete from user_workshops where user_id=?", true},
		{"delete from log_shops where user_id=?", true},
		{"delete from log_shop_buy_cnts where user_id=?", true},
		{"delete from log_merchants where user_id=?", true},
		{"delete from log_gifts where user_id=?", true},
		{"delete from log_workshop_freshes where user_id=?", true},
		{"delete from log_user_actions where user_id=?", true},
		{"delete from log_action_counts where user_id=?", true},
		{"delete from reports where user_id=?", true},
		{"delete from alarms where user_id=?", true},
		{"delete from user_buffers where user_id=?", true},
		{"delete from city_schedule where city_id=?", false},
		{"delete from city_res_add where city_id=?", false},
		// M7 战斗（先删引用 battles 的子表，再删 battles 本体）
		{"delete from battle_rounds where battleid in (select id from battles where attackuid=? or resistuid=?)", true},
		{"delete from battle_tactics where battleid in (select id from battles where attackuid=? or resistuid=?)", true},
		{"delete from bak_troops where uid=?", true},
		{"delete from battles where attackuid=? or resistuid=?", true},
		{"delete from city_wounded where city_id=?", false},
		{"delete from city_defences where city_id=?", false},
		{"delete from city_resources where city_id=?", false},
		{"delete from city_technics where city_id=?", false},
		{"delete from buildings where city_id=?", false},
		{"delete from troops where city_id=?", false},
		{"delete from heroes where city_id=?", false},
		{"delete from cities where id=?", false},
	}
	for _, dl := range deletes {
		arg := f.CID
		if dl.uid {
			arg = f.UID
		}
		_, _ = d.Exec(ctx, dl.q, arg)
	}
	_, _ = d.Exec(ctx, "delete from city_trades where cid=? or buycid=?", f.CID, f.CID)
	_, _ = d.Exec(ctx, "delete from tickets where user_id=? or binduid=?", f.UID, f.UID)
	_, _ = d.Exec(ctx, "delete from users where id=?", f.UID)
}
