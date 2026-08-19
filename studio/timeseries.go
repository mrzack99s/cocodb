package studio

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/mrzack99s/cocodb"
)

// handleTimeSeriesList returns the names of series registered in the database.
func (s *Server) handleTimeSeriesList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"series": s.db.ListTimeSeries()})
}

// handleTimeSeriesQuery queries timestamped points by range and exact tags.
func (s *Server) handleTimeSeriesQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	var req struct {
		Series     string            `json:"series"`
		Start      string            `json:"start"`
		End        string            `json:"end"`
		Tags       map[string]string `json:"tags"`
		Limit      int               `json:"limit"`
		Descending bool              `json:"descending"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if strings.TrimSpace(req.Series) == "" {
		s.writeError(w, http.StatusBadRequest, "Series name is required")
		return
	}
	start, err := parseStudioTime(req.Start)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid start time; use RFC3339")
		return
	}
	end, err := parseStudioTime(req.End)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid end time; use RFC3339")
		return
	}
	query := s.db.TimeSeries(req.Series).Query().Range(start, end).Limit(req.Limit)
	for key, value := range req.Tags {
		query.Tag(key, value)
	}
	if req.Descending {
		query.Desc()
	}
	points, err := query.All()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"points": points, "count": len(points)})
}

// handleTimeSeriesWrite appends a point to a series.
func (s *Server) handleTimeSeriesWrite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if s.readOnly {
		s.writeError(w, http.StatusForbidden, "Database is in read-only mode")
		return
	}
	var req struct {
		Series string       `json:"series"`
		Point  cocodb.Point `json:"point"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if strings.TrimSpace(req.Series) == "" {
		s.writeError(w, http.StatusBadRequest, "Series name is required")
		return
	}
	id, err := s.db.TimeSeries(req.Series).Write(req.Point)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

// handleTimeSeriesPrune removes points before a retention cutoff.
func (s *Server) handleTimeSeriesPrune(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if s.readOnly {
		s.writeError(w, http.StatusForbidden, "Database is in read-only mode")
		return
	}
	var req struct {
		Series string `json:"series"`
		Before string `json:"before"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if strings.TrimSpace(req.Series) == "" {
		s.writeError(w, http.StatusBadRequest, "Series name is required")
		return
	}
	before, err := parseStudioTime(req.Before)
	if err != nil || before.IsZero() {
		s.writeError(w, http.StatusBadRequest, "A valid RFC3339 retention cutoff is required")
		return
	}
	removed, err := s.db.TimeSeries(req.Series).PruneBefore(before)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"removed": removed})
}

func parseStudioTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, value)
}
