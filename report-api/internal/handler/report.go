package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/tsmc/report-api/internal/auth"
	"github.com/tsmc/report-api/internal/export"
	"github.com/tsmc/report-api/internal/model"
	"github.com/tsmc/report-api/internal/repository"
	"github.com/tsmc/report-api/internal/service"
)

const (
	errInvalidQueryParams = "invalid query parameters"
	errInternal           = "internal error"
	errInternalServer     = "internal server error"
	errAccessDenied       = "access denied"
)

var (
	requestTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "report_api_requests_total",
		Help: "Total HTTP requests by endpoint and status",
	}, []string{"endpoint", "status"})

	requestLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "report_api_request_duration_ms",
		Help:    "Request latency in milliseconds by endpoint",
		Buckets: prometheus.ExponentialBuckets(1, 2, 14), // 1ms → ~16s
	}, []string{"endpoint"})
)

// ReportHandler handles all report-related HTTP endpoints.
type ReportHandler struct {
	svc     *service.ReportService
	orgRepo repository.OrgRepository
}

// NewReportHandler creates a new handler with service and org repository.
func NewReportHandler(svc *service.ReportService, orgRepo repository.OrgRepository) *ReportHandler {
	return &ReportHandler{svc: svc, orgRepo: orgRepo}
}

// resolveRequester extracts user identity, org unit, and report role from X-User-ID.
func (h *ReportHandler) resolveRequester(c *gin.Context) (userID, orgUnitID string, role auth.ReportRole, ok bool) {
	userID = c.GetHeader("X-User-ID")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing X-User-ID header"})
		return "", "", "", false
	}
	if _, err := uuid.Parse(userID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid X-User-ID"})
		return "", "", "", false
	}

	info, err := h.orgRepo.GetEmployeeInfo(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database unavailable"})
		return "", "", "", false
	}
	if info == nil || info.OrgUnitID == "" {
		c.JSON(http.StatusForbidden, gin.H{"error": "employee has no org unit assigned"})
		return "", "", "", false
	}
	return userID, info.OrgUnitID, auth.ParseRole(info.ReportRole), true
}

// PersonalReport handles GET /reports/personal
func (h *ReportHandler) PersonalReport(c *gin.Context) {
	start := time.Now()
	defer func() {
		requestLatency.WithLabelValues("personal").Observe(float64(time.Since(start).Milliseconds()))
	}()

	userID, _, _, ok := h.resolveRequester(c)
	if !ok {
		requestTotal.WithLabelValues("personal", "4xx").Inc()
		return
	}

	var req model.PersonalReportRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		requestTotal.WithLabelValues("personal", "400").Inc()
		slog.Warn(errInvalidQueryParams, "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": errInvalidQueryParams})
		return
	}

	resp, err := h.svc.GetPersonalReport(c.Request.Context(), userID, req.StartDate, req.EndDate)
	if err != nil {
		requestTotal.WithLabelValues("personal", "500").Inc()
		slog.Error(errInternal, "endpoint", "personal", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": errInternalServer})
		return
	}

	requestTotal.WithLabelValues("personal", "200").Inc()
	c.JSON(http.StatusOK, resp)
}

// DepartmentReport handles GET /reports/department
func (h *ReportHandler) DepartmentReport(c *gin.Context) {
	start := time.Now()
	defer func() {
		requestLatency.WithLabelValues("department").Observe(float64(time.Since(start).Milliseconds()))
	}()

	_, orgUnitID, role, ok := h.resolveRequester(c)
	if !ok {
		requestTotal.WithLabelValues("department", "4xx").Inc()
		return
	}
	if !role.CanViewDepartmentReports() {
		requestTotal.WithLabelValues("department", "403").Inc()
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied: insufficient report role"})
		return
	}

	var req model.DepartmentReportRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		requestTotal.WithLabelValues("department", "400").Inc()
		slog.Warn(errInvalidQueryParams, "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": errInvalidQueryParams})
		return
	}

	resp, err := h.svc.GetDepartmentReport(c.Request.Context(), req, orgUnitID)
	if err != nil {
		if errors.Is(err, service.ErrAccessDenied) {
			requestTotal.WithLabelValues("department", "403").Inc()
			slog.Warn(errAccessDenied, "error", err)
			c.JSON(http.StatusForbidden, gin.H{"error": errAccessDenied})
			return
		}
		slog.Error(errInternal, "endpoint", "department", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": errInternalServer})
		return
	}

	requestTotal.WithLabelValues("department", "200").Inc()
	c.JSON(http.StatusOK, resp)
}

// AuditLog handles GET /reports/audit
func (h *ReportHandler) AuditLog(c *gin.Context) {
	start := time.Now()
	defer func() {
		requestLatency.WithLabelValues("audit").Observe(float64(time.Since(start).Milliseconds()))
	}()

	userID, orgUnitID, role, ok := h.resolveRequester(c)
	if !ok {
		requestTotal.WithLabelValues("audit", "4xx").Inc()
		return
	}

	var req model.AuditLogRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		requestTotal.WithLabelValues("audit", "400").Inc()
		slog.Warn(errInvalidQueryParams, "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": errInvalidQueryParams})
		return
	}

	resp, err := h.svc.GetAuditLog(c.Request.Context(), req, userID, orgUnitID, role)
	if err != nil {
		requestTotal.WithLabelValues("audit", "500").Inc()
		slog.Error(errInternal, "endpoint", "audit", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": errInternalServer})
		return
	}

	requestTotal.WithLabelValues("audit", "200").Inc()
	c.JSON(http.StatusOK, resp)
}

// Export handles GET /reports/export (sync CSV or PDF download)
func (h *ReportHandler) Export(c *gin.Context) {
	start := time.Now()
	defer func() {
		requestLatency.WithLabelValues("export").Observe(float64(time.Since(start).Milliseconds()))
	}()

	userID, orgUnitID, role, ok := h.resolveRequester(c)
	if !ok {
		requestTotal.WithLabelValues("export", "4xx").Inc()
		return
	}

	var req model.ExportRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		requestTotal.WithLabelValues("export", "400").Inc()
		slog.Warn(errInvalidQueryParams, "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": errInvalidQueryParams})
		return
	}
	reportType := req.Type
	if reportType == "" {
		reportType = "events"
	}
	if reportType != "personal" && !role.CanViewDepartmentReports() {
		requestTotal.WithLabelValues("export", "403").Inc()
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied: insufficient report role"})
		return
	}

	data, ext, err := h.svc.ExportSync(c.Request.Context(), req, userID, orgUnitID, role)
	if err != nil {
		if errors.Is(err, service.ErrAccessDenied) {
			requestTotal.WithLabelValues("export", "403").Inc()
			slog.Warn(errAccessDenied, "error", err)
			c.JSON(http.StatusForbidden, gin.H{"error": errAccessDenied})
			return
		}
		slog.Error(errInternal, "endpoint", "export", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": errInternalServer})
		return
	}

	filename := h.exportFilename(c.Request.Context(), reportType, req, role, req.OrgUnitID, ext)
	if req.Format == "pdf" {
		c.Header("Content-Type", "application/pdf")
	} else {
		c.Header("Content-Type", "text/csv")
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	requestTotal.WithLabelValues("export", "200").Inc()
	if _, err := c.Writer.Write(data); err != nil {
		slog.Error("failed to write export response", "endpoint", "export", "error", err)
	}
}

// ExportJobCreate handles POST /reports/export/jobs (async export)
func (h *ReportHandler) ExportJobCreate(c *gin.Context) {
	start := time.Now()
	defer func() {
		requestLatency.WithLabelValues("export_job").Observe(float64(time.Since(start).Milliseconds()))
	}()

	userID, orgUnitID, role, ok := h.resolveRequester(c)
	if !ok {
		requestTotal.WithLabelValues("export_job", "4xx").Inc()
		return
	}

	var req model.ExportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		requestTotal.WithLabelValues("export_job", "400").Inc()
		slog.Warn("invalid json payload", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json payload"})
		return
	}

	if h.svc.Jobs() == nil {
		requestTotal.WithLabelValues("export_job", "503").Inc()
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "export jobs unavailable"})
		return
	}

	reportType := req.Type
	if reportType == "" {
		reportType = "events"
	}
	if reportType != "personal" && !role.CanViewDepartmentReports() {
		requestTotal.WithLabelValues("export_job", "403").Inc()
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied: insufficient report role"})
		return
	}
	jobID := h.svc.Jobs().Create(req.Format, reportType)
	h.svc.RunExportJob(jobID, req, userID, orgUnitID, role)

	requestTotal.WithLabelValues("export_job", "202").Inc()
	c.JSON(http.StatusAccepted, gin.H{"jobId": jobID, "status": "pending", "format": req.Format, "type": reportType})
}

// ExportJobGet handles GET /reports/export/jobs/:jobId
func (h *ReportHandler) ExportJobGet(c *gin.Context) {
	jobID := c.Param("jobId")
	if _, err := uuid.Parse(jobID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid jobId"})
		return
	}
	store := h.svc.Jobs()
	if store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "export jobs unavailable"})
		return
	}
	job, ok := store.Get(jobID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	switch job.Status {
	case export.JobPending:
		c.JSON(http.StatusAccepted, exportJobResponse(job))
		return
	case export.JobFailed:
		c.JSON(http.StatusInternalServerError, gin.H{
			"jobId":  job.ID,
			"status": job.Status,
			"error":  "export job failed",
		})
		return
	case export.JobDone:
		f, name, err := store.OpenResult(jobID)
		if err != nil {
			slog.Error(errInternal, "endpoint", "export_job_get", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": errInternalServer})
			return
		}
		defer f.Close()
		if job.Format == "pdf" {
			c.Header("Content-Type", "application/pdf")
		} else {
			c.Header("Content-Type", "text/csv")
		}
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", name))
		if _, err := io.Copy(c.Writer, f); err != nil {
			slog.Error("failed to stream export job result", "endpoint", "export_job_get", "error", err)
			return
		}
		return
	}
	c.JSON(http.StatusOK, exportJobResponse(job))
}

func exportJobResponse(job *export.Job) gin.H {
	return gin.H{
		"jobId":     job.ID,
		"status":    job.Status,
		"format":    job.Format,
		"type":      job.Type,
		"createdAt": job.CreatedAt,
	}
}

// DoorHeatmap handles GET /reports/analytics/door-heatmap
func (h *ReportHandler) DoorHeatmap(c *gin.Context) {
	start := time.Now()
	defer func() {
		requestLatency.WithLabelValues("door_heatmap").Observe(float64(time.Since(start).Milliseconds()))
	}()

	_, orgUnitID, role, ok := h.resolveRequester(c)
	if !ok {
		requestTotal.WithLabelValues("door_heatmap", "4xx").Inc()
		return
	}
	var req model.DoorHeatmapRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		requestTotal.WithLabelValues("door_heatmap", "400").Inc()
		slog.Warn(errInvalidQueryParams, "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": errInvalidQueryParams})
		return
	}
	resp, err := h.svc.GetDoorHeatmap(c.Request.Context(), req.OrgUnitID, req.Minutes, orgUnitID, role)
	if err != nil {
		if errors.Is(err, service.ErrAccessDenied) {
			requestTotal.WithLabelValues("door_heatmap", "403").Inc()
			slog.Warn(errAccessDenied, "error", err)
			c.JSON(http.StatusForbidden, gin.H{"error": errAccessDenied})
			return
		}
		slog.Error(errInternal, "endpoint", "door_heatmap", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": errInternalServer})
		return
	}
	requestTotal.WithLabelValues("door_heatmap", "200").Inc()
	c.JSON(http.StatusOK, resp)
}

// AttendanceTrends handles GET /reports/analytics/attendance-trends
func (h *ReportHandler) AttendanceTrends(c *gin.Context) {
	start := time.Now()
	defer func() {
		requestLatency.WithLabelValues("attendance_trends").Observe(float64(time.Since(start).Milliseconds()))
	}()

	_, orgUnitID, role, ok := h.resolveRequester(c)
	if !ok {
		requestTotal.WithLabelValues("attendance_trends", "4xx").Inc()
		return
	}
	var req model.AttendanceTrendsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		requestTotal.WithLabelValues("attendance_trends", "400").Inc()
		slog.Warn(errInvalidQueryParams, "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": errInvalidQueryParams})
		return
	}
	resp, err := h.svc.GetAttendanceTrends(c.Request.Context(), req, orgUnitID, role)
	if err != nil {
		if errors.Is(err, service.ErrAccessDenied) {
			requestTotal.WithLabelValues("attendance_trends", "403").Inc()
			slog.Warn(errAccessDenied, "error", err)
			c.JSON(http.StatusForbidden, gin.H{"error": errAccessDenied})
			return
		}
		slog.Error(errInternal, "endpoint", "attendance_trends", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": errInternalServer})
		return
	}
	requestTotal.WithLabelValues("attendance_trends", "200").Inc()
	c.JSON(http.StatusOK, resp)
}

// WorkforceUtilization handles GET /reports/analytics/workforce-utilization
func (h *ReportHandler) WorkforceUtilization(c *gin.Context) {
	start := time.Now()
	defer func() {
		requestLatency.WithLabelValues("workforce_utilization").Observe(float64(time.Since(start).Milliseconds()))
	}()

	_, orgUnitID, role, ok := h.resolveRequester(c)
	if !ok {
		requestTotal.WithLabelValues("workforce_utilization", "4xx").Inc()
		return
	}
	var req model.WorkforceUtilizationRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		requestTotal.WithLabelValues("workforce_utilization", "400").Inc()
		slog.Warn(errInvalidQueryParams, "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": errInvalidQueryParams})
		return
	}
	resp, err := h.svc.GetWorkforceUtilization(c.Request.Context(), req, orgUnitID, role)
	if err != nil {
		if errors.Is(err, service.ErrAccessDenied) {
			requestTotal.WithLabelValues("workforce_utilization", "403").Inc()
			slog.Warn(errAccessDenied, "error", err)
			c.JSON(http.StatusForbidden, gin.H{"error": errAccessDenied})
			return
		}
		slog.Error(errInternal, "endpoint", "workforce_utilization", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": errInternalServer})
		return
	}
	requestTotal.WithLabelValues("workforce_utilization", "200").Inc()
	c.JSON(http.StatusOK, resp)
}

func sanitizeExportSlug(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ', r == '-', r == '_', r == '.':
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "org"
	}
	return out
}

func (h *ReportHandler) exportFilename(ctx context.Context, reportType string, req model.ExportRequest, role auth.ReportRole, orgUnitID, ext string) string {
	scope := sanitizeExportSlug(string(role))
	if reportType == "personal" {
		scope = scope + "_personal"
	} else if orgUnitID != "" {
		if ou, err := h.orgRepo.GetOrgUnit(ctx, orgUnitID); err == nil && ou != nil && ou.Name != "" {
			scope = scope + "_" + sanitizeExportSlug(ou.Name)
		}
	}
	return fmt.Sprintf("access-report_%s_%s_%s-%s%s", scope, reportType, req.StartDate, req.EndDate, ext)
}

