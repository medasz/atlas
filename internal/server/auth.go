package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"

	"github.com/gin-gonic/gin"
)

const sessionCookie = "atlas_session"

// expectedToken 由密钥派生的会话令牌（MVP 单管理员）
func expectedToken(secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("atlas"))
	return hex.EncodeToString(mac.Sum(nil))
}

// cors 允许前端跨域调用（开发期宽松，生产可收拢）
func cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", c.GetHeader("Origin"))
		if c.GetHeader("Origin") == "" {
			c.Header("Access-Control-Allow-Origin", "*")
		}
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Headers", "Content-Type, X-Operator")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// authRequired 保护 /api 路由（除登录/健康检查外）
func (s *Server) authRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !s.deps.Cfg.Auth.Enabled {
			c.Next()
			return
		}
		if c.Request.URL.Path == "/api/login" || c.Request.URL.Path == "/healthz" {
			c.Next()
			return
		}
		tok, err := c.Cookie(sessionCookie)
		if err != nil || tok != expectedToken(s.deps.Cfg.Auth.Secret) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
}

func (s *Server) registerAuth(g *gin.RouterGroup) {
	g.POST("/login", s.login)
	g.POST("/logout", s.logout)
}

type loginReq struct {
	Password string `json:"password"`
}

func (s *Server) login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !s.deps.Cfg.Auth.Enabled || req.Password == s.deps.Cfg.Auth.Password {
		tok := expectedToken(s.deps.Cfg.Auth.Secret)
		c.SetCookie(sessionCookie, tok, 86400*7, "/", "", false, true)
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}
	c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid password"})
}

func (s *Server) logout(c *gin.Context) {
	c.SetCookie(sessionCookie, "", -1, "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
