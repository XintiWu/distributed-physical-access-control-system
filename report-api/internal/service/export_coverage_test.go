package service

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tsmc/report-api/internal/auth"
	"github.com/tsmc/report-api/internal/export"
	"github.com/tsmc/report-api/internal/model"
	"github.com/tsmc/report-api/internal/repository"
)

type mockFullOrgRepo struct {
	GetOrgUnitFn           func(ctx context.Context, id string) (*model.OrgUnit, error)
	GetSubtreeIDsFn        func(ctx context.Context, orgUnitID string) ([]string, error)
	GetEmployeeInfoFn      func(ctx context.Context, employeeID string) (*repository.EmployeeInfo, error)
	IsInSubtreeFn          func(ctx context.Context, req, target string) (bool, error)
	CountActiveEmployeesFn func(ctx context.Context, ids []string) (int, error)
	GetChildUnitsFn        func(ctx context.Context, parentID string) ([]model.OrgUnit, error)
}

func (m mockFullOrgRepo) Ping(ctx context.Context) error { return nil }
func (m mockFullOrgRepo) GetOrgUnit(ctx context.Context, id string) (*model.OrgUnit, error) {
	if m.GetOrgUnitFn != nil {
		return m.GetOrgUnitFn(ctx, id)
	}
	return &model.OrgUnit{ID: id, Name: "Test Org"}, nil
}
func (m mockFullOrgRepo) GetSubtreeIDs(ctx context.Context, orgUnitID string) ([]string, error) {
	if m.GetSubtreeIDsFn != nil {
		return m.GetSubtreeIDsFn(ctx, orgUnitID)
	}
	return []string{orgUnitID}, nil
}
func (m mockFullOrgRepo) GetEmployeeDisplayName(ctx context.Context, employeeID string) (string, error) {
	return "Test Employee", nil
}
func (m mockFullOrgRepo) GetEmployeeInfo(ctx context.Context, employeeID string) (*repository.EmployeeInfo, error) {
	if m.GetEmployeeInfoFn != nil {
		return m.GetEmployeeInfoFn(ctx, employeeID)
	}
	return &repository.EmployeeInfo{OrgUnitID: uuid.New().String(), ReportRole: "TEAM_MANAGER"}, nil
}
func (m mockFullOrgRepo) GetEmployeeOrgUnitID(ctx context.Context, employeeID string) (string, error) {
	return uuid.New().String(), nil
}
func (m mockFullOrgRepo) GetRootOrgUnitID(ctx context.Context) (string, error) {
	return uuid.New().String(), nil
}
func (m mockFullOrgRepo) IsInSubtree(ctx context.Context, req, target string) (bool, error) {
	if m.IsInSubtreeFn != nil {
		return m.IsInSubtreeFn(ctx, req, target)
	}
	return true, nil
}
func (m mockFullOrgRepo) CountActiveEmployees(ctx context.Context, ids []string) (int, error) {
	if m.CountActiveEmployeesFn != nil {
		return m.CountActiveEmployeesFn(ctx, ids)
	}
	return 10, nil
}
func (m mockFullOrgRepo) GetChildUnits(ctx context.Context, parentID string) ([]model.OrgUnit, error) {
	if m.GetChildUnitsFn != nil {
		return m.GetChildUnitsFn(ctx, parentID)
	}
	return nil, nil
}
func (mockFullOrgRepo) Close() error { return nil }

type mockFullInOutRepo struct {
	GetPersonalEventsFn     func(ctx context.Context, employeeID, startDate, endDate string) ([]model.InOutEvent, error)
	GetAuditEventsFn        func(ctx context.Context, f repository.AuditFilter) ([]model.InOutEvent, int, error)
	GetEventsForExportFn    func(ctx context.Context, orgUnitIDs []string, startDate, endDate string) ([]model.InOutEvent, error)
	GetSecurityDenySummaryFn func(ctx context.Context, orgUnitIDs []string, startDate, endDate string) (model.SecurityDenySummary, error)
}

func (m mockFullInOutRepo) GetPersonalEvents(ctx context.Context, employeeID, startDate, endDate string) ([]model.InOutEvent, error) {
	if m.GetPersonalEventsFn != nil {
		return m.GetPersonalEventsFn(ctx, employeeID, startDate, endDate)
	}
	return nil, nil
}
func (m mockFullInOutRepo) GetAuditEvents(ctx context.Context, f repository.AuditFilter) ([]model.InOutEvent, int, error) {
	if m.GetAuditEventsFn != nil {
		return m.GetAuditEventsFn(ctx, f)
	}
	return nil, 0, nil
}
func (m mockFullInOutRepo) GetEventsForExport(ctx context.Context, orgUnitIDs []string, startDate, endDate string) ([]model.InOutEvent, error) {
	if m.GetEventsForExportFn != nil {
		return m.GetEventsForExportFn(ctx, orgUnitIDs, startDate, endDate)
	}
	return nil, nil
}
func (m mockFullInOutRepo) GetSecurityDenySummary(ctx context.Context, orgUnitIDs []string, startDate, endDate string) (model.SecurityDenySummary, error) {
	if m.GetSecurityDenySummaryFn != nil {
		return m.GetSecurityDenySummaryFn(ctx, orgUnitIDs, startDate, endDate)
	}
	return model.SecurityDenySummary{}, nil
}
func (m mockFullInOutRepo) GetEmployeeReportRows(ctx context.Context, orgUnitIDs []string, startDate, endDate string) ([]model.EmployeeReportRow, error) {
	return nil, nil
}
func (mockFullInOutRepo) Close() error { return nil }

type mockFullReportRepo struct {
	GetAggregatedFn func(ctx context.Context, orgUnitIDs []string, startDate, endDate string) ([]model.AggregatedRow, error)
	GetSummaryFn    func(ctx context.Context, orgUnitIDs []string, startDate, endDate string) (model.DepartmentSummary, error)
}

func (m mockFullReportRepo) GetAggregated(ctx context.Context, orgUnitIDs []string, startDate, endDate string) ([]model.AggregatedRow, error) {
	if m.GetAggregatedFn != nil {
		return m.GetAggregatedFn(ctx, orgUnitIDs, startDate, endDate)
	}
	return nil, nil
}
func (m mockFullReportRepo) GetSummary(ctx context.Context, orgUnitIDs []string, startDate, endDate string) (model.DepartmentSummary, error) {
	if m.GetSummaryFn != nil {
		return m.GetSummaryFn(ctx, orgUnitIDs, startDate, endDate)
	}
	return model.DepartmentSummary{}, nil
}
func (mockFullReportRepo) GetOnSiteCount(ctx context.Context, orgUnitIDs []string) (int, error) {
	return 0, nil
}
func (mockFullReportRepo) GetDoorHeatmap(ctx context.Context, orgUnitIDs []string, minutes int) ([]repository.DoorHeatmapRow, error) {
	return nil, nil
}
func (mockFullReportRepo) GetAttendanceTrends(ctx context.Context, orgUnitIDs []string, startDate, endDate string) ([]repository.PeriodAttendanceMetrics, error) {
	return nil, nil
}
func (mockFullReportRepo) GetPassbackDenyCountsLastMinute(ctx context.Context) ([]repository.PassbackDenyRow, error) {
	return nil, nil
}
func (mockFullReportRepo) Close() error { return nil }

func TestBuildExportDocument_PersonalFailingGetPersonalReport(t *testing.T) {
	inout := mockFullInOutRepo{
		GetPersonalEventsFn: func(ctx context.Context, id, start, end string) ([]model.InOutEvent, error) {
			return nil, errors.New("personal db error")
		},
	}
	svc := NewReportService(mockFullOrgRepo{}, mockFullReportRepo{}, inout, nil, nil)
	_, err := svc.BuildExportDocument(context.Background(), model.ExportRequest{Type: "personal"}, uuid.New().String(), uuid.New().String(), auth.RoleTeamManager)
	if err == nil || !strings.Contains(err.Error(), "personal db error") {
		t.Errorf("expected personal db error, got %v", err)
	}
}

func TestBuildExportDocument_DepartmentAccessDeniedRole(t *testing.T) {
	svc := NewReportService(mockFullOrgRepo{}, mockFullReportRepo{}, mockFullInOutRepo{}, nil, nil)
	_, err := svc.BuildExportDocument(context.Background(), model.ExportRequest{Type: "department"}, uuid.New().String(), uuid.New().String(), auth.RoleEmployee)
	if !errors.Is(err, ErrAccessDenied) {
		t.Errorf("expected ErrAccessDenied, got %v", err)
	}
}

func TestBuildExportDocument_DepartmentMissingOrgUnitID(t *testing.T) {
	svc := NewReportService(mockFullOrgRepo{}, mockFullReportRepo{}, mockFullInOutRepo{}, nil, nil)
	_, err := svc.BuildExportDocument(context.Background(), model.ExportRequest{Type: "department"}, uuid.New().String(), uuid.New().String(), auth.RoleTeamManager)
	if err == nil || err.Error() != "orgUnitId is required for department export" {
		t.Errorf("expected missing orgUnitId error, got %v", err)
	}
}

func TestBuildExportDocument_DepartmentSuccess(t *testing.T) {
	orgID := uuid.New().String()
	org := mockFullOrgRepo{
		IsInSubtreeFn: func(ctx context.Context, req, target string) (bool, error) {
			return true, nil
		},
	}
	report := mockFullReportRepo{
		GetAggregatedFn: func(ctx context.Context, ids []string, start, end string) ([]model.AggregatedRow, error) {
			return []model.AggregatedRow{{OrgUnitID: "org1", ReportDate: "2026-05-01", TotalEntries: 10, TotalExits: 10}}, nil
		},
	}
	svc := NewReportService(org, report, mockFullInOutRepo{}, nil, nil)
	doc, err := svc.BuildExportDocument(context.Background(), model.ExportRequest{Type: "department", OrgUnitID: orgID, StartDate: "2026-05-01", EndDate: "2026-05-02"}, uuid.New().String(), orgID, auth.RoleTeamManager)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if doc.Title == "" {
		t.Error("expected non-empty department document title")
	}
}

func TestBuildExportDocument_EventsMissingOrgUnitID(t *testing.T) {
	svc := NewReportService(mockFullOrgRepo{}, mockFullReportRepo{}, mockFullInOutRepo{}, nil, nil)
	_, err := svc.BuildExportDocument(context.Background(), model.ExportRequest{Type: "events"}, uuid.New().String(), uuid.New().String(), auth.RoleTeamManager)
	if err == nil || err.Error() != "orgUnitId is required for events export" {
		t.Errorf("expected missing orgUnitId error, got %v", err)
	}
}

func TestBuildExportDocument_EventsIsInSubtreeError(t *testing.T) {
	org := mockFullOrgRepo{
		IsInSubtreeFn: func(ctx context.Context, req, target string) (bool, error) {
			return false, errors.New("subtree check failed")
		},
	}
	svc := NewReportService(org, mockFullReportRepo{}, mockFullInOutRepo{}, nil, nil)
	_, err := svc.BuildExportDocument(context.Background(), model.ExportRequest{Type: "events", OrgUnitID: uuid.New().String()}, uuid.New().String(), uuid.New().String(), auth.RoleTeamManager)
	if err == nil || !strings.Contains(err.Error(), "subtree check failed") {
		t.Errorf("expected subtree check failed, got %v", err)
	}
}

func TestBuildExportDocument_EventsTargetNotInSubtree(t *testing.T) {
	org := mockFullOrgRepo{
		IsInSubtreeFn: func(ctx context.Context, req, target string) (bool, error) {
			return false, nil
		},
	}
	svc := NewReportService(org, mockFullReportRepo{}, mockFullInOutRepo{}, nil, nil)
	_, err := svc.BuildExportDocument(context.Background(), model.ExportRequest{Type: "events", OrgUnitID: uuid.New().String()}, uuid.New().String(), uuid.New().String(), auth.RoleTeamManager)
	if !errors.Is(err, ErrAccessDenied) {
		t.Errorf("expected ErrAccessDenied, got %v", err)
	}
}

func TestBuildExportDocument_EventsOrgUnitNameErrorOrNil(t *testing.T) {
	org := mockFullOrgRepo{
		GetOrgUnitFn: func(ctx context.Context, id string) (*model.OrgUnit, error) {
			return nil, errors.New("org error")
		},
	}
	svc := NewReportService(org, mockFullReportRepo{}, mockFullInOutRepo{}, nil, nil)
	_, err := svc.BuildExportDocument(context.Background(), model.ExportRequest{Type: "events", OrgUnitID: uuid.New().String()}, uuid.New().String(), uuid.New().String(), auth.RoleTeamManager)
	if err == nil || !strings.Contains(err.Error(), "org unit not found") {
		t.Errorf("expected org unit not found, got %v", err)
	}
}

func TestBuildExportDocument_EventsGetSubtreeIDsError(t *testing.T) {
	org := mockFullOrgRepo{
		GetSubtreeIDsFn: func(ctx context.Context, id string) ([]string, error) {
			return nil, errors.New("subtree ids error")
		},
	}
	svc := NewReportService(org, mockFullReportRepo{}, mockFullInOutRepo{}, nil, nil)
	_, err := svc.BuildExportDocument(context.Background(), model.ExportRequest{Type: "events", OrgUnitID: uuid.New().String()}, uuid.New().String(), uuid.New().String(), auth.RoleTeamManager)
	if err == nil || err.Error() != "subtree ids error" {
		t.Errorf("expected subtree ids error, got %v", err)
	}
}

func TestBuildExportDocument_EventsGetEventsForExportError(t *testing.T) {
	inout := mockFullInOutRepo{
		GetEventsForExportFn: func(ctx context.Context, ids []string, start, end string) ([]model.InOutEvent, error) {
			return nil, errors.New("events db error")
		},
	}
	svc := NewReportService(mockFullOrgRepo{}, mockFullReportRepo{}, inout, nil, nil)
	_, err := svc.BuildExportDocument(context.Background(), model.ExportRequest{Type: "events", OrgUnitID: uuid.New().String()}, uuid.New().String(), uuid.New().String(), auth.RoleTeamManager)
	if err == nil || err.Error() != "events db error" {
		t.Errorf("expected events db error, got %v", err)
	}
}

func TestExportSync_VisualPDFPath(t *testing.T) {
	orgID := uuid.New().String()
	org := mockFullOrgRepo{
		IsInSubtreeFn: func(ctx context.Context, req, target string) (bool, error) {
			return true, nil
		},
	}
	report := mockFullReportRepo{
		GetSummaryFn: func(ctx context.Context, ids []string, start, end string) (model.DepartmentSummary, error) {
			return model.DepartmentSummary{TotalEntries: 10, TotalExits: 10}, nil
		},
	}
	inout := mockFullInOutRepo{
		GetSecurityDenySummaryFn: func(ctx context.Context, ids []string, start, end string) (model.SecurityDenySummary, error) {
			return model.SecurityDenySummary{}, nil
		},
	}
	svc := NewReportService(org, report, inout, nil, nil)
	req := model.ExportRequest{Type: "department", Format: "pdf", OrgUnitID: orgID, StartDate: "2026-05-01", EndDate: "2026-05-02"}
	data, ext, err := svc.ExportSync(context.Background(), req, uuid.New().String(), orgID, auth.RoleTeamManager)
	if err != nil {
		t.Fatalf("ExportSync failed: %v", err)
	}
	if ext != ".pdf" {
		t.Errorf("expected extension .pdf, got %s", ext)
	}
	if len(data) == 0 {
		t.Error("expected non-empty visual report PDF data")
	}
}

func TestRunExportJob_NilJobsStoreReturnsImmediately(t *testing.T) {
	svc := NewReportService(mockFullOrgRepo{}, mockFullReportRepo{}, mockFullInOutRepo{}, nil, nil)
	svc.RunExportJob("dummy-job", model.ExportRequest{}, uuid.New().String(), uuid.New().String(), auth.RoleTeamManager)
}

func TestRunExportJob_SuccessAndFailurePaths(t *testing.T) {
	tmp := t.TempDir()
	store, err := export.NewJobStore(tmp)
	if err != nil {
		t.Fatal(err)
	}

	orgID := uuid.New().String()
	mockOrg := mockFullOrgRepo{
		IsInSubtreeFn: func(ctx context.Context, req, target string) (bool, error) {
			return true, nil
		},
	}
	mockReport := mockFullReportRepo{
		GetSummaryFn: func(ctx context.Context, ids []string, start, end string) (model.DepartmentSummary, error) {
			return model.DepartmentSummary{TotalEntries: 100, TotalExits: 100}, nil
		},
	}
	svcVisual := NewReportService(mockOrg, mockReport, mockFullInOutRepo{}, nil, store)
	jobID1 := store.Create("pdf", "department")
	req1 := model.ExportRequest{Type: "department", Format: "pdf", OrgUnitID: orgID, StartDate: "2026-05-01", EndDate: "2026-05-02"}
	svcVisual.RunExportJob(jobID1, req1, uuid.New().String(), orgID, auth.RoleTeamManager)

	for i := 0; i < 20; i++ {
		if j, ok := store.Get(jobID1); ok && j.Status != export.JobPending {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if j, ok := store.Get(jobID1); !ok || j.Status != export.JobDone {
		t.Errorf("expected job1 to be JobDone, got %v", j)
	}

	jobID2 := store.Create("csv", "events")
	req2 := model.ExportRequest{Type: "events", Format: "csv", OrgUnitID: ""}
	svcVisual.RunExportJob(jobID2, req2, uuid.New().String(), orgID, auth.RoleTeamManager)

	for i := 0; i < 20; i++ {
		if j, ok := store.Get(jobID2); ok && j.Status != export.JobPending {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if j, ok := store.Get(jobID2); !ok || j.Status != export.JobFailed {
		t.Errorf("expected job2 to be JobFailed, got %v", j)
	}
}

func TestRunExportJob_PDFRenderFailure(t *testing.T) {
	tmp := t.TempDir()
	store, err := export.NewJobStore(tmp)
	if err != nil {
		t.Fatal(err)
	}

	orgID := uuid.New().String()
	mockOrg := mockFullOrgRepo{
		IsInSubtreeFn: func(ctx context.Context, req, target string) (bool, error) {
			return false, errors.New("subtree check error to trigger failure")
		},
	}
	svcVisual := NewReportService(mockOrg, mockFullReportRepo{}, mockFullInOutRepo{}, nil, store)

	// requesting department PDF triggers ExportDepartmentVisualPDF which fails due to mockOrg IsInSubtreeFn error
	jobID := store.Create("pdf", "department")
	req := model.ExportRequest{Type: "department", Format: "pdf", OrgUnitID: orgID}
	svcVisual.RunExportJob(jobID, req, uuid.New().String(), orgID, auth.RoleTeamManager)

	for i := 0; i < 20; i++ {
		if j, ok := store.Get(jobID); ok && j.Status != export.JobPending {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if j, ok := store.Get(jobID); !ok || j.Status != export.JobFailed {
		t.Errorf("expected job to be JobFailed due to PDF render failure, got %v", j)
	}
}

func TestRunExportJob_WriteFailure(t *testing.T) {
	tmp := t.TempDir()
	store, err := export.NewJobStore(tmp)
	if err != nil {
		t.Fatal(err)
	}

	orgID := uuid.New().String()
	mockOrg := mockFullOrgRepo{
		IsInSubtreeFn: func(ctx context.Context, req, target string) (bool, error) {
			return true, nil
		},
	}
	svcVisual := NewReportService(mockOrg, mockFullReportRepo{}, mockFullInOutRepo{}, nil, store)

	// Make the directory non-writable
	if err := os.Chmod(tmp, 0000); err != nil {
		t.Skip("skipping write failure test: failed to chmod tmp dir")
	}
	defer func() {
		if err := os.Chmod(tmp, 0o755); err != nil {
			t.Logf("failed to restore permissions for %s: %v", tmp, err)
		}
	}() // restore so cleanup doesn't fail

	jobID := store.Create("csv", "events")
	req := model.ExportRequest{Type: "events", Format: "csv", OrgUnitID: orgID}
	svcVisual.RunExportJob(jobID, req, uuid.New().String(), orgID, auth.RoleTeamManager)

	for i := 0; i < 20; i++ {
		if j, ok := store.Get(jobID); ok && j.Status != export.JobPending {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if j, ok := store.Get(jobID); !ok || j.Status != export.JobFailed {
		t.Errorf("expected job to be JobFailed due to write failure, got %v", j)
	}
}

func TestExportCSV_EmployeeAccessDenied(t *testing.T) {
	svc := NewReportService(mockFullOrgRepo{}, mockFullReportRepo{}, mockFullInOutRepo{}, nil, nil)
	_, err := svc.ExportCSV(context.Background(), model.ExportRequest{}, uuid.New().String(), auth.RoleEmployee)
	if !errors.Is(err, ErrAccessDenied) {
		t.Errorf("expected ErrAccessDenied, got %v", err)
	}
}

func TestExportCSV_IsInSubtreeError(t *testing.T) {
	org := mockFullOrgRepo{
		IsInSubtreeFn: func(ctx context.Context, req, target string) (bool, error) {
			return false, errors.New("subtree db error")
		},
	}
	svc := NewReportService(org, mockFullReportRepo{}, mockFullInOutRepo{}, nil, nil)
	_, err := svc.ExportCSV(context.Background(), model.ExportRequest{OrgUnitID: uuid.New().String()}, uuid.New().String(), auth.RoleTeamManager)
	if err == nil || !strings.Contains(err.Error(), "subtree db error") {
		t.Errorf("expected subtree db error, got %v", err)
	}
}

func TestExportCSV_TargetNotInSubtree(t *testing.T) {
	org := mockFullOrgRepo{
		IsInSubtreeFn: func(ctx context.Context, req, target string) (bool, error) {
			return false, nil
		},
	}
	svc := NewReportService(org, mockFullReportRepo{}, mockFullInOutRepo{}, nil, nil)
	_, err := svc.ExportCSV(context.Background(), model.ExportRequest{OrgUnitID: uuid.New().String()}, uuid.New().String(), auth.RoleTeamManager)
	if !errors.Is(err, ErrAccessDenied) {
		t.Errorf("expected ErrAccessDenied, got %v", err)
	}
}

func TestExportCSV_GetSubtreeIDsError(t *testing.T) {
	org := mockFullOrgRepo{
		IsInSubtreeFn: func(ctx context.Context, req, target string) (bool, error) {
			return true, nil
		},
		GetSubtreeIDsFn: func(ctx context.Context, id string) ([]string, error) {
			return nil, errors.New("subtree ids db error")
		},
	}
	svc := NewReportService(org, mockFullReportRepo{}, mockFullInOutRepo{}, nil, nil)
	_, err := svc.ExportCSV(context.Background(), model.ExportRequest{OrgUnitID: uuid.New().String()}, uuid.New().String(), auth.RoleTeamManager)
	if err == nil || !strings.Contains(err.Error(), "subtree ids db error") {
		t.Errorf("expected subtree ids db error, got %v", err)
	}
}

func TestExportCSV_GetEventsForExportError(t *testing.T) {
	org := mockFullOrgRepo{
		IsInSubtreeFn: func(ctx context.Context, req, target string) (bool, error) {
			return true, nil
		},
	}
	inout := mockFullInOutRepo{
		GetEventsForExportFn: func(ctx context.Context, ids []string, start, end string) ([]model.InOutEvent, error) {
			return nil, errors.New("export events db error")
		},
	}
	svc := NewReportService(org, mockFullReportRepo{}, inout, nil, nil)
	_, err := svc.ExportCSV(context.Background(), model.ExportRequest{OrgUnitID: uuid.New().String()}, uuid.New().String(), auth.RoleTeamManager)
	if err == nil || !strings.Contains(err.Error(), "export events db error") {
		t.Errorf("expected export events db error, got %v", err)
	}
}

func TestExportCSV_Success(t *testing.T) {
	orgID := uuid.New().String()
	org := mockFullOrgRepo{
		IsInSubtreeFn: func(ctx context.Context, req, target string) (bool, error) {
			return true, nil
		},
	}
	reasonText := "Passback error"
	inout := mockFullInOutRepo{
		GetEventsForExportFn: func(ctx context.Context, ids []string, start, end string) ([]model.InOutEvent, error) {
			return []model.InOutEvent{
				{
					EventID:    "evt1",
					EmployeeID: "emp1",
					DoorID:     "door1",
					Direction:  "IN",
					EventTime:  time.Now(),
					Status:     "DENIED",
					Reason:     &reasonText,
					SourceIP:   "127.0.0.1",
				},
			}, nil
		},
	}
	svc := NewReportService(org, mockFullReportRepo{}, inout, nil, nil)
	reader, err := svc.ExportCSV(context.Background(), model.ExportRequest{OrgUnitID: orgID, StartDate: "2026-05-01", EndDate: "2026-05-02"}, orgID, auth.RoleTeamManager)
	if err != nil {
		t.Fatalf("ExportCSV failed: %v", err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "EventID,EmployeeID,DoorID,Direction,EventTime,Status,Reason,SourceIP") {
		t.Error("expected CSV header in output")
	}
	if !strings.Contains(content, "evt1,emp1,door1,IN,") {
		t.Error("expected event row in output")
	}
	if !strings.Contains(content, "Passback error") {
		t.Error("expected reason in output")
	}
}

func TestReportService_JobsStoreGetter(t *testing.T) {
	store, _ := export.NewJobStore(t.TempDir())
	svc := NewReportService(mockFullOrgRepo{}, mockFullReportRepo{}, mockFullInOutRepo{}, nil, store)
	if svc.Jobs() != store {
		t.Errorf("Jobs() returned unexpected store")
	}
}
