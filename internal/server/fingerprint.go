package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Server) registerFingerprint(g *gin.RouterGroup) {
	g.POST("/fingerprint/reload", s.reloadFingerprint)
}

// reloadFingerprint 热加载自研指纹规则
func (s *Server) reloadFingerprint(c *gin.Context) {
	if s.deps.Fingerprint == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "fingerprint service unavailable"})
		return
	}
	if err := s.deps.Fingerprint.Reload(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
