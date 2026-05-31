package consumer

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/tsmc/aggregation-worker/internal/model"
)

type KafkaReader interface {
	FetchMessage(ctx context.Context) (kafka.Message, error)
	CommitMessages(ctx context.Context, msgs ...kafka.Message) error
	Close() error
}

type Repository interface {
	Insert(ctx context.Context, e model.InOutEvent, orgUnitID string) error
	GetEmployeeOrgUnitID(ctx context.Context, employeeID string) (string, error)
}

type Worker struct {
	reader KafkaReader
	repo   Repository
}

func NewWorker(brokers []string, topic, group string, repo Repository) *Worker {
	return &Worker{
		reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers:        brokers,
			Topic:          topic,
			GroupID:        group,
			MinBytes:       1,
			MaxBytes:       10e6,
			CommitInterval: time.Second,
			StartOffset:    kafka.FirstOffset,
		}),
		repo: repo,
	}
}

func (w *Worker) Run(ctx context.Context) error {
	log.Printf("aggregation worker started and listening")
	for {
		log.Printf("waiting for message...")
		msg, err := w.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				log.Printf("worker context canceled, exiting Run")
				return nil
			}
			log.Printf("fetch error: %v", err)
			time.Sleep(time.Second)
			continue
		}
		log.Printf("fetched message from partition %d, offset %d: %s", msg.Partition, msg.Offset, string(msg.Value))

		var event model.InOutEvent
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			log.Printf("invalid message: %v", err)
			_ = w.reader.CommitMessages(ctx, msg)
			continue
		}

		// Fetch employee's org_unit_id from ClickHouse to enrich the event
		orgLookupCtx, orgLookupCancel := context.WithTimeout(ctx, 5*time.Second)
		orgUnitID, err := w.repo.GetEmployeeOrgUnitID(orgLookupCtx, event.EmployeeID)
		if err != nil {
			log.Printf("org lookup failed eventId=%s: %v", event.EventID, err)
		}
		orgLookupCancel()
		if err == nil && orgUnitID == "" {
			orgUnitID = "a0000000-0000-0000-0000-000000000001" // Fallback to root TSMC Corp for unregistered/simulated users
		}
		log.Printf("org lookup for employee %s returned orgUnitID %s", event.EmployeeID, orgUnitID)

		insertCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if err := w.repo.Insert(insertCtx, event, orgUnitID); err != nil {
			cancel()
			log.Printf("insert failed eventId=%s: %v", event.EventID, err)
			continue
		}
		cancel()
		log.Printf("successfully inserted eventId=%s into ClickHouse", event.EventID)

		if err := w.reader.CommitMessages(ctx, msg); err != nil {
			log.Printf("commit failed: %v", err)
		} else {
			log.Printf("committed message eventId=%s", event.EventID)
		}
	}
}

func (w *Worker) Close() error {
	return w.reader.Close()
}
