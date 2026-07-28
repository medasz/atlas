package server

import (
	"net/http"

	"atlas/internal/model"

	"github.com/gin-gonic/gin"
)

func (s *Server) registerTasks(g *gin.RouterGroup) {
	g.POST("/tasks", s.createTask)
	g.GET("/tasks", s.listTasks)
	g.GET("/tasks/:id", s.getTask)
	g.POST("/tasks/:id/resume", s.resumeTask)
	g.POST("/tasks/:id/pause", s.pauseTask)
	g.DELETE("/tasks/:id", s.deleteTask)
}

type createTaskReq struct {
	Kind      string         `json:"kind"` // scan | vuln
	Scope     map[string]any `json:"scope"`
	Schedule  map[string]any `json:"schedule"`
	RateLimit map[string]any `json:"rate_limit"`
}

func (s *Server) createTask(c *gin.Context) {
	var req createTaskReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Kind != model.TaskScan && req.Kind != model.TaskVuln {
		c.JSON(http.StatusBadRequest, gin.H{"error": "kind must be scan or vuln"})
		return
	}
	id, err := s.deps.Task.Create(c.Request.Context(), operatorFromCtx(c), req.Kind, req.Scope, req.Schedule, req.RateLimit)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func (s *Server) listTasks(c *gin.Context) {
	tasks, err := s.deps.Store.ListTasks(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": tasks})
}

func (s *Server) getTask(c *gin.Context) {
	t, err := s.deps.Store.GetTask(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	items, _ := s.deps.Store.ListTaskItems(c.Request.Context(), t.ID, nil)
	c.JSON(http.StatusOK, gin.H{"task": t, "items": items})
}

func (s *Server) resumeTask(c *gin.Context) {
	if err := s.deps.Task.Resume(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) pauseTask(c *gin.Context) {
	if err := s.deps.Task.Pause(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) deleteTask(c *gin.Context) {
	id := c.Param("id")
	if _, err := s.deps.Store.GetTask(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	if err := s.deps.Store.DeleteTask(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
