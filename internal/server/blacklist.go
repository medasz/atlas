package server

import (
	"net/http"

	"atlas/internal/model"

	"github.com/gin-gonic/gin"
)

func (s *Server) registerBlacklist(g *gin.RouterGroup) {
	g.GET("/blacklist", s.listBlacklist)
	g.POST("/blacklist", s.addBlacklist)
	g.DELETE("/blacklist", s.deleteBlacklist)
}

// operatorFromCtx 从请求头取操作人，缺省 system
func operatorFromCtx(c *gin.Context) string {
	if v := c.GetHeader("X-Operator"); v != "" {
		return v
	}
	return "system"
}

func (s *Server) listBlacklist(c *gin.Context) {
	items, err := s.deps.Blacklist.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (s *Server) addBlacklist(c *gin.Context) {
	var req model.BlacklistItem
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.deps.Blacklist.Add(c.Request.Context(), operatorFromCtx(c), req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"ok": true})
}

func (s *Server) deleteBlacklist(c *gin.Context) {
	typ := c.Query("type")
	value := c.Query("value")
	if typ == "" || value == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type and value required"})
		return
	}
	if err := s.deps.Blacklist.Remove(c.Request.Context(), operatorFromCtx(c), typ, value); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
