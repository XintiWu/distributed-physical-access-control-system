package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tsmc/access-api/internal/cache"
	"github.com/tsmc/access-api/internal/model"
	"github.com/tsmc/access-api/internal/service"
)

type mockStateCache struct {
	passback   model.PassbackState
	passbackErr error
	doorStatus  string
	doorErr     error
}

func (m *mockStateCache) GetPassback(_ context.Context, _ string) (model.PassbackState, error) {
	return m.passback, m.passbackErr
}

func (m *mockStateCache) GetDoorStatus(_ context.Context, _ string) (string, error) {
	return m.doorStatus, m.doorErr
}

type mockCacheStore struct {
	denied   map[string]bool
	passback map[string]model.PassbackState
	readErr  error
}

func (m *mockCacheStore) IsDenied(_ context.Context, userID string) (bool, error) {
	if m.readErr != nil {
		return false, m.readErr
	}
	return m.denied[userID], nil
}

func (m *mockCacheStore) GetPassback(_ context.Context, userID string) (model.PassbackState, error) {
	if m.readErr != nil {
		return model.PassbackNone, m.readErr
	}
	if s, ok := m.passback[userID]; ok {
		return s, nil
	}
	return model.PassbackNone, nil
}

func (m *mockCacheStore) SetPassback(_ context.Context, _ string, _ model.PassbackState) error {
	return m.readErr
}

func (m *mockCacheStore) LookupCard(_ context.Context, _ string) (string, error) {
	return "", m.readErr
}

func (m *mockCacheStore) SetCardMapping(_ context.Context, _, _ string) error {
	return m.readErr
}

func (m *mockCacheStore) BatchRead(_ context.Context, cardUID, userID string) (cache.BatchReadResult, error) {
	if m.readErr != nil {
		return cache.BatchReadResult{}, m.readErr
	}
	var result cache.BatchReadResult
	if m.denied != nil {
		result.IsDenied = m.denied[userID]
	}
	if m.passback != nil {
		result.PassbackState = m.passback[userID]
	}
	return result, nil
}

type mockPublisher struct {
	err error
}

func (m *mockPublisher) Publish(_ context.Context, _ model.InOutEvent) error {
	return m.err
}

func (m *mockPublisher) Close() error { return nil }

func TestAccessHandler_Swipe_InvalidJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewAccessHandler(service.NewAccessDecisionService(&mockCacheStore{}), &mockStateCache{}, &mockPublisher{})
	r := gin.New()
	r.POST("/access/swipe", h.Swipe)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/access/swipe", bytes.NewReader([]byte("{bad")))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAccessHandler_Swipe_CacheUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache := &mockCacheStore{readErr: errors.New("redis down")}
	h := NewAccessHandler(service.NewAccessDecisionService(cache), &mockStateCache{}, &mockPublisher{})
	r := gin.New()
	r.POST("/access/swipe", h.Swipe)

	userID := uuid.New().String()
	body, _ := json.Marshal(model.SwipeRequest{
		UserID:    userID,
		DoorID:    uuid.New().String(),
		Direction: "IN",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/access/swipe", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestAccessHandler_Swipe_Allow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	userID := uuid.New().String()
	cache := &mockCacheStore{
		passback: map[string]model.PassbackState{userID: model.PassbackNone},
	}
	h := NewAccessHandler(service.NewAccessDecisionService(cache), &mockStateCache{}, &mockPublisher{})
	r := gin.New()
	r.POST("/access/swipe", h.Swipe)

	body, _ := json.Marshal(model.SwipeRequest{
		UserID:    userID,
		CardUID:   "card-1",
		DoorID:    uuid.New().String(),
		Direction: "IN",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/access/swipe", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestAccessHandler_EmployeeState_InvalidUUID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewAccessHandler(service.NewAccessDecisionService(&mockCacheStore{}), &mockStateCache{}, &mockPublisher{})
	r := gin.New()
	r.GET("/access/employee/:userId/state", h.EmployeeState)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/access/employee/not-a-uuid/state", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAccessHandler_EmployeeState_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	userID := uuid.New().String()
	stateCache := &mockStateCache{passback: model.PassbackIN}
	h := NewAccessHandler(service.NewAccessDecisionService(&mockCacheStore{}), stateCache, &mockPublisher{})
	r := gin.New()
	r.GET("/access/employee/:userId/state", h.EmployeeState)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/access/employee/"+userID+"/state", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestAccessHandler_DoorStatus_InvalidUUID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewAccessHandler(service.NewAccessDecisionService(&mockCacheStore{}), &mockStateCache{}, &mockPublisher{})
	r := gin.New()
	r.GET("/access/door/:doorId/status", h.DoorStatus)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/access/door/bad-id/status", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAccessHandler_DoorStatus_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	doorID := uuid.New().String()
	stateCache := &mockStateCache{doorStatus: "online"}
	h := NewAccessHandler(service.NewAccessDecisionService(&mockCacheStore{}), stateCache, &mockPublisher{})
	r := gin.New()
	r.GET("/access/door/:doorId/status", h.DoorStatus)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/access/door/"+doorID+"/status", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestAccessHandler_EmployeeState_CacheError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	userID := uuid.New().String()
	stateCache := &mockStateCache{passbackErr: errors.New("redis down")}
	h := NewAccessHandler(service.NewAccessDecisionService(&mockCacheStore{}), stateCache, &mockPublisher{})
	r := gin.New()
	r.GET("/access/employee/:userId/state", h.EmployeeState)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/access/employee/"+userID+"/state", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}
