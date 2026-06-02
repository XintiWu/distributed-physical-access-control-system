package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/tsmc/report-api/internal/auth"
	"github.com/tsmc/report-api/internal/model"
	"github.com/tsmc/report-api/internal/repository"
)

func TestDateAtStart(t *testing.T) {
	got := dateAtStart("2026-05-01")
	want := "2026-05-01 00:00:00"
	if got != want {
		t.Errorf("dateAtStart(%q) = %q, want %q", "2026-05-01", got, want)
	}
}

func TestDateAtEnd(t *testing.T) {
	got := dateAtEnd("2026-05-31")
	want := "2026-05-31 23:59:59"
	if got != want {
		t.Errorf("dateAtEnd(%q) = %q, want %q", "2026-05-31", got, want)
	}
}

func TestFormatAnomalyNotes_AllZero(t *testing.T) {
	sec := model.SecurityDenySummary{}
	got := formatAnomalyNotes(sec, 0)
	if got != "OK" {
		t.Errorf("formatAnomalyNotes(zero) = %q, want OK", got)
	}
}

func TestFormatAnomalyNotes_OnlyPassback(t *testing.T) {
	sec := model.SecurityDenySummary{AntiPassbackDenies: 3}
	got := formatAnomalyNotes(sec, 0)
	want := "Anti-passback deny: 3"
	if got != want {
		t.Errorf("formatAnomalyNotes = %q, want %q", got, want)
	}
}

func TestFormatAnomalyNotes_OnlyPermission(t *testing.T) {
	sec := model.SecurityDenySummary{PermissionDenied: 5}
	got := formatAnomalyNotes(sec, 0)
	want := "Blacklist/banned swipe: 5"
	if got != want {
		t.Errorf("formatAnomalyNotes = %q, want %q", got, want)
	}
}

func TestFormatAnomalyNotes_OnlyMissingPunch(t *testing.T) {
	sec := model.SecurityDenySummary{}
	got := formatAnomalyNotes(sec, 2)
	want := "Missing OUT punch days: 2"
	if got != want {
		t.Errorf("formatAnomalyNotes = %q, want %q", got, want)
	}
}

func TestFormatAnomalyNotes_Mixed(t *testing.T) {
	sec := model.SecurityDenySummary{
		AntiPassbackDenies: 1,
		PermissionDenied:   2,
	}
	got := formatAnomalyNotes(sec, 3)
	if got == "OK" {
		t.Error("expected non-OK for mixed anomalies")
	}
	if len(got) < 30 {
		t.Errorf("expected longer combined string, got %q", got)
	}
}

// Visual Export Mock Implementations

type mockVisualOrgRepo struct {
	getOrgUnitFn             func(ctx context.Context, id string) (*model.OrgUnit, error)
	getSubtreeIDsFn           func(ctx context.Context, orgUnitID string) ([]string, error)
	getEmployeeDisplayNameFn func(ctx context.Context, employeeID string) (string, error)
	getEmployeeInfoFn         func(ctx context.Context, employeeID string) (*repository.EmployeeInfo, error)
	getEmployeeOrgUnitIDFn   func(ctx context.Context, employeeID string) (string, error)
	getRootOrgUnitIDFn       func(ctx context.Context) (string, error)
	isInSubtreeFn             func(ctx context.Context, req, target string) (bool, error)
	countActiveEmployeesFn    func(ctx context.Context, ids []string) (int, error)
	getChildUnitsFn           func(ctx context.Context, parentID string) ([]model.OrgUnit, error)
}

func (m mockVisualOrgRepo) Ping(ctx context.Context) error { return nil }
func (m mockVisualOrgRepo) GetOrgUnit(ctx context.Context, id string) (*model.OrgUnit, error) {
	if m.getOrgUnitFn != nil {
		return m.getOrgUnitFn(ctx, id)
	}
	return &model.OrgUnit{ID: id, Name: "Mock Org", MaterializedPath: "Mock Path"}, nil
}
func (m mockVisualOrgRepo) GetSubtreeIDs(ctx context.Context, orgUnitID string) ([]string, error) {
	if m.getSubtreeIDsFn != nil {
		return m.getSubtreeIDsFn(ctx, orgUnitID)
	}
	return []string{orgUnitID}, nil
}
func (m mockVisualOrgRepo) GetEmployeeDisplayName(ctx context.Context, employeeID string) (string, error) {
	if m.getEmployeeDisplayNameFn != nil {
		return m.getEmployeeDisplayNameFn(ctx, employeeID)
	}
	return "Mock Employee", nil
}
func (m mockVisualOrgRepo) GetEmployeeInfo(ctx context.Context, employeeID string) (*repository.EmployeeInfo, error) {
	if m.getEmployeeInfoFn != nil {
		return m.getEmployeeInfoFn(ctx, employeeID)
	}
	return &repository.EmployeeInfo{OrgUnitID: uuid.New().String(), ReportRole: "EMPLOYEE"}, nil
}
func (m mockVisualOrgRepo) GetEmployeeOrgUnitID(ctx context.Context, employeeID string) (string, error) {
	if m.getEmployeeOrgUnitIDFn != nil {
		return m.getEmployeeOrgUnitIDFn(ctx, employeeID)
	}
	return uuid.New().String(), nil
}
func (m mockVisualOrgRepo) GetRootOrgUnitID(ctx context.Context) (string, error) {
	if m.getRootOrgUnitIDFn != nil {
		return m.getRootOrgUnitIDFn(ctx)
	}
	return uuid.New().String(), nil
}
func (m mockVisualOrgRepo) IsInSubtree(ctx context.Context, req, target string) (bool, error) {
	if m.isInSubtreeFn != nil {
		return m.isInSubtreeFn(ctx, req, target)
	}
	return true, nil
}
func (m mockVisualOrgRepo) CountActiveEmployees(ctx context.Context, ids []string) (int, error) {
	if m.countActiveEmployeesFn != nil {
		return m.countActiveEmployeesFn(ctx, ids)
	}
	return 10, nil
}
func (m mockVisualOrgRepo) GetChildUnits(ctx context.Context, parentID string) ([]model.OrgUnit, error) {
	if m.getChildUnitsFn != nil {
		return m.getChildUnitsFn(ctx, parentID)
	}
	return nil, nil
}
func (m mockVisualOrgRepo) Close() error { return nil }

type mockVisualInOutRepo struct {
	getPersonalEventsFn     func(ctx context.Context, employeeID, startDate, endDate string) ([]model.InOutEvent, error)
	getSecurityDenySummaryFn func(ctx context.Context, orgUnitIDs []string, startDate, endDate string) (model.SecurityDenySummary, error)
	getEmployeeReportRowsFn  func(ctx context.Context, orgUnitIDs []string, startDate, endDate string) ([]model.EmployeeReportRow, error)
}

func (m mockVisualInOutRepo) GetPersonalEvents(ctx context.Context, employeeID, startDate, endDate string) ([]model.InOutEvent, error) {
	if m.getPersonalEventsFn != nil {
		return m.getPersonalEventsFn(ctx, employeeID, startDate, endDate)
	}
	return nil, nil
}
func (m mockVisualInOutRepo) GetAuditEvents(ctx context.Context, f repository.AuditFilter) ([]model.InOutEvent, int, error) {
	return nil, 0, nil
}
func (m mockVisualInOutRepo) GetEventsForExport(ctx context.Context, orgUnitIDs []string, startDate, endDate string) ([]model.InOutEvent, error) {
	return nil, nil
}
func (m mockVisualInOutRepo) GetSecurityDenySummary(ctx context.Context, orgUnitIDs []string, startDate, endDate string) (model.SecurityDenySummary, error) {
	if m.getSecurityDenySummaryFn != nil {
		return m.getSecurityDenySummaryFn(ctx, orgUnitIDs, startDate, endDate)
	}
	return model.SecurityDenySummary{AntiPassbackDenies: 1}, nil
}
func (m mockVisualInOutRepo) GetEmployeeReportRows(ctx context.Context, orgUnitIDs []string, startDate, endDate string) ([]model.EmployeeReportRow, error) {
	if m.getEmployeeReportRowsFn != nil {
		return m.getEmployeeReportRowsFn(ctx, orgUnitIDs, startDate, endDate)
	}
	return []model.EmployeeReportRow{
		{EmployeeID: "emp-1", TotalSwipes: 10, TotalHours: 45.5, AntiPassbackDenies: 1, MissingPunchDays: 1},
	}, nil
}
func (m mockVisualInOutRepo) Close() error { return nil }

type mockVisualReportRepo struct {
	getSummaryFn           func(ctx context.Context, orgUnitIDs []string, startDate, endDate string) (model.DepartmentSummary, error)
	getDoorHeatmapFn       func(ctx context.Context, orgUnitIDs []string, minutes int) ([]repository.DoorHeatmapRow, error)
	getAttendanceTrendsFn func(ctx context.Context, orgUnitIDs []string, startDate, endDate string) ([]repository.PeriodAttendanceMetrics, error)
}

func (m mockVisualReportRepo) GetAggregated(ctx context.Context, orgUnitIDs []string, startDate, endDate string) ([]model.AggregatedRow, error) {
	return []model.AggregatedRow{
		{ReportDate: "2026-05-01", TotalEntries: 50, UniqueEmployees: 10, AvgHours: 8.2},
	}, nil
}
func (m mockVisualReportRepo) GetSummary(ctx context.Context, orgUnitIDs []string, startDate, endDate string) (model.DepartmentSummary, error) {
	if m.getSummaryFn != nil {
		return m.getSummaryFn(ctx, orgUnitIDs, startDate, endDate)
	}
	return model.DepartmentSummary{
		TotalEntries: 100, TotalExits: 100, UniqueEmployees: 10, Headcount: 12, WorkforceUtilization: 0.83, AvgHoursPerDay: 8.1, LateRate: 0.05,
	}, nil
}
func (m mockVisualReportRepo) GetOnSiteCount(ctx context.Context, orgUnitIDs []string) (int, error) {
	return 5, nil
}
func (m mockVisualReportRepo) GetDoorHeatmap(ctx context.Context, orgUnitIDs []string, minutes int) ([]repository.DoorHeatmapRow, error) {
	if m.getDoorHeatmapFn != nil {
		return m.getDoorHeatmapFn(ctx, orgUnitIDs, minutes)
	}
	return []repository.DoorHeatmapRow{
		{DoorID: "door-1", DoorName: "Front Gate", SwipeCount: 150},
	}, nil
}
func (m mockVisualReportRepo) GetAttendanceTrends(ctx context.Context, orgUnitIDs []string, startDate, endDate string) ([]repository.PeriodAttendanceMetrics, error) {
	if m.getAttendanceTrendsFn != nil {
		return m.getAttendanceTrendsFn(ctx, orgUnitIDs, startDate, endDate)
	}
	return []repository.PeriodAttendanceMetrics{
		{PeriodStart: "2026-05-01 00:00:00", LateRate: 0.85, AvgHours: 8.5, Headcount: 10},
	}, nil
}
func (m mockVisualReportRepo) GetPassbackDenyCountsLastMinute(ctx context.Context) ([]repository.PassbackDenyRow, error) {
	return nil, nil
}
func (m mockVisualReportRepo) Close() error { return nil }

func TestExportDepartmentVisualPDF_Success(t *testing.T) {
	svc := NewReportService(mockVisualOrgRepo{}, mockVisualReportRepo{}, mockVisualInOutRepo{}, nil, nil)
	req := model.ExportRequest{
		OrgUnitID:   uuid.New().String(),
		StartDate:   "2026-05-01",
		EndDate:     "2026-05-30",
		Granularity: "daily",
	}

	data, err := svc.ExportDepartmentVisualPDF(context.Background(), req, uuid.New().String(), uuid.New().String(), auth.RoleTeamManager)
	if err != nil {
		t.Fatalf("ExportDepartmentVisualPDF failed: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty visual PDF data")
	}
}

func TestExportDepartmentVisualPDF_AccessDenied(t *testing.T) {
	svc := NewReportService(mockVisualOrgRepo{}, mockVisualReportRepo{}, mockVisualInOutRepo{}, nil, nil)
	req := model.ExportRequest{
		OrgUnitID: uuid.New().String(),
		StartDate: "2026-05-01",
		EndDate:   "2026-05-30",
	}

	_, err := svc.ExportDepartmentVisualPDF(context.Background(), req, uuid.New().String(), uuid.New().String(), auth.RoleEmployee)
	if !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("expected ErrAccessDenied, got %v", err)
	}
}

func TestExportDepartmentVisualPDF_MissingOrgUnitID(t *testing.T) {
	svc := NewReportService(mockVisualOrgRepo{}, mockVisualReportRepo{}, mockVisualInOutRepo{}, nil, nil)
	req := model.ExportRequest{
		OrgUnitID: "",
		StartDate: "2026-05-01",
		EndDate:   "2026-05-30",
	}

	_, err := svc.ExportDepartmentVisualPDF(context.Background(), req, uuid.New().String(), uuid.New().String(), auth.RoleTeamManager)
	if err == nil {
		t.Error("expected error for missing orgUnitId")
	}
}

func TestExportDepartmentVisualPDF_OrgNotFound(t *testing.T) {
	org := mockVisualOrgRepo{
		getOrgUnitFn: func(ctx context.Context, id string) (*model.OrgUnit, error) {
			return nil, nil
		},
	}
	svc := NewReportService(org, mockVisualReportRepo{}, mockVisualInOutRepo{}, nil, nil)
	req := model.ExportRequest{
		OrgUnitID: uuid.New().String(),
		StartDate: "2026-05-01",
		EndDate:   "2026-05-30",
	}

	_, err := svc.ExportDepartmentVisualPDF(context.Background(), req, uuid.New().String(), uuid.New().String(), auth.RoleTeamManager)
	if err == nil {
		t.Error("expected error when org unit not found")
	}
}

func TestBuildReportDetailRows_Subunits(t *testing.T) {
	org := mockVisualOrgRepo{
		getChildUnitsFn: func(ctx context.Context, parentID string) ([]model.OrgUnit, error) {
			return []model.OrgUnit{{ID: "sub-1", Name: "Sub Unit 1"}}, nil
		},
	}
	svc := NewReportService(org, mockVisualReportRepo{}, mockVisualInOutRepo{}, nil, nil)

	dept := &model.DepartmentReportResponse{
		OrgUnitID: "parent-1",
		SubUnits: []model.SubUnitSummary{
			{OrgUnitID: "sub-1", OrgUnitName: "Sub Unit 1", TotalEntries: 5, TotalExits: 5},
		},
	}

	rows, err := svc.buildReportDetailRows(context.Background(), dept, "2026-05-01", "2026-05-30")
	if err != nil {
		t.Fatalf("buildReportDetailRows error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].EntityID != "sub-1" || rows[0].EntityType != "Department" {
		t.Errorf("unexpected row structure: %+v", rows[0])
	}
}

func TestExportDepartmentVisualPDF_MockDataFallback(t *testing.T) {
	req := model.ExportRequest{
		OrgUnitID: "mock-org-1",
		StartDate: "2026-05-01",
		EndDate:   "2026-05-30",
		Granularity: "daily",
	}
	seedVal := hashCodeGo(req.EndDate + req.OrgUnitID)
	targetEntries := 1000 + (seedVal % 500)

	reportRepo := mockVisualReportRepo{
		getSummaryFn: func(ctx context.Context, orgUnitIDs []string, startDate, endDate string) (model.DepartmentSummary, error) {
			return model.DepartmentSummary{
				TotalEntries: targetEntries,
			}, nil
		},
		getDoorHeatmapFn: func(ctx context.Context, orgUnitIDs []string, minutes int) ([]repository.DoorHeatmapRow, error) {
			return []repository.DoorHeatmapRow{}, nil // Empty to trigger fallback
		},
		getAttendanceTrendsFn: func(ctx context.Context, orgUnitIDs []string, startDate, endDate string) ([]repository.PeriodAttendanceMetrics, error) {
			return []repository.PeriodAttendanceMetrics{}, nil // Empty to trigger fallback
		},
	}
	inoutRepo := mockVisualInOutRepo{
		getSecurityDenySummaryFn: func(ctx context.Context, orgUnitIDs []string, startDate, endDate string) (model.SecurityDenySummary, error) {
			return model.SecurityDenySummary{AntiPassbackDenies: 0, PermissionDenied: 0}, nil
		},
	}

	svc := NewReportService(mockVisualOrgRepo{}, reportRepo, inoutRepo, nil, nil)
	data, err := svc.ExportDepartmentVisualPDF(context.Background(), req, "user-1", "user-org", auth.RoleCEO)
	if err != nil {
		t.Fatalf("ExportDepartmentVisualPDF failed: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty visual PDF data with mocked data")
	}
}

func TestExportDepartmentVisualPDF_GetDepartmentReportError(t *testing.T) {
	org := mockVisualOrgRepo{
		isInSubtreeFn: func(ctx context.Context, req, target string) (bool, error) {
			return false, errors.New("subtree query error")
		},
	}
	svc := NewReportService(org, mockVisualReportRepo{}, mockVisualInOutRepo{}, nil, nil)
	req := model.ExportRequest{
		OrgUnitID: uuid.New().String(),
		StartDate: "2026-05-01",
		EndDate:   "2026-05-30",
	}
	_, err := svc.ExportDepartmentVisualPDF(context.Background(), req, "user-1", "user-org", auth.RoleTeamManager)
	if err == nil {
		t.Error("expected error when GetDepartmentReport fails")
	}
}

func TestExportDepartmentVisualPDF_GetSubtreeIDsError(t *testing.T) {
	org := mockVisualOrgRepo{
		getSubtreeIDsFn: func(ctx context.Context, orgUnitID string) ([]string, error) {
			return nil, errors.New("subtree ids database error")
		},
	}
	svc := NewReportService(org, mockVisualReportRepo{}, mockVisualInOutRepo{}, nil, nil)
	req := model.ExportRequest{
		OrgUnitID: uuid.New().String(),
		StartDate: "2026-05-01",
		EndDate:   "2026-05-30",
	}
	_, err := svc.ExportDepartmentVisualPDF(context.Background(), req, "user-1", "user-org", auth.RoleTeamManager)
	if err == nil {
		t.Error("expected error when GetSubtreeIDs fails")
	}
}

func TestExportDepartmentVisualPDF_GetSecurityDenySummaryError(t *testing.T) {
	inout := mockVisualInOutRepo{
		getSecurityDenySummaryFn: func(ctx context.Context, orgUnitIDs []string, startDate, endDate string) (model.SecurityDenySummary, error) {
			return model.SecurityDenySummary{}, errors.New("security query error")
		},
	}
	svc := NewReportService(mockVisualOrgRepo{}, mockVisualReportRepo{}, inout, nil, nil)
	req := model.ExportRequest{
		OrgUnitID: uuid.New().String(),
		StartDate: "2026-05-01",
		EndDate:   "2026-05-30",
	}
	_, err := svc.ExportDepartmentVisualPDF(context.Background(), req, "user-1", "user-org", auth.RoleTeamManager)
	if err == nil {
		t.Error("expected error when GetSecurityDenySummary fails")
	}
}

func TestExportDepartmentVisualPDF_GetDoorHeatmapError(t *testing.T) {
	report := mockVisualReportRepo{
		getDoorHeatmapFn: func(ctx context.Context, orgUnitIDs []string, minutes int) ([]repository.DoorHeatmapRow, error) {
			return nil, errors.New("door heatmap db error")
		},
	}
	svc := NewReportService(mockVisualOrgRepo{}, report, mockVisualInOutRepo{}, nil, nil)
	req := model.ExportRequest{
		OrgUnitID: uuid.New().String(),
		StartDate: "2026-05-01",
		EndDate:   "2026-05-30",
	}
	_, err := svc.ExportDepartmentVisualPDF(context.Background(), req, "user-1", "user-org", auth.RoleTeamManager)
	if err == nil {
		t.Error("expected error when GetDoorHeatmap fails")
	}
}

func TestExportDepartmentVisualPDF_GetAttendanceTrendsError(t *testing.T) {
	report := mockVisualReportRepo{
		getAttendanceTrendsFn: func(ctx context.Context, orgUnitIDs []string, startDate, endDate string) ([]repository.PeriodAttendanceMetrics, error) {
			return nil, errors.New("attendance trends db error")
		},
	}
	svc := NewReportService(mockVisualOrgRepo{}, report, mockVisualInOutRepo{}, nil, nil)
	req := model.ExportRequest{
		OrgUnitID: uuid.New().String(),
		StartDate: "2026-05-01",
		EndDate:   "2026-05-30",
	}
	_, err := svc.ExportDepartmentVisualPDF(context.Background(), req, "user-1", "user-org", auth.RoleTeamManager)
	if err == nil {
		t.Error("expected error when GetAttendanceTrends fails")
	}
}

func TestBuildReportDetailRows_SubtreeIDsError(t *testing.T) {
	org := mockVisualOrgRepo{
		getSubtreeIDsFn: func(ctx context.Context, orgUnitID string) ([]string, error) {
			return nil, errors.New("subtree ids error")
		},
	}
	svc := NewReportService(org, mockVisualReportRepo{}, mockVisualInOutRepo{}, nil, nil)
	dept := &model.DepartmentReportResponse{
		OrgUnitID: "unit-1",
	}
	_, err := svc.buildReportDetailRows(context.Background(), dept, "2026-05-01", "2026-05-30")
	if err == nil {
		t.Error("expected error when GetSubtreeIDs fails in buildReportDetailRows")
	}
}

func TestBuildReportDetailRows_GetEmployeeReportRowsError(t *testing.T) {
	inout := mockVisualInOutRepo{
		getEmployeeReportRowsFn: func(ctx context.Context, orgUnitIDs []string, startDate, endDate string) ([]model.EmployeeReportRow, error) {
			return nil, errors.New("employee report rows db error")
		},
	}
	svc := NewReportService(mockVisualOrgRepo{}, mockVisualReportRepo{}, inout, nil, nil)
	dept := &model.DepartmentReportResponse{
		OrgUnitID: "unit-1",
	}
	_, err := svc.buildReportDetailRows(context.Background(), dept, "2026-05-01", "2026-05-30")
	if err == nil {
		t.Error("expected error when GetEmployeeReportRows fails")
	}
}

func TestExportDepartmentVisualPDF_ExecutiveRole(t *testing.T) {
	svc := NewReportService(mockVisualOrgRepo{}, mockVisualReportRepo{}, mockVisualInOutRepo{}, nil, nil)
	req := model.ExportRequest{
		OrgUnitID: uuid.New().String(),
		StartDate: "2026-05-01",
		EndDate:   "2026-05-30",
	}
	_, err := svc.ExportDepartmentVisualPDF(context.Background(), req, "user-1", "user-org", auth.RoleCEO)
	if err != nil {
		t.Fatalf("expected success with executive role, got: %v", err)
	}
}

