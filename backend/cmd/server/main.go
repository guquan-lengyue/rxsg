package main

import (
	"flag"
	"log"
	"strconv"

	"github.com/gin-gonic/gin"

	"rxsg/backend/internal/army"
	"rxsg/backend/internal/auth"
	"rxsg/backend/internal/building"
	"rxsg/backend/internal/city"
	"rxsg/backend/internal/config"
	"rxsg/backend/internal/db"
	"rxsg/backend/internal/middleware"
	"rxsg/backend/internal/technic"
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
	sessionStore := auth.NewSessionStore()
	authSvc := auth.NewService(database, jwtMgr, sessionStore)
	authHandler := auth.NewHandler(authSvc)
	buildingSvc := building.NewService(database)
	buildingHandler := building.NewHandler(buildingSvc)
	technicHandler := technic.NewHandler(technic.NewService(database))
	armyHandler := army.NewHandler(army.NewService(database))
	cityHandler := city.NewHandler(city.NewService(database, buildingSvc))

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	r.GET("/api/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	api := r.Group("/api/v1")

	// 公开路由：登录、登录公告。
	authHandler.RegisterPublic(api)

	// 受保护路由：需携带 Bearer JWT，且 (uid,sid) 与内存会话一致。
	protected := api.Group("")
	protected.Use(middleware.RequireAuth(jwtMgr, sessionStore))
	authHandler.RegisterProtected(protected)
	cityHandler.Register(protected)
	buildingHandler.Register(protected)
	technicHandler.Register(protected)
	armyHandler.Register(protected)

	addr := ":" + strconv.Itoa(cfg.Server.Port)
	log.Printf("rxsg backend 启动于 %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}