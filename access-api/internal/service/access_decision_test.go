package service

import (
	"context"
	"errors"
	"testing"

	"github.com/tsmc/access-api/internal/cache"
	"github.com/tsmc/access-api/internal/model"
)

// --- mock implementations ---

type mockCache struct {
	denied   map[string]bool
	passback map[string]model.PassbackState
	cards    map[string]string // cardUID → userID
	setErr   error
	readErr  error // simulates Redis being completely down
}

func (m *mockCache) IsDenied(_ context.Context, userID string) (bool, error) {
	if m.readErr != nil {
		return false, m.readErr
	}
	return m.denied[userID], nil
}

func (m *mockCache) GetPassback(_ context.Context, userID string) (model.PassbackState, error) {
	if m.readErr != nil {
		return model.PassbackNone, m.readErr
	}
	if s, ok := m.passback[userID]; ok {
		return s, nil
	}
	return model.PassbackNone, nil
}

func (m *mockCache) SetPassback(_ context.Context, userID string, state model.PassbackState) error {
	if m.setErr != nil {
		return m.setErr
	}
	if m.readErr != nil {
		return m.readErr
	}
	if m.passback == nil {
		m.passback = make(map[string]model.PassbackState)
	}
	m.passback[userID] = state
	return nil
}

func (m *mockCache) LookupCard(_ context.Context, cardUID string) (string, error) {
	if m.readErr != nil {
		return "", m.readErr
	}
	if m.cards == nil {
		return "", nil
	}
	return m.cards[cardUID], nil
}

func (m *mockCache) SetCardMapping(_ context.Context, cardUID, userID string) error {
	if m.setErr != nil {
		return m.setErr
	}
	if m.readErr != nil {
		return m.readErr
	}
	if m.cards == nil {
		m.cards = make(map[string]string)
	}
	m.cards[cardUID] = userID
	return nil
}

// BatchRead implements the hot-path pipeline read used by Evaluate.
func (m *mockCache) BatchRead(_ context.Context, cardUID, userID string) (cache.BatchReadResult, error) {
	if m.readErr != nil {
		return cache.BatchReadResult{}, m.readErr
	}
	var result cache.BatchReadResult
	// Card mapping
	if cardUID != "" && m.cards != nil {
		result.MappedUserID = m.cards[cardUID]
	}
	// Permission denied
	if m.denied != nil {
		result.IsDenied = m.denied[userID]
	}
	// Passback state
	if m.passback != nil {
		result.PassbackState = m.passback[userID]
	}
	return result, nil
}

type mockDB struct {
	active   map[string]bool   // userID → is_active
	cards    map[string]string // cardUID → userID
	passback map[string]string // userID → last IN/OUT
	err      error
}

func (m *mockDB) IsActive(_ context.Context, userID string) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	active, ok := m.active[userID]
	if !ok {
		return true, nil // unknown user — fail-open
	}
	return active, nil
}

func (m *mockDB) LookupCardUID(_ context.Context, cardUID string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	if m.cards == nil {
		return "", nil
	}
	return m.cards[cardUID], nil
}

func (m *mockDB) GetLastPassbackState(_ context.Context, userID string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	if m.passback == nil {
		return "", nil
	}
	return m.passback[userID], nil
}

// --- Tests: CARD_NOT_FOUND ---

func TestEvaluate_CardNotFound(t *testing.T) {
	svc := NewAccessDecisionService(&mockCache{
		cards: map[string]string{"CARD001": "u1"},
	})
	res, err := svc.Evaluate(context.Background(), "u1", "INVALID_CARD", model.DirectionIN)
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision != model.DecisionDeny || res.Reason == nil || *res.Reason != model.ReasonCardNotFound {
		t.Fatalf("expected DENY+CARD_NOT_FOUND, got %+v", res)
	}
}

func TestEvaluate_CardMismatch(t *testing.T) {
	svc := NewAccessDecisionService(&mockCache{
		cards: map[string]string{"CARD001": "other-user"},
	})
	res, err := svc.Evaluate(context.Background(), "u1", "CARD001", model.DirectionIN)
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision != model.DecisionDeny || *res.Reason != model.ReasonCardNotFound {
		t.Fatalf("expected DENY+CARD_NOT_FOUND for mismatched card, got %+v", res)
	}
}

func TestEvaluate_CardValid(t *testing.T) {
	svc := NewAccessDecisionService(&mockCache{
		cards: map[string]string{"CARD001": "u1"},
	})
	res, err := svc.Evaluate(context.Background(), "u1", "CARD001", model.DirectionIN)
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision != model.DecisionAllow {
		t.Fatalf("expected ALLOW for valid card, got %+v", res)
	}
}

func TestEvaluate_EmptyCardUID_SkipsValidation(t *testing.T) {
	svc := NewAccessDecisionService(&mockCache{})
	res, err := svc.Evaluate(context.Background(), "u1", "", model.DirectionIN)
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision != model.DecisionAllow {
		t.Fatalf("expected ALLOW when cardUID is empty, got %+v", res)
	}
}

// --- Tests: Original logic (preserved) ---

func TestEvaluate_PermissionDenied(t *testing.T) {
	svc := NewAccessDecisionService(&mockCache{
		denied: map[string]bool{"u1": true},
	})
	res, err := svc.Evaluate(context.Background(), "u1", "", model.DirectionIN)
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision != model.DecisionDeny || res.Reason == nil || *res.Reason != model.ReasonPermissionDenied {
		t.Fatalf("got %+v", res)
	}
}

func TestEvaluate_AntiPassbackIN(t *testing.T) {
	svc := NewAccessDecisionService(&mockCache{
		passback: map[string]model.PassbackState{"u1": model.PassbackIN},
	})
	res, err := svc.Evaluate(context.Background(), "u1", "", model.DirectionIN)
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision != model.DecisionDeny || *res.Reason != model.ReasonAntiPassback {
		t.Fatalf("got %+v", res)
	}
}

func TestEvaluate_AntiPassbackOUT(t *testing.T) {
	svc := NewAccessDecisionService(&mockCache{
		passback: map[string]model.PassbackState{"u1": model.PassbackOUT},
	})
	res, err := svc.Evaluate(context.Background(), "u1", "", model.DirectionOUT)
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision != model.DecisionDeny {
		t.Fatalf("got %+v", res)
	}
}

func TestEvaluate_AllowINFromOUT(t *testing.T) {
	svc := NewAccessDecisionService(&mockCache{
		passback: map[string]model.PassbackState{"u1": model.PassbackOUT},
	})
	res, err := svc.Evaluate(context.Background(), "u1", "", model.DirectionIN)
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision != model.DecisionAllow {
		t.Fatalf("got %+v", res)
	}
}

func TestEvaluate_AllowOUTFromIN(t *testing.T) {
	svc := NewAccessDecisionService(&mockCache{
		passback: map[string]model.PassbackState{"u1": model.PassbackIN},
	})
	res, err := svc.Evaluate(context.Background(), "u1", "", model.DirectionOUT)
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision != model.DecisionAllow {
		t.Fatalf("got %+v", res)
	}
}

func TestEvaluate_AllowFirstIN(t *testing.T) {
	svc := NewAccessDecisionService(&mockCache{})
	res, err := svc.Evaluate(context.Background(), "u1", "", model.DirectionIN)
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision != model.DecisionAllow {
		t.Fatalf("got %+v", res)
	}
}

// --- Tests: Redis down + DB fallback ---

func TestEvaluate_RedisDown_NoDB_ReturnsError(t *testing.T) {
	svc := NewAccessDecisionService(&mockCache{readErr: errors.New("redis down")})
	// No DB configured — should return ErrCacheUnavailable
	_, err := svc.Evaluate(context.Background(), "u1", "", model.DirectionIN)
	if !errors.Is(err, ErrCacheUnavailable) {
		t.Fatalf("expected ErrCacheUnavailable, got %v", err)
	}
}

func TestEvaluate_RedisDown_DBFallback_Active(t *testing.T) {
	svc := NewAccessDecisionService(&mockCache{readErr: errors.New("redis down")})
	svc.SetDBFallback(&mockDB{active: map[string]bool{"u1": true}})

	res, err := svc.Evaluate(context.Background(), "u1", "", model.DirectionIN)
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision != model.DecisionAllow {
		t.Fatalf("expected ALLOW via fallback, got %+v", res)
	}
	if !res.Degraded {
		t.Fatal("expected Degraded=true")
	}
}

func TestEvaluate_RedisDown_DBFallback_Banned(t *testing.T) {
	svc := NewAccessDecisionService(&mockCache{readErr: errors.New("redis down")})
	svc.SetDBFallback(&mockDB{active: map[string]bool{"u1": false}})

	res, err := svc.Evaluate(context.Background(), "u1", "", model.DirectionIN)
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision != model.DecisionDeny || *res.Reason != model.ReasonPermissionDenied {
		t.Fatalf("expected DENY+PERMISSION_DENIED via fallback, got %+v", res)
	}
	if !res.Degraded {
		t.Fatal("expected Degraded=true")
	}
}

func TestEvaluate_RedisDown_DBFallback_DBAlsoDown(t *testing.T) {
	svc := NewAccessDecisionService(&mockCache{readErr: errors.New("redis down")})
	svc.SetDBFallback(&mockDB{err: errors.New("db down too")})

	_, err := svc.Evaluate(context.Background(), "u1", "", model.DirectionIN)
	if err == nil {
		t.Fatal("expected error when both Redis and DB are down")
	}
}

// --- Tests: Card lookup fallback to DB ---

func TestEvaluate_CardLookup_RedisDown_DBFallback_CardNotFound(t *testing.T) {
	svc := NewAccessDecisionService(&mockCache{readErr: errors.New("redis down")})
	svc.SetDBFallback(&mockDB{
		active: map[string]bool{"u1": true},
		cards:  map[string]string{}, // no cards
	})

	res, err := svc.Evaluate(context.Background(), "u1", "UNKNOWN_CARD", model.DirectionIN)
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision != model.DecisionDeny || *res.Reason != model.ReasonCardNotFound {
		t.Fatalf("expected DENY+CARD_NOT_FOUND via DB fallback, got %+v", res)
	}
}

func TestEvaluate_CacheSetError(t *testing.T) {
	svc := NewAccessDecisionService(&mockCache{setErr: errors.New("redis down")})
	// No DB configured — SetPassback fails → returns ErrCacheUnavailable
	_, err := svc.Evaluate(context.Background(), "u1", "", model.DirectionIN)
	if !errors.Is(err, ErrCacheUnavailable) {
		t.Fatalf("expected ErrCacheUnavailable, got %v", err)
	}
}

func TestEvaluate_RedisDown_DBFallback_AntiPassback(t *testing.T) {
	svc := NewAccessDecisionService(&mockCache{readErr: errors.New("redis down")})
	svc.SetDBFallback(&mockDB{
		active:   map[string]bool{"u1": true},
		passback: map[string]string{"u1": "IN"},
	})
	res, err := svc.Evaluate(context.Background(), "u1", "", model.DirectionIN)
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision != model.DecisionDeny || res.Reason == nil || *res.Reason != model.ReasonAntiPassback {
		t.Fatalf("expected DENY+ANTI_PASSBACK via DB passback, got %+v", res)
	}
	if !res.Degraded {
		t.Fatal("expected Degraded=true")
	}
}
