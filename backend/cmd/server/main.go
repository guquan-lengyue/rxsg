package main

import (
	"flag"
	"log"
	"strconv"

	"github.com/gin-gonic/gin"

	"rxsg/backend/internal/auth"
	"rxsg/backend/internal/city"
	"rxsg/backend/internal/config"
	"rxsg/backend/internal/db"
	"rxsg/backend/internal/middleware"
)

func main() {
	configPath := flag.String("config", "config.yaml", "配置文件路径")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	database, err := db.Open(cfg.DB)
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	defer database.Close()

	jwtMgr := auth.NewJWTManager(cfg.JWT.Secret, cfg.JWT.TTL)
	authSvc := auth.NewService(database, jwtMgr, cfg.Session.FileDir)
	authHandler := auth.NewHandler(authSvc)
	cityHandler := city.NewHandler(city.NewService(database))

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	r.GET("/api/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	api := r.Group("/api/v1")

	// 公开路由：登录、登录公告。
	authHandler.RegisterPublic(api)

	// 受保护路由：需携带 Bearer JWT，且 (uid,sid) 与 sys_sessions 一致。
	protected := api.Group("")
	protected.Use(middleware.RequireAuth(database, jwtMgr))
	authHandler.RegisterProtected(protected)
	cityHandler.Register(protected)

	addr := ":" + strconv.Itoa(cfg.Server.Port)
	log.Printf("rxsg backend 启动于 %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}