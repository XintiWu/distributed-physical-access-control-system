package consumer

import (
	"context"
	"errors"
	"testing"

	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockKafkaReader is a mock of KafkaReader
type MockKafkaReader struct {
	mock.Mock
}

func (m *MockKafkaReader) FetchMessage(ctx context.Context) (kafka.Message, error) {
	args := m.Called(ctx)
	return args.Get(0).(kafka.Message), args.Error(1)
}

func (m *MockKafkaReader) CommitMessages(ctx context.Context, msgs ...kafka.Message) error {
	args := m.Called(ctx, msgs)
	return args.Error(0)
}

func (m *MockKafkaReader) Close() error {
	args := m.Called()
	return args.Error(0)
}

// MockCacheStore is a mock of CacheStore
type MockCacheStore struct {
	mock.Mock
}

func (m *MockCacheStore) SetDenied(ctx context.Context, userID string) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

func (m *MockCacheStore) ClearDenied(ctx context.Context, userID string) error {
	if ctx == nil {
		return nil
	}
	args := m.Called(ctx, userID)
	return args.Error(0)
}

func TestWorker_Run_Success_Ban(t *testing.T) {
	mockReader := new(MockKafkaReader)
	mockCache := new(MockCacheStore)

	eventJSON := `{"userId":"user-123","action":"BAN"}`
	msg := kafka.Message{
		Topic: "perm_events",
		Value: []byte(eventJSON),
	}

	ctx, cancel := context.WithCancel(context.Background())

	mockReader.On("FetchMessage", mock.Anything).Return(msg, nil).Once()
	mockReader.On("FetchMessage", mock.Anything).Run(func(args mock.Arguments) {
		cancel()
	}).Return(kafka.Message{}, context.Canceled).Once()

	mockCache.On("SetDenied", mock.Anything, "user-123").Return(nil).Once()
	mockReader.On("CommitMessages", mock.Anything, []kafka.Message{msg}).Return(nil).Once()

	w := &Worker{
		reader: mockReader,
		cache:  mockCache,
	}

	err := w.Run(ctx)
	assert.NoError(t, err)

	mockReader.AssertExpectations(t)
	mockCache.AssertExpectations(t)
}

func TestWorker_Run_Success_Unban(t *testing.T) {
	mockReader := new(MockKafkaReader)
	mockCache := new(MockCacheStore)

	eventJSON := `{"userId":"user-123","action":"UNBAN"}`
	msg := kafka.Message{
		Topic: "perm_events",
		Value: []byte(eventJSON),
	}

	ctx, cancel := context.WithCancel(context.Background())

	mockReader.On("FetchMessage", mock.Anything).Return(msg, nil).Once()
	mockReader.On("FetchMessage", mock.Anything).Run(func(args mock.Arguments) {
		cancel()
	}).Return(kafka.Message{}, context.Canceled).Once()

	mockCache.On("ClearDenied", mock.Anything, "user-123").Return(nil).Once()
	mockReader.On("CommitMessages", mock.Anything, []kafka.Message{msg}).Return(nil).Once()

	w := &Worker{
		reader: mockReader,
		cache:  mockCache,
	}

	err := w.Run(ctx)
	assert.NoError(t, err)

	mockReader.AssertExpectations(t)
	mockCache.AssertExpectations(t)
}

func TestWorker_Run_UnknownAction(t *testing.T) {
	mockReader := new(MockKafkaReader)
	mockCache := new(MockCacheStore)

	eventJSON := `{"userId":"user-123","action":"UNKNOWN"}`
	msg := kafka.Message{
		Topic: "perm_events",
		Value: []byte(eventJSON),
	}

	ctx, cancel := context.WithCancel(context.Background())

	mockReader.On("FetchMessage", mock.Anything).Return(msg, nil).Once()
	mockReader.On("FetchMessage", mock.Anything).Run(func(args mock.Arguments) {
		cancel()
	}).Return(kafka.Message{}, context.Canceled).Once()

	mockReader.On("CommitMessages", mock.Anything, []kafka.Message{msg}).Return(nil).Once()

	w := &Worker{
		reader: mockReader,
		cache:  mockCache,
	}

	err := w.Run(ctx)
	assert.NoError(t, err)

	mockReader.AssertExpectations(t)
	mockCache.AssertExpectations(t)
}

func TestWorker_Run_CacheError(t *testing.T) {
	mockReader := new(MockKafkaReader)
	mockCache := new(MockCacheStore)

	eventJSON := `{"userId":"user-123","action":"BAN"}`
	msg := kafka.Message{
		Topic: "perm_events",
		Value: []byte(eventJSON),
	}

	ctx, cancel := context.WithCancel(context.Background())

	mockReader.On("FetchMessage", mock.Anything).Return(msg, nil).Once()
	mockReader.On("FetchMessage", mock.Anything).Run(func(args mock.Arguments) {
		cancel()
	}).Return(kafka.Message{}, context.Canceled).Once()

	mockCache.On("SetDenied", mock.Anything, "user-123").Return(errors.New("redis connection failed")).Once()

	w := &Worker{
		reader: mockReader,
		cache:  mockCache,
	}

	err := w.Run(ctx)
	assert.NoError(t, err)

	mockReader.AssertExpectations(t)
	mockCache.AssertExpectations(t)
}

func TestWorker_Run_InvalidJSON(t *testing.T) {
	mockReader := new(MockKafkaReader)
	mockCache := new(MockCacheStore)

	msg := kafka.Message{
		Topic: "perm_events",
		Value: []byte("{invalid-json}"),
	}

	ctx, cancel := context.WithCancel(context.Background())

	mockReader.On("FetchMessage", mock.Anything).Return(msg, nil).Once()
	mockReader.On("FetchMessage", mock.Anything).Run(func(args mock.Arguments) {
		cancel()
	}).Return(kafka.Message{}, context.Canceled).Once()

	mockReader.On("CommitMessages", mock.Anything, []kafka.Message{msg}).Return(nil).Once()

	w := &Worker{
		reader: mockReader,
		cache:  mockCache,
	}

	err := w.Run(ctx)
	assert.NoError(t, err)

	mockReader.AssertExpectations(t)
	mockCache.AssertExpectations(t)
}

func TestWorker_Close(t *testing.T) {
	mockReader := new(MockKafkaReader)
	mockReader.On("Close").Return(nil).Once()

	w := &Worker{
		reader: mockReader,
	}

	err := w.Close()
	assert.NoError(t, err)
	mockReader.AssertExpectations(t)
}

func TestNewWorker(t *testing.T) {
	mockCache := new(MockCacheStore)
	w := NewWorker([]string{"localhost:9092"}, "topic", "group", mockCache)
	assert.NotNil(t, w)
	assert.NotNil(t, w.reader)
	assert.NotNil(t, w.cache)
}

func TestWorker_Run_FetchError(t *testing.T) {
	mockReader := new(MockKafkaReader)
	mockCache := new(MockCacheStore)

	ctx, cancel := context.WithCancel(context.Background())

	// Return error first time, then context canceled
	mockReader.On("FetchMessage", mock.Anything).Return(kafka.Message{}, errors.New("temporary error")).Once()
	mockReader.On("FetchMessage", mock.Anything).Run(func(args mock.Arguments) {
		cancel()
	}).Return(kafka.Message{}, context.Canceled).Once()

	w := &Worker{
		reader: mockReader,
		cache:  mockCache,
	}

	err := w.Run(ctx)
	assert.NoError(t, err)

	mockReader.AssertExpectations(t)
	mockCache.AssertExpectations(t)
}

func TestWorker_Run_CommitError(t *testing.T) {
	mockReader := new(MockKafkaReader)
	mockCache := new(MockCacheStore)

	eventJSON := `{"userId":"user-123","action":"BAN"}`
	msg := kafka.Message{
		Topic: "perm_events",
		Value: []byte(eventJSON),
	}

	ctx, cancel := context.WithCancel(context.Background())

	mockReader.On("FetchMessage", mock.Anything).Return(msg, nil).Once()
	mockReader.On("FetchMessage", mock.Anything).Run(func(args mock.Arguments) {
		cancel()
	}).Return(kafka.Message{}, context.Canceled).Once()

	mockCache.On("SetDenied", mock.Anything, "user-123").Return(nil).Once()
	mockReader.On("CommitMessages", mock.Anything, []kafka.Message{msg}).Return(errors.New("commit failed")).Once()

	w := &Worker{
		reader: mockReader,
		cache:  mockCache,
	}

	err := w.Run(ctx)
	assert.NoError(t, err)

	mockReader.AssertExpectations(t)
	mockCache.AssertExpectations(t)
}
