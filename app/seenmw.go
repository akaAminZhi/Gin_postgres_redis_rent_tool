// app/seenmw.go
package app

import (
	"Gin_postgres_redis_rent_tool/db"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func TouchLastSeen(repo *db.Repo, rdb *redis.Client, throttle time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		v, ok := c.Get("userID")
		if !ok {
			c.Next()
			return
		}
		uid, _ := v.(string)
		if uid == "" {
			c.Next()
			return
		}

		key := "user:lastseen:" + uid
		if ok, _ := rdb.SetNX(c, key, "1", throttle).Result(); ok {
			_ = repo.TouchUserSeen(c, uid) // 忽略错误，不阻塞请求
		}
		c.Next()
	}
}
