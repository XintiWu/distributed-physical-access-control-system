package service

import (
	"context"
	"fmt"

	"github.com/tsmc/report-api/internal/auth"
	"github.com/tsmc/report-api/internal/model"
	"github.com/tsmc/report-api/internal/repository"
)

const errOrgUnitNotInSubtree = "orgUnitId %s is not in your subtree"

// GetDoorHeatmap returns real-time door swipe ranking scoped to an org subtree.
func (s *ReportService) GetDoorHeatmap(ctx context.Context, orgUnitID string, minutes int, requesterOrgUnitID string, role auth.ReportRole) (*model.DoorHeatmapResponse, error) {
	if !role.CanViewDepartmentReports() {
		return nil, NewAccessDeniedError(fmt.Sprintf("role %s cannot view door analytics", role))
	}
	if orgUnitID == "" {
		orgUnitID = requesterOrgUnitID
	}
	inSubtree, err := s.orgRepo.IsInSubtree(ctx, requesterOrgUnitID, orgUnitID)
	if err != nil {
		return nil, err
	}
	if !inSubtree {
		return nil, NewAccessDeniedError(fmt.Sprintf(errOrgUnitNotInSubtree, orgUnitID))
	}
	if minutes < 1 {
		minutes = 60
	}
	if minutes > 24*60 {
		minutes = 24 * 60
	}
	subtreeIDs, err := s.orgRepo.GetSubtreeIDs(ctx, orgUnitID)
	if err != nil {
		return nil, fmt.Errorf("get subtree: %w", err)
	}
	rows, err := s.reportRepo.GetDoorHeatmap(ctx, subtreeIDs, minutes)
	if err != nil {
		return nil, fmt.Errorf("door heatmap: %w", err)
	}
	doors := make([]model.DoorTrafficRow, 0, len(rows))
	for _, r := range rows {
		doors = append(doors, model.DoorTrafficRow{
			DoorID: r.DoorID, DoorName: r.DoorName, Site: r.Site, SwipeCount: r.SwipeCount,
		})
	}
	return &model.DoorHeatmapResponse{WindowMinutes: minutes, Doors: doors}, nil
}

// GetAttendanceTrends returns avg hours and late rate series for charts.
func (s *ReportService) GetAttendanceTrends(ctx context.Context, req model.AttendanceTrendsRequest, requesterOrgUnitID string, role auth.ReportRole) (*model.AttendanceTrendsResponse, error) {
	if !role.CanViewDepartmentReports() {
		return nil, NewAccessDeniedError(fmt.Sprintf("role %s cannot view attendance trends", role))
	}
	inSubtree, err := s.orgRepo.IsInSubtree(ctx, requesterOrgUnitID, req.OrgUnitID)
	if err != nil {
		return nil, err
	}
	if !inSubtree {
		return nil, NewAccessDeniedError(fmt.Sprintf(errOrgUnitNotInSubtree, req.OrgUnitID))
	}
	granularity := req.Granularity
	if granularity == "" {
		granularity = "monthly"
	}
	orgUnit, err := s.orgRepo.GetOrgUnit(ctx, req.OrgUnitID)
	if err != nil || orgUnit == nil {
		return nil, fmt.Errorf("org unit not found: %s", req.OrgUnitID)
	}
	subtreeIDs, err := s.orgRepo.GetSubtreeIDs(ctx, req.OrgUnitID)
	if err != nil {
		return nil, fmt.Errorf("get subtree: %w", err)
	}
	daily, err := s.reportRepo.GetAttendanceTrends(ctx, subtreeIDs, req.StartDate, req.EndDate)
	if err != nil {
		return nil, fmt.Errorf("attendance trends: %w", err)
	}
	aggRows := make([]model.AggregatedRow, len(daily))
	for i, d := range daily {
		aggRows[i] = model.AggregatedRow{
			ReportDate: d.PeriodStart,
			AvgHours:   d.AvgHours,
		}
	}
	periods := buildPeriods(aggRows, granularity, req.StartDate, req.EndDate)
	periods = repository.MergeTrendsIntoPeriods(periods, daily, granularity)

	series := make([]model.AttendancePoint, 0, len(periods))
	for _, p := range periods {
		series = append(series, model.AttendancePoint{
			PeriodStart: p.PeriodStart,
			PeriodEnd:   p.PeriodEnd,
			AvgHours:    p.AvgHours,
			LateRate:    p.LateRate,
		})
	}
	return &model.AttendanceTrendsResponse{
		OrgUnitID: req.OrgUnitID, OrgUnitName: orgUnit.Name,
		StartDate: req.StartDate, EndDate: req.EndDate,
		Granularity: granularity, Series: series,
	}, nil
}

// GetWorkforceUtilization returns headcount-based utilization for an org subtree.
func (s *ReportService) GetWorkforceUtilization(ctx context.Context, req model.WorkforceUtilizationRequest, requesterOrgUnitID string, role auth.ReportRole) (*model.WorkforceUtilizationResponse, error) {
	if !role.CanViewDepartmentReports() {
		return nil, NewAccessDeniedError(fmt.Sprintf("role %s cannot view workforce utilization", role))
	}
	inSubtree, err := s.orgRepo.IsInSubtree(ctx, requesterOrgUnitID, req.OrgUnitID)
	if err != nil {
		return nil, err
	}
	if !inSubtree {
		return nil, NewAccessDeniedError(fmt.Sprintf(errOrgUnitNotInSubtree, req.OrgUnitID))
	}
	orgUnit, err := s.orgRepo.GetOrgUnit(ctx, req.OrgUnitID)
	if err != nil || orgUnit == nil {
		return nil, fmt.Errorf("org unit not found: %s", req.OrgUnitID)
	}
	subtreeIDs, err := s.orgRepo.GetSubtreeIDs(ctx, req.OrgUnitID)
	if err != nil {
		return nil, fmt.Errorf("get subtree: %w", err)
	}
	summary, err := s.reportRepo.GetSummary(ctx, subtreeIDs, req.StartDate, req.EndDate)
	if err != nil {
		return nil, fmt.Errorf("get summary: %w", err)
	}
	headcount, err := s.orgRepo.CountActiveEmployees(ctx, subtreeIDs)
	if err != nil {
		return nil, fmt.Errorf("headcount: %w", err)
	}
	onSite, err := s.reportRepo.GetOnSiteCount(ctx, subtreeIDs)
	if err != nil {
		return nil, fmt.Errorf("on-site: %w", err)
	}
	util := calcUtilization(summary.UniqueEmployees, headcount)
	onSiteRate := calcUtilization(onSite, headcount)
	return &model.WorkforceUtilizationResponse{
		OrgUnitID:            req.OrgUnitID,
		OrgUnitName:          orgUnit.Name,
		StartDate:            req.StartDate,
		EndDate:              req.EndDate,
		Headcount:            headcount,
		UniquePresent:        summary.UniqueEmployees,
		WorkforceUtilization: util,
		OnSiteNow:            onSite,
		OnSiteRate:           onSiteRate,
	}, nil
}
