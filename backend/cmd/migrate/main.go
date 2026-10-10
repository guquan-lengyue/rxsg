package main

// cmd/migrate 将规范化 schema 与种子数据导入数据库（默认按 backend/config.yaml 连接）。
// 用法（在 backend 目录下）：
//   go run ./cmd/migrate                 # 执行 migrations/0003 + 0004 + 程序化种子
//   go run ./cmd/migrate -config config.yaml
// 幂等：DDL 为 CREATE TABLE IF NOT EXISTS，数据为 REPLACE INTO，可重复执行。

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"

	"rxsg/backend/internal/config"
)

func main() {
	cfgPath := flag.String("config", "config.yaml", "配置文件路径")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=false&loc=Local&multiStatements=true",
		cfg.DB.User, cfg.DB.Password, cfg.DB.Host, cfg.DB.Port, cfg.DB.Name, cfg.DB.Charset)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("ping db: %v", err)
	}
	log.Printf("connected %s:%d/%s", cfg.DB.Host, cfg.DB.Port, cfg.DB.Name)

	files := []string{
		"migrations/0003_normalized.sql",
		"migrations/0005_m1_city.sql",
		"migrations/0006_m2_goods.sql",
		"migrations/0007_m3_hero.sql",
		"migrations/0008_m4_tavern.sql",
		"migrations/0009_m5_armor.sql",
		"migrations/0010_m6_economy.sql",
		"migrations/0011_m7_battle.sql",
		"migrations/0012_m8_task.sql",
		"migrations/0013_m9_activity.sql",
		"migrations/0014_m11_world.sql",
		"migrations/0015_m12_building.sql",
		"migrations/0016_m12b_defence.sql",
		"migrations/0017_m13_office_barn.sql",
		"migrations/0004_seed_normalized.sql",
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			log.Fatalf("read %s: %v", f, err)
		}
		if _, err := db.Exec(string(data)); err != nil {
			log.Fatalf("exec %s: %v", f, err)
		}
		log.Printf("executed %s", f)
	}

	if err := seedUser(db); err != nil {
		log.Fatalf("seed user: %v", err)
	}
	if err := seedHeroLevels(db); err != nil {
		log.Fatalf("seed hero levels: %v", err)
	}
	if err := seedTechnicLevels(db); err != nil {
		log.Fatalf("seed technic levels: %v", err)
	}

	if err := report(db); err != nil {
		log.Fatalf("report: %v", err)
	}
}

// seedUser 重建测试账号（hero_8954dde2 / test123456，bcrypt）。
func seedUser(db *sql.DB) error {
	hash, err := bcrypt.GenerateFromPassword([]byte("test123456"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = db.Exec(`replace into users (id, passport, password_hash, nickname, state, money, honour, nobility, created_at)
		values (1, 'hero_8954dde2', ?, '将帅', 0, 0, 0, '', ?)`, string(hash), time.Now().Unix())
	return err
}

// seedHeroLevels 生成 cfg_hero_levels（140 级）。
// 1-100 公式与原版 dump 一致（004_cfg_seed.sql cfg_hero_level）：upgrade_exp = (level-1)^2 × 100，total_exp 为前缀和。
// 101-140 为合成值（原 dump 仅到 100；M3 突破档位 120/125/130/135 与 heroState 读取 level+1 行
// 需要 136+ 才能正确判定升级），沿用同一公式外推，非原版精确复刻——已在复刻对照表标注。
func seedHeroLevels(db *sql.DB) error {
	var total int64
	for level := 1; level <= 140; level++ {
		upgrade := int64(level-1) * int64(level-1) * 100
		total += upgrade
		if _, err := db.Exec("replace into cfg_hero_levels (level, total_exp, upgrade_exp) values (?,?,?)",
			level, total, upgrade); err != nil {
			return err
		}
	}
	log.Printf("seeded cfg_hero_levels (140)")
	return nil
}

// seedTechnicLevels 生成 cfg_technic_levels（20 科技 × 10 级）。
// 原版 dump 已丢失、仓库无权威数值，此处按建筑曲线同构的合成成本（1.6 倍/级），
// 保证科技面板与升级流程可用；数值非原版精确复刻。
func seedTechnicLevels(db *sql.DB) error {
	for tid := 1; tid <= 20; tid++ {
		base := 1.0
		for level := 1; level <= 10; level++ {
			f := base * pow(1.6, float64(level-1))
			wood := int64(500 * f)
			rock := int64(400 * f)
			iron := int64(300 * f)
			food := int64(600 * f)
			upTime := int64(600 * f)
			if _, err := db.Exec(`replace into cfg_technic_levels
				(tid, level, upgrade_wood, upgrade_rock, upgrade_iron, upgrade_food, upgrade_gold, upgrade_time, description)
				values (?,?,?,?,?,0,?,?,?)`,
				tid, level, wood, rock, iron, food, upTime,
				fmt.Sprintf("Lv.%d", level)); err != nil {
				return err
			}
		}
	}
	log.Printf("seeded cfg_technic_levels (20x10)")
	return nil
}

func pow(a, b float64) float64 {
	r := 1.0
	for i := 0; i < int(b); i++ {
		r *= a
	}
	return r
}

func report(db *sql.DB) error {
	tables := []string{"users", "cities", "city_resources", "buildings", "cfg_buildings", "cfg_building_levels",
		"cfg_technics", "cfg_technic_levels", "cfg_hero_levels", "cfg_soldiers", "cfg_soldier_conditions",
		"heroes", "city_soldiers", "fields", "city_res_add", "city_schedule",
		"cfg_hero_expr_types", "hero_exprs", "hero_base_add", "log_money",
		"recruit_heroes", "hero_blood", "cfg_names",
		"cfg_armor", "user_armors", "hero_armors", "hero_attributes", "cfg_attribute",
		"cfg_strong_probability", "cfg_xilian", "cfg_xilian_type", "cfg_armor_level_attr",
		"cfg_armor_hole_rule", "cfg_tie", "cfg_tie_attribute", "user_tie_deify_attribute",
		"things", "log_things", "log_armor_strong", "log_armor_combine", "log_selled_armor", "log_armor",
		"battles", "battle_rounds", "battle_tactics", "cfg_defence", "bak_troops", "city_wounded", "city_defences",
		"cfg_task_groups", "cfg_tasks", "cfg_task_goals", "cfg_task_rewards", "user_tasks", "user_goals",
		"cfg_achivement_groups", "cfg_achivements", "cfg_achivement_goals", "cfg_achivement_goal_mappings", "user_achivements",
		"log_lottery", "mem_lottery_goods",
		"cfg_pk_battle", "cfg_pk_level", "cfg_pk_hero", "cfg_pk_reward", "cfg_pk_first", "sys_pk_user",
		"mem_world", "user_favourites", "user_inwars", "user_trickwars", "user_states",
		"unions", "union_marks", "union_relations", "log_city_soldiers", "city_lamsters", "city_captives",
		"cfg_nobility", "cfg_world_type", "cfg_office_pos", "cfg_name", "cfg_special_act",
		"building_upgrading", "building_destroying", "cfg_building_conditions", "cfg_defence_conditions"}
	for _, t := range tables {
		var n int64
		if err := db.QueryRow("select count(*) from `" + t + "`").Scan(&n); err != nil {
			return fmt.Errorf("count %s: %w", t, err)
		}
		fmt.Printf("%-24s %6d rows\n", t, n)
	}
	return nil
}
