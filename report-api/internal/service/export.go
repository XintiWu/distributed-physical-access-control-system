package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/tsmc/report-api/internal/auth"
	"github.com/tsmc/report-api/internal/export"
	"github.com/tsmc/report-api/internal/model"
)

const errExportJobFailed = "export job failed due to internal error"

// BuildExportDocument loads data and builds a layout document for the given export type.
func (s *ReportService) BuildExportDocument(ctx context.Context, req model.ExportRequest, userID, requesterOrgUnitID string, role auth.ReportRole) (export.Document, error) {
	reportType := req.Type
	if reportType == "" {
		reportType = "events"
	}

	switch reportType {
	case "personal":
		resp, err := s.GetPersonalReport(ctx, userID, req.StartDate, req.EndDate)
		if err != nil {
			return export.Document{}, err
		}
		return export.PersonalDocument(resp), nil

	case "department":
		if !role.CanViewDepartmentReports() {
			return export.Document{}, NewAccessDeniedError(fmt.Sprintf("role %s cannot export department reports", role))
		}
		if req.OrgUnitID == "" {
			return export.Document{}, fmt.Errorf("orgUnitId is required for department export")
		}
		deptReq := model.DepartmentReportRequest{
			OrgUnitID: req.OrgUnitID, StartDate: req.StartDate, EndDate: req.EndDate,
			Granularity: req.Granularity,
		}
		resp, err := s.GetDepartmentReport(ctx, deptReq, requesterOrgUnitID)
		if err != nil {
			return export.Document{}, err
		}
		return export.DepartmentDocument(resp), nil

	default: // events
		if !role.CanViewDepartmentReports() {
			return export.Document{}, NewAccessDeniedError(fmt.Sprintf("role %s cannot export event logs", role))
		}
		if req.OrgUnitID == "" {
			return export.Document{}, fmt.Errorf("orgUnitId is required for events export")
		}
		inSubtree, err := s.orgRepo.IsInSubtree(ctx, requesterOrgUnitID, req.OrgUnitID)
		if err != nil {
			return export.Document{}, fmt.Errorf("check subtree: %w", err)
		}
		if !inSubtree {
			return export.Document{}, NewAccessDeniedError(fmt.Sprintf("orgUnitId %s is not in your subtree", req.OrgUnitID))
		}
		orgUnit, err := s.orgRepo.GetOrgUnit(ctx, req.OrgUnitID)
		if err != nil || orgUnit == nil {
			return export.Document{}, fmt.Errorf("org unit not found: %s", req.OrgUnitID)
		}
		subtreeIDs, err := s.orgRepo.GetSubtreeIDs(ctx, req.OrgUnitID)
		if err != nil {
			return export.Document{}, err
		}
		events, err := s.inoutRepo.GetEventsForExport(ctx, subtreeIDs, req.StartDate, req.EndDate)
		if err != nil {
			return export.Document{}, err
		}
		name := orgUnit.Name
		return export.EventsDocument(name, req.OrgUnitID, req.StartDate, req.EndDate, events), nil
	}
}

// RenderExport produces file bytes for csv or pdf format.
func RenderExport(doc export.Document, format string) ([]byte, string, error) {
	switch format {
	case "pdf":
		b, err := export.WritePDF(doc)
		return b, ".pdf", err
	default:
		b, err := export.WriteCSV(doc)
		return b, ".csv", err
	}
}

// ExportSync builds and renders an export (used by GET /reports/export).
func (s *ReportService) ExportSync(ctx context.Context, req model.ExportRequest, userID, requesterOrgUnitID string, role auth.ReportRole) ([]byte, string, error) {
	reportType := req.Type
	if reportType == "" {
		reportType = "events"
	}
	if req.Format == "pdf" && reportType == "department" {
		data, err := s.ExportDepartmentVisualPDF(ctx, req, userID, requesterOrgUnitID, role)
		if err != nil {
			return nil, "", err
		}
		return data, ".pdf", nil
	}
	doc, err := s.BuildExportDocument(ctx, req, userID, requesterOrgUnitID, role)
	if err != nil {
		return nil, "", err
	}
	return RenderExport(doc, req.Format)
}

// RunExportJob generates a file asynchronously and updates the job store.
func (s *ReportService) RunExportJob(jobID string, req model.ExportRequest, userID, requesterOrgUnitID string, role auth.ReportRole) {
	if s.jobs == nil {
		return
	}
	go func() {
		ctx := context.Background()
		var data []byte
		var ext string
		var err error
		reportType := req.Type
		if reportType == "" {
			reportType = "events"
		}
		if req.Format == "pdf" && reportType == "department" {
			data, err = s.ExportDepartmentVisualPDF(ctx, req, userID, requesterOrgUnitID, role)
			ext = ".pdf"
		} else {
			doc, derr := s.BuildExportDocument(ctx, req, userID, requesterOrgUnitID, role)
			if derr != nil {
				slog.Error("export document build failed", "jobId", jobID, "error", derr)
				s.jobs.MarkFailed(jobID, errExportJobFailed)
				return
			}
			data, ext, err = RenderExport(doc, req.Format)
		}
		if err != nil {
			slog.Error("export render failed", "jobId", jobID, "error", err)
			s.jobs.MarkFailed(jobID, errExportJobFailed)
			return
		}
		path := s.jobs.FilePath(jobID, ext)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			slog.Error("export write file failed", "jobId", jobID, "error", err)
			s.jobs.MarkFailed(jobID, errExportJobFailed)
			return
		}
		s.jobs.MarkDone(jobID)
	}()
}
