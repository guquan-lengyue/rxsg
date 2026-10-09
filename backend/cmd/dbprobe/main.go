// 临时探测工具：dump 远程测试库结构与数据规模。用完即删。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"rxsg/backend/internal/config"
	"rxsg/backend/internal/db"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	configPath := flag.String("config", "config.yaml", "配置文件路径")
	full := flag.Bool("full", false, "打印全部表定义")
	q := flag.String("q", "", "执行自定义查询并打印结果")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Println("加载配置失败:", err)
		os.Exit(1)
	}
	fmt.Printf("目标库: %s:%d/%s\n", cfg.DB.Host, cfg.DB.Port, cfg.DB.Name)

	d, err := db.Open(cfg.DB)
	if err != nil {
		fmt.Println("连接失败:", err)
		os.Exit(1)
	}
	defer d.Close()

	ctx := context.Background()
	if *q == "-bcrypt" {
		cands := []string{"123456", "1234567", "12345678", "test1234", "test", "password", "admin", "111111", "000000", "abc123", "e2etest", "e2e", "rxsg", "rxsg123", "666666", "888888"}
		rows, err := d.FetchRows(ctx, "select id,passport,password_hash from users")
		if err != nil {
			fmt.Println("查询失败:", err)
			os.Exit(1)
		}
		found := 0
		for _, r := range rows {
			h := fmt.Sprint(r["password_hash"])
			if b, ok := r["password_hash"].([]byte); ok {
				h = string(b)
			}
			for _, c := range cands {
				if bcrypt.CompareHashAndPassword([]byte(h), []byte(c)) == nil {
					fmt.Printf("命中! passport=%v password=%q\n", r["passport"], c)
					found++
				}
			}
		}
		fmt.Printf("扫描 %d 个账号，命中 %d 个\n", len(rows), found)
		return
	}
	if *q != "" {
		rows, err := d.FetchRows(ctx, *q)
		if err != nil {
			fmt.Println("查询失败:", err)
			os.Exit(1)
		}
		fmt.Printf("共 %d 行\n", len(rows))
		for i, r := range rows {
			fmt.Printf("[%d] %v\n", i, r)
		}
		return
	}
	rows, _ := d.FetchRows(ctx, "show tables")
	tables := make([]string, 0, len(rows))
	for _, r := range rows {
		for _, v := range r {
			s := fmt.Sprint(v)
			if b, ok := v.([]byte); ok {
				s = string(b)
			}
			tables = append(tables, s)
		}
	}

	for _, t := range tables {
		n, _ := d.FetchCellInt64(ctx, "select count(*) from `"+t+"`")
		fmt.Printf("==== %s (rows=%d) ====\n", t, n)
		if !*full && strings.HasPrefix(t, "cfg_") {
			// cfg_ 表只列列名，避免输出过长
		}
		cr, err := d.FetchRows(ctx, "show create table `"+t+"`")
		if err != nil {
			fmt.Println("  err:", err)
			continue
		}
		for _, r := range cr {
			for k, v := range r {
				if k == "Create Table" || k == "Create View" {
					fmt.Println(v)
				}
			}
		}
		fmt.Println()
	}
}
