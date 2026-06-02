package cache

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/tsmc/access-api/internal/model"
)

func newTestCache(t *testing.T) (*RedisCache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	c := NewRedisCache(mr.Addr())
	return c, mr
}

func TestRedisCache_Ping(t *testing.T) {
	c, _ := newTestCache(t)
	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping() = %v", err)
	}
}

func TestRedisCache_IsDenied_Miss(t *testing.T) {
	c, _ := newTestCache(t)
	denied, err := c.IsDenied(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("IsDenied() error = %v", err)
	}
	if denied {
		t.Error("expected not denied for missing key")
	}
}

func TestRedisCache_IsDenied_Hit(t *testing.T) {
	c, mr := newTestCache(t)
	if err := mr.Set("perm:denied:user-1", "DENY"); err != nil {
		t.Fatalf("mr.Set failed: %v", err)
	}
	denied, err := c.IsDenied(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("IsDenied() error = %v", err)
	}
	if !denied {
		t.Error("expected denied for existing key")
	}
}

func TestRedisCache_GetPassback_None(t *testing.T) {
	c, _ := newTestCache(t)
	state, err := c.GetPassback(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetPassback() error = %v", err)
	}
	if state != model.PassbackNone {
		t.Errorf("state = %q, want NONE", state)
	}
}

func TestRedisCache_GetPassback_IN(t *testing.T) {
	c, mr := newTestCache(t)
	if err := mr.Set("passback:user-1", "IN"); err != nil {
		t.Fatalf("mr.Set failed: %v", err)
	}
	state, err := c.GetPassback(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetPassback() error = %v", err)
	}
	if state != model.PassbackIN {
		t.Errorf("state = %q, want IN", state)
	}
}

func TestRedisCache_GetPassback_OUT(t *testing.T) {
	c, mr := newTestCache(t)
	if err := mr.Set("passback:user-1", "OUT"); err != nil {
		t.Fatalf("mr.Set failed: %v", err)
	}
	state, err := c.GetPassback(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetPassback() error = %v", err)
	}
	if state != model.PassbackOUT {
		t.Errorf("state = %q, want OUT", state)
	}
}

func TestRedisCache_GetPassback_UnknownValue(t *testing.T) {
	c, mr := newTestCache(t)
	if err := mr.Set("passback:user-1", "WEIRD"); err != nil {
		t.Fatalf("mr.Set failed: %v", err)
	}
	state, err := c.GetPassback(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetPassback() error = %v", err)
	}
	if state != model.PassbackNone {
		t.Errorf("state = %q, want NONE for unknown value", state)
	}
}

func TestRedisCache_SetPassback(t *testing.T) {
	c, mr := newTestCache(t)
	err := c.SetPassback(context.Background(), "user-1", model.PassbackIN)
	if err != nil {
		t.Fatalf("SetPassback() = %v", err)
	}
	val, _ := mr.Get("passback:user-1")
	if val != "IN" {
		t.Errorf("stored value = %q, want IN", val)
	}
}

func TestRedisCache_LookupCard_Miss(t *testing.T) {
	c, _ := newTestCache(t)
	val, err := c.LookupCard(context.Background(), "card-123")
	if err != nil {
		t.Fatalf("LookupCard() error = %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string for miss, got %q", val)
	}
}

func TestRedisCache_LookupCard_Hit(t *testing.T) {
	c, mr := newTestCache(t)
	if err := mr.Set("card:card-123", "user-42"); err != nil {
		t.Fatalf("mr.Set failed: %v", err)
	}
	val, err := c.LookupCard(context.Background(), "card-123")
	if err != nil {
		t.Fatalf("LookupCard() error = %v", err)
	}
	if val != "user-42" {
		t.Errorf("val = %q, want user-42", val)
	}
}

func TestRedisCache_SetCardMapping(t *testing.T) {
	c, mr := newTestCache(t)
	err := c.SetCardMapping(context.Background(), "card-abc", "user-xyz")
	if err != nil {
		t.Fatalf("SetCardMapping() = %v", err)
	}
	val, _ := mr.Get("card:card-abc")
	if val != "user-xyz" {
		t.Errorf("stored value = %q, want user-xyz", val)
	}
}

func TestRedisCache_GetDoorStatus_Offline(t *testing.T) {
	c, _ := newTestCache(t)
	status, err := c.GetDoorStatus(context.Background(), "door-1")
	if err != nil {
		t.Fatalf("GetDoorStatus() error = %v", err)
	}
	if status != "OFFLINE" {
		t.Errorf("status = %q, want OFFLINE", status)
	}
}

func TestRedisCache_GetDoorStatus_Online(t *testing.T) {
	c, mr := newTestCache(t)
	if err := mr.Set("door:status:door-1", "ONLINE"); err != nil {
		t.Fatalf("mr.Set failed: %v", err)
	}
	status, err := c.GetDoorStatus(context.Background(), "door-1")
	if err != nil {
		t.Fatalf("GetDoorStatus() error = %v", err)
	}
	if status != "ONLINE" {
		t.Errorf("status = %q, want ONLINE", status)
	}
}

func TestRedisCache_SetDoorStatus(t *testing.T) {
	c, mr := newTestCache(t)
	err := c.SetDoorStatus(context.Background(), "door-1", "ONLINE")
	if err != nil {
		t.Fatalf("SetDoorStatus() = %v", err)
	}
	val, _ := mr.Get("door:status:door-1")
	if val != "ONLINE" {
		t.Errorf("stored value = %q, want ONLINE", val)
	}
}

func TestKeyFormatters(t *testing.T) {
	if permDeniedKey("u1") != "perm:denied:u1" {
		t.Errorf("permDeniedKey = %q", permDeniedKey("u1"))
	}
	if passbackKey("u2") != "passback:u2" {
		t.Errorf("passbackKey = %q", passbackKey("u2"))
	}
	if cardKey("c1") != "card:c1" {
		t.Errorf("cardKey = %q", cardKey("c1"))
	}
	if doorStatusKey("d1") != "door:status:d1" {
		t.Errorf("doorStatusKey = %q", doorStatusKey("d1"))
	}
}

func TestRedisCache_BatchRead_HitAll(t *testing.T) {
	c, mr := newTestCache(t)
	mr.Set("card:card-1", "user-1")
	mr.Set("perm:denied:user-1", "1")
	mr.Set("passback:user-1", "IN")

	res, err := c.BatchRead(context.Background(), "card-1", "user-1")
	if err != nil {
		t.Fatalf("BatchRead error: %v", err)
	}
	if res.MappedUserID != "user-1" {
		t.Errorf("MappedUserID = %q, want user-1", res.MappedUserID)
	}
	if !res.IsDenied {
		t.Errorf("IsDenied = %v, want true", res.IsDenied)
	}
	if res.PassbackState != model.PassbackIN {
		t.Errorf("PassbackState = %v, want IN", res.PassbackState)
	}
}

func TestRedisCache_BatchRead_MissAll(t *testing.T) {
	c, _ := newTestCache(t)
	res, err := c.BatchRead(context.Background(), "card-2", "user-2")
	if err != nil {
		t.Fatalf("BatchRead error: %v", err)
	}
	if res.MappedUserID != "" {
		t.Errorf("MappedUserID = %q, want empty", res.MappedUserID)
	}
	if res.IsDenied {
		t.Errorf("IsDenied = %v, want false", res.IsDenied)
	}
	if res.PassbackState != model.PassbackNone {
		t.Errorf("PassbackState = %v, want NONE", res.PassbackState)
	}
}

func TestRedisCache_BatchRead_EmptyCard(t *testing.T) {
	c, mr := newTestCache(t)
	mr.Set("passback:user-3", "OUT")

	res, err := c.BatchRead(context.Background(), "", "user-3")
	if err != nil {
		t.Fatalf("BatchRead error: %v", err)
	}
	if res.MappedUserID != "" {
		t.Errorf("MappedUserID = %q, want empty", res.MappedUserID)
	}
	if res.PassbackState != model.PassbackOUT {
		t.Errorf("PassbackState = %v, want OUT", res.PassbackState)
	}
}

func TestRedisCache_IsDenied_Error(t *testing.T) {
	c, _ := newTestCache(t)
	c.client.Close() // Close client to force error
	_, err := c.IsDenied(context.Background(), "user-1")
	if err == nil {
		t.Error("expected error from closed client")
	}
}

func TestNewRedisCache_Cluster(t *testing.T) {
	t.Setenv("REDIS_CLUSTER", "true")
	c := NewRedisCache("localhost:6379")
	if c == nil {
		t.Fatal("expected non-nil cache")
	}
}

func TestRedisCache_LookupCard_Error(t *testing.T) {
	c, _ := newTestCache(t)
	c.client.Close() // Close client to force error
	_, err := c.LookupCard(context.Background(), "card-1")
	if err == nil {
		t.Error("expected error from closed client")
	}
}

func TestRedisCache_GetPassback_Error(t *testing.T) {
	c, _ := newTestCache(t)
	c.client.Close() // Close client to force error
	_, err := c.GetPassback(context.Background(), "user-1")
	if err == nil {
		t.Error("expected error from closed client")
	}
}

func TestRedisCache_GetDoorStatus_Error(t *testing.T) {
	c, _ := newTestCache(t)
	c.client.Close() // Close client to force error
	_, err := c.GetDoorStatus(context.Background(), "door-1")
	if err == nil {
		t.Error("expected error from closed client")
	}
}

func TestRedisCache_BatchRead_Error(t *testing.T) {
	c, _ := newTestCache(t)
	c.client.Close() // Close client to force error
	_, err := c.BatchRead(context.Background(), "card-1", "user-1")
	if err == nil {
		t.Error("expected error from closed client")
	}
}

