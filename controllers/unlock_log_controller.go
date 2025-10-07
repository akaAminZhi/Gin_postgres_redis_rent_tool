package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type LockController struct{ *Srv }

func NewLockController(s *Srv) *LockController { return &LockController{Srv: s} }

type unlockRequest struct {
	Reason *string `json:"reason"`
}

func (lc *LockController) Unlock(c *gin.Context) {
	// 1) 解析请求
	var req unlockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "need unlock reason"})
		return
	}
	// if req.TargetType == "" {
	// c.JSON(http.StatusBadRequest, gin.H{"error": "targetType is required"})
	// return
	// }

	// 2) 从鉴权中间件获取操作者信息
	actorIDRaw, _ := c.Get("userID")
	actorNameRaw, _ := c.Get("username")
	actorID, _ := actorIDRaw.(string)
	actorName, _ := actorNameRaw.(string)
	if actorID == "" || actorName == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing user in context"})
		return
	}

	// 3) 这里执行你的实际解锁动作（可选）：
	// - 调硬件 SDK
	// - 调用下游服务
	// - 或者修改其它业务表的状态
	// 失败则直接返回 500/对应错误码

	// 4) 写审计日志
	log, err := lc.Repo.LogUnlock(c.Request.Context(), actorID, actorName, req.Reason)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 5) 返回成功 + 日志
	c.JSON(http.StatusOK, gin.H{
		"ok":        true,
		"unlockLog": log,
	})
}
