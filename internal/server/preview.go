package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/guiyumin/vget/internal/core/extractor"
)

func (s *Server) handleDownloadPreview(c *gin.Context) {
	var req DownloadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: 400, Message: "url is required"})
		return
	}
	var playlist *extractor.Playlist
	if inspector, ok := extractor.Match(req.URL).(extractor.PlaylistExtractor); ok {
		var err error
		playlist, err = inspector.ExtractPlaylist(c.Request.Context(), req.URL)
		if err != nil {
			c.JSON(http.StatusBadGateway, Response{Code: 502, Message: err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, Response{Code: 200, Data: gin.H{"playlist": playlist}, Message: "download preview"})
}
