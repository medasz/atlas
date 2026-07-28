package server

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func (s *Server) registerAssets(g *gin.RouterGroup) {
	g.GET("/assets", s.searchAssets)
	g.GET("/hosts/:ip", s.getHost)
	g.GET("/hosts/:ip/detail", s.getHostDetail)
}

// searchAssets 资产检索（ES 优先，未配置则 PG 回退），支持 page/page_size 标准分页
func (s *Server) searchAssets(c *gin.Context) {
	q := c.Query("q")
	docType := c.Query("type") // host | port | domain | 空
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	res, err := s.deps.Store.SearchAssets(c.Request.Context(), q, docType, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
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
