package service

import (
	"context"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/tsmc/access-api/internal/cache"
	"github.com/tsmc/access-api/internal/model"
)

var (
	fallbackTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "access_api_fallback_total",
		Help: "Number of swipe decisions that fell back to DB because Redis was unavailable",
	})
)

// CacheStore abstracts Redis operations for the decision service.
type CacheStore interface {
	// BatchRead fetches card mapping, permission-denied flag, and passback state
	// in a single pipelined round-trip (hot path optimisation).
	BatchRead(ctx context.Context, cardUID, userID string) (cache.BatchReadResult, error)
	SetPassback(ctx context.Context, userID string, state model.PassbackState) error
	SetCardMapping(ctx context.Context, cardUID, userID string) error
	// Individual methods retained for the DB-fallback path.
	IsDenied(ctx context.Context, userID string) (bool, error)
	GetPassback(ctx context.Context, userID string) (model.PassbackState, error)
	LookupCard(ctx context.Context, cardUID string) (string, error)
}

// DBStore abstracts ClickHouse employee queries used only when Redis is down (fallback path).
type DBStore interface {
	IsActive(ctx context.Context, userID string) (bool, error)
	LookupCardUID(ctx context.Context, cardUID string) (string, error)
	GetLastPassbackState(ctx context.Context, userID string) (string, error)
}

type DecisionResult struct {
	Decision model.Decision
	Reason   *model.DenyReason
	Degraded bool // true when decision was made via DB fallback
}

type AccessDecisionService struct {
	cache CacheStore
	db    DBStore // nil if DB fallback is not configured
}

func NewAccessDecisionService(cache CacheStore) *AccessDecisionService {
	return &AccessDecisionService{cache: cache}
}

// SetDBFallback enables the DB fallback path for when Redis is unavailable.
func (s *AccessDecisionService) SetDBFallback(db DBStore) {
	s.db = db
}

// Evaluate performs the access decision.
// Hot path: BatchRead pipelines 3 Redis reads into 1 RTT, then 1 write = 2 RTTs total.
// Order: card validation → permission check → anti-passback → ALLOW.
func (s *AccessDecisionService) Evaluate(ctx context.Context, userID, cardUID string, direction model.Direction) (DecisionResult, error) {
	// Pipelined read: card mapping + denied flag + passback state in ONE round-trip.
	batch, err := s.cache.BatchRead(ctx, cardUID, userID)
	if err != nil {
		// Redis unavailable — fall back to individual DB queries.
		return s.evaluateFallback(ctx, userID, cardUID, direction)
	}

	// Step 1: Card validation
	if cardUID != "" {
		if batch.MappedUserID == "" {
			// Cache miss — try DB fallback with read-through population.
			if s.db != nil {
				dbUser, dbErr := s.db.LookupCardUID(ctx, cardUID)
				if dbErr != nil {
					slog.Warn("card lookup DB fallback failed", "error", dbErr)
				} else if dbUser != "" {
					batch.MappedUserID = dbUser
					// Populate Redis so subsequent requests hit the cache.
					_ = s.cache.SetCardMapping(ctx, cardUID, dbUser)
				}
			}
		}
		if batch.MappedUserID == "" || batch.MappedUserID != userID {
			r := model.ReasonCardNotFound
			return DecisionResult{Decision: model.DecisionDeny, Reason: &r}, nil
		}
	}

	// Step 2: Permission denied check (result already in batch)
	if batch.IsDenied {
		r := model.ReasonPermissionDenied
		return DecisionResult{Decision: model.DecisionDeny, Reason: &r}, nil
	}

	// Step 3: Anti-passback (result already in batch)
	if checkAntiPassback(batch.PassbackState, direction) {
		r := model.ReasonAntiPassback
		return DecisionResult{Decision: model.DecisionDeny, Reason: &r}, nil
	}

	// Step 4: Update passback state (single write RTT)
	if err := s.cache.SetPassback(ctx, userID, model.PassbackState(direction)); err != nil {
		return s.evaluateFallback(ctx, userID, cardUID, direction)
	}

	return DecisionResult{Decision: model.DecisionAllow, Reason: nil}, nil
}

// evaluateFallback makes a degraded decision via DB when Redis is down.
// It also validates the card if cardUID is provided.
// Anti-passback uses the last ALLOW event from ClickHouse when available.
func (s *AccessDecisionService) evaluateFallback(ctx context.Context, userID, cardUID string, direction model.Direction) (DecisionResult, error) {
	if s.db == nil {
		return DecisionResult{}, ErrCacheUnavailable
	}

	fallbackTotal.Inc()
	slog.Warn("redis unavailable, falling back to DB (degraded)", "userId", userID)

	// Card validation via DB (when Redis is unavailable for card lookup)
	if cardUID != "" {
		dbUser, dbErr := s.db.LookupCardUID(ctx, cardUID)
		if dbErr != nil {
			slog.Warn("card lookup DB fallback failed", "error", dbErr)
		}
		if dbUser == "" || dbUser != userID {
			r := model.ReasonCardNotFound
			return DecisionResult{Decision: model.DecisionDeny, Reason: &r, Degraded: true}, nil
		}
	}

	active, err := s.db.IsActive(ctx, userID)
	if err != nil {
		return DecisionResult{}, err
	}
	if !active {
		r := model.ReasonPermissionDenied
		return DecisionResult{Decision: model.DecisionDeny, Reason: &r, Degraded: true}, nil
	}

	state := model.PassbackNone
	if lastDir, err := s.db.GetLastPassbackState(ctx, userID); err == nil && lastDir != "" {
		state = model.PassbackState(lastDir)
	}
	if violation := checkAntiPassback(state, direction); violation {
		r := model.ReasonAntiPassback
		return DecisionResult{Decision: model.DecisionDeny, Reason: &r, Degraded: true}, nil
	}

	return DecisionResult{Decision: model.DecisionAllow, Reason: nil, Degraded: true}, nil
}

func checkAntiPassback(state model.PassbackState, direction model.Direction) bool {
	switch {
	case direction == model.DirectionIN && state == model.PassbackIN:
		return true
	case direction == model.DirectionOUT && state == model.PassbackOUT:
		return true
	default:
		return false
	}
}
