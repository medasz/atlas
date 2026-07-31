package server

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

func (s *Server) registerAudit(g *gin.RouterGroup) {
	g.GET("/audit/logs", s.listAuditLogs)
}

func (s *Server) listAuditLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	search := c.Query("q")

	items, total, err := s.deps.Store.ListAuditLogs(c.Request.Context(), page, pageSize, search)
	if err != nil {
		c.JSON(500, gin.H{"error": "获取审计日志失败: " + err.Error()})
		return
	}

	totalPages := 0
	if pageSize > 0 && total > 0 {
		totalPages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}

	c.JSON(200, gin.H{
		"items":       items,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": totalPages,
	})
}
