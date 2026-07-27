package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Server) registerAssets(g *gin.RouterGroup) {
	g.GET("/assets", s.searchAssets)
	g.GET("/hosts/:ip", s.getHost)
	g.GET("/hosts/:ip/detail", s.getHostDetail)
}

// searchAssets 资产检索（ES 优先，未配置则 PG 回退）
func (s *Server) searchAssets(c *gin.Context) {
	q := c.Query("q")
	docType := c.Query("type") // host | port | domain | 空
	items, err := s.deps.Store.SearchAssets(c.Request.Context(), q, docType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// getHost 按 IP 查询主机资产
func (s *Server) getHost(c *gin.Context) {
	h, err := s.deps.Store.GetHost(c.Request.Context(), c.Param("ip"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "host not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"host": h})
}

// getHostDetail 主机详情：主机 + 全部端口（含指纹/HTTP）+ 关联漏洞
func (s *Server) getHostDetail(c *gin.Context) {
	ip := c.Param("ip")
	h, err := s.deps.Store.GetHost(c.Request.Context(), ip)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "host not found"})
		return
	}
	ports, _ := s.deps.Store.ListPortsByIP(c.Request.Context(), ip)
	vulns, _ := s.deps.Store.ListVulnsByHost(c.Request.Context(), ip)
	c.JSON(http.StatusOK, gin.H{"host": h, "ports": ports, "vulns": vulns})
}
