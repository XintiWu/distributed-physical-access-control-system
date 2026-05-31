package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tsmc/report-api/internal/model"
)

type mockOrgRepoForReports struct {
	exportOrgRepo
	isInSubtreeFn func(ctx context.Context, req, target string) (bool, error)
}

func (m mockOrgRepoForReports) IsInSubtree(ctx context.Context, req, target string) (bool, error) {
	if m.isInSubtreeFn != nil {
		return m.isInSubtreeFn(ctx, req, target)
	}
	return m.exportOrgRepo.IsInSubtree(ctx, req, target)
}

type mockInOutRepoForReports struct {
	exportInOutRepo
	getPersonalEventsFn func(ctx context.Context, employeeID, startDate, endDate string) ([]model.InOutEvent, error)
}

func (m mockInOutRepoForReports) GetPersonalEvents(ctx context.Context, employeeID, startDate, endDate string) ([]model.InOutEvent, error) {
	if m.getPersonalEventsFn != nil {
		return m.getPersonalEventsFn(ctx, employeeID, startDate, endDate)
	}
	return nil, nil
}

type mockReportRepoForReports struct {
	exportReportRepo
	getAggregatedFn func(ctx context.Context, orgUnitIDs []string, startDate, endDate string) ([]model.AggregatedRow, error)
	getSummaryFn    func(ctx context.Context, orgUnitIDs []string, startDate, endDate string) (model.DepartmentSummary, error)
}

func (m mockReportRepoForReports) GetAggregated(ctx context.Context, orgUnitIDs []string, startDate, endDate string) ([]model.AggregatedRow, error) {
	if m.getAggregatedFn != nil {
		return m.getAggregatedFn(ctx, orgUnitIDs, startDate, endDate)
	}
	return nil, nil
}

func (m mockReportRepoForReports) GetSummary(ctx context.Context, orgUnitIDs []string, startDate, endDate string) (model.DepartmentSummary, error) {
	if m.getSummaryFn != nil {
		return m.getSummaryFn(ctx, orgUnitIDs, startDate, endDate)
	}
	return model.DepartmentSummary{}, nil
}

func TestGetPersonalReport_Success(t *testing.T) {
	orgRepo := mockOrgRepoForReports{}

	now, _ := time.Parse("2006-01-02", "2026-05-30")
	inoutRepo := mockInOutRepoForReports{
		getPersonalEventsFn: func(ctx context.Context, employeeID, startDate, endDate string) ([]model.InOutEvent, error) {
			return []model.InOutEvent{
				{
					EventID:    "evt-1",
					EmployeeID: employeeID,
					Direction:  "IN",
					EventTime:  now,
					Status:     "SUCCESS",
				},
				{
					EventID:    "evt-2",
					EmployeeID: employeeID,
					Direction:  "OUT",
					EventTime:  now.Add(8 * time.Hour),
					Status:     "SUCCESS",
				},
			}, nil
		},
	}

	svc := NewReportService(orgRepo, exportReportRepo{}, &inoutRepo, nil, nil)

	resp, err := svc.GetPersonalReport(context.Background(), "emp-1", "2026-05-30", "2026-05-30")
	assert.NoError(t, err)
	assert.Equal(t, "emp-1", resp.UserID)
	assert.Equal(t, 1, resp.TotalDays)
	assert.Len(t, resp.DailyRecords, 1)
	assert.Equal(t, 8.0, resp.DailyRecords[0].HoursWorked)
}

func TestGetDepartmentReport_Success(t *testing.T) {
	orgRepo := mockOrgRepoForReports{
		isInSubtreeFn: func(ctx context.Context, req, target string) (bool, error) {
			return true, nil // Authorized
		},
	}

	reportRepo := mockReportRepoForReports{
		getAggregatedFn: func(ctx context.Context, orgUnitIDs []string, startDate, endDate string) ([]model.AggregatedRow, error) {
			return []model.AggregatedRow{
				{
					ReportDate:      "2026-05-30",
					TotalEntries:    5,
					TotalExits:      5,
					UniqueEmployees: 3,
					AvgHours:        8.0,
				},
			}, nil
		},
		getSummaryFn: func(ctx context.Context, orgUnitIDs []string, startDate, endDate string) (model.DepartmentSummary, error) {
			return model.DepartmentSummary{
				TotalEntries: 5,
				TotalExits:   5,
			}, nil
		},
	}

	svc := NewReportService(orgRepo, reportRepo, &exportInOutRepo{}, nil, nil)

	req := model.DepartmentReportRequest{
		OrgUnitID:   "org-1",
		StartDate:   "2026-05-30",
		EndDate:     "2026-05-30",
		Granularity: "daily",
	}

	resp, err := svc.GetDepartmentReport(context.Background(), req, "org-1")
	assert.NoError(t, err)
	assert.Equal(t, "org-1", resp.OrgUnitID)
	assert.Equal(t, 5, resp.Summary.TotalEntries)
	assert.Len(t, resp.Periods, 1)
	assert.Equal(t, 5, resp.Periods[0].TotalEntries)
}

func TestGetDepartmentReport_AccessDenied(t *testing.T) {
	orgRepo := mockOrgRepoForReports{
		isInSubtreeFn: func(ctx context.Context, req, target string) (bool, error) {
			return false, nil // Unauthorized
		},
	}

	svc := NewReportService(orgRepo, exportReportRepo{}, &exportInOutRepo{}, nil, nil)

	req := model.DepartmentReportRequest{
		OrgUnitID:   "org-target",
		StartDate:   "2026-05-30",
		EndDate:     "2026-05-30",
		Granularity: "daily",
	}

	_, err := svc.GetDepartmentReport(context.Background(), req, "org-requester")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrAccessDenied))
}
