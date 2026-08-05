package server

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"atlas/internal/assetstore"
	"atlas/internal/model"

	"github.com/gin-gonic/gin"
)

func (s *Server) registerAssets(g *gin.RouterGroup) {
	g.GET("/assets", s.searchAssets)
	g.DELETE("/assets", s.deleteAsset)
	g.GET("/hosts/:ip", s.getHost)
	g.GET("/hosts/:ip/detail", s.getHostDetail)
	g.DELETE("/hosts/:ip", s.deleteHost)
}

// searchAssets 资产检索（仅 ES），支持 page/page_size 标准分页及 aggregated 聚合模式
func (s *Server) searchAssets(c *gin.Context) {
	q := c.Query("q")
	aggregated := c.Query("aggregated") == "true" || c.Query("aggregated") == "1"
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	res, err := s.deps.Asset.SearchAssets(c.Request.Context(), q, aggregated, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

// getHost 按 IP 查询主机资产
func (s *Server) getHost(c *gin.Context) {
	h, err := s.deps.Asset.GetHost(c.Request.Context(), c.Param("ip"))
	if errors.Is(err, assetstore.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "host not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"host": h})
}

// getHostDetail 主机详情：主机 + 全部端口（含指纹/HTTP）+ 关联漏洞
func (s *Server) getHostDetail(c *gin.Context) {
	ip := c.Param("ip")
	h, ports, err := s.deps.Asset.GetHostDetail(c.Request.Context(), ip)
	if errors.Is(err, assetstore.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "host not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	vulns, _ := s.deps.Store.ListVulnsByHost(c.Request.Context(), ip)
	c.JSON(http.StatusOK, gin.H{"host": h, "ports": ports, "vulns": vulns})
}

// deleteAsset deletes one concrete port or domain asset.
func (s *Server) deleteAsset(c *gin.Context) {
	ip := c.Query("ip")
	domain := c.Query("domain")
	portText := c.Query("port")

	var (
		asset  model.Asset
		target string
	)
	switch {
	case ip != "" && portText != "" && domain == "":
		if net.ParseIP(ip) == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ip"})
			return
		}
		port, err := strconv.Atoi(portText)
		if err != nil || port < 1 || port > 65535 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid port"})
			return
		}
		asset = model.Asset{IP: ip, Port: port}
		target = ip + ":" + strconv.Itoa(port)
	case domain != "" && ip == "" && portText == "":
		if !validAssetDomain(domain) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid domain"})
			return
		}
		asset = model.Asset{Domain: domain}
		target = domain
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "provide either ip and port, or domain"})
		return
	}

	ctx := c.Request.Context()
	if err := s.deps.Asset.Delete(ctx, asset); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if s.deps.Store != nil {
		if asset.Port > 0 {
			ports, err := s.deps.Asset.ListPortsByIP(ctx, asset.IP)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			openCount := 0
			for _, p := range ports {
				if p.State == "open" {
					openCount++
				}
			}
			if err := s.deps.Store.DeletePortMetadata(ctx, asset.IP, asset.Port, openCount, len(ports) == 0); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		} else if err := s.deps.Store.DeleteVulnsByAsset(ctx, asset.Domain); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if s.deps.Audit != nil {
		_ = s.deps.Audit.Log(ctx, operatorFromCtx(c), target, "", "asset.delete")
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func validAssetDomain(domain string) bool {
	if len(domain) > 253 || strings.ContainsAny(domain, "/\\?#%") {
		return false
	}
	return strings.IndexFunc(domain, func(r rune) bool {
		return unicode.IsControl(r) || unicode.IsSpace(r)
	}) == -1
}

// deleteHost deletes every asset and relational record owned by an IP.
func (s *Server) deleteHost(c *gin.Context) {
	ip := c.Param("ip")
	if net.ParseIP(ip) == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ip"})
		return
	}
	ctx := c.Request.Context()
	deleted, err := s.deps.Asset.DeleteHost(ctx, ip)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if s.deps.Store != nil {
		if err := s.deps.Store.DeleteHostMetadata(ctx, ip); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if s.deps.Audit != nil {
		_ = s.deps.Audit.Log(ctx, operatorFromCtx(c), ip, "", "asset.host.delete")
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "deleted": deleted})
}
