package server

import (
	"net/http"

	"atlas/internal/vuln"

	"github.com/gin-gonic/gin"
)

func (s *Server) registerVuln(g *gin.RouterGroup) {
	g.GET("/vulns", s.listVulns)
	g.GET("/templates", s.listTemplates)
	g.POST("/templates", s.addTemplate)
}

func (s *Server) listVulns(c *gin.Context) {
	items, err := s.deps.Store.ListVulns(c.Request.Context(), c.Query("asset"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (s *Server) listTemplates(c *gin.Context) {
	templates := []gin.H{}
	for _, t := range s.deps.Vuln.Templates() {
		templates = append(templates, gin.H{"id": t.ID, "name": t.Info.Name, "severity": t.Info.Severity, "tags": t.Info.Tags})
	}
	c.JSON(http.StatusOK, gin.H{"items": templates})
}

type addTemplateReq struct {
	Content string `json:"content"`
}

func (s *Server) addTemplate(c *gin.Context) {
	var req addTemplateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	t, err := vuln.ParseTemplate([]byte(req.Content))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	s.deps.Vuln.AddTemplate(t)
	_ = s.deps.Store.UpsertTemplate(c.Request.Context(), t.ID, req.Content)
	c.JSON(http.StatusCreated, gin.H{"ok": true, "id": t.ID})
}
