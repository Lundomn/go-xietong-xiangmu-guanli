package main

import (
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"net/http"
	"os"
	_ "test.com/project-api/api"
	"test.com/project-api/api/midd"
	"test.com/project-api/config"
	"test.com/project-api/router"
	srv "test.com/project-common"
)

func main() {
	r := gin.Default()
	r.Use(midd.RequestLog())
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	//路由
	router.InitRouter(r)
	// pprof 只在显式开启时暴露，避免生产环境意外开放调试接口。
	if os.Getenv("ENABLE_PPROF") == "1" {
		pprof.Register(r)
	}
	srv.Run(r, config.C.SC.Name, config.C.SC.Addr, nil)
}
