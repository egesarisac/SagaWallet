package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/egesarisac/SagaWallet/pkg/kafka"
	"github.com/egesarisac/SagaWallet/pkg/logger"
	"github.com/egesarisac/SagaWallet/pkg/models"
	"github.com/egesarisac/SagaWallet/services/transaction-service/internal/repository"
)

// OutboxPublisher durably delivers transfer events recorded by database transactions.
type OutboxPublisher struct {
	repo     *repository.TransferRepository
	producer *kafka.Producer
	log      *logger.Logger
	workerID string
}

func NewOutboxPublisher(repo *repository.TransferRepository, producer *kafka.Producer, log *logger.Logger) *OutboxPublisher {
	return &OutboxPublisher{repo: repo, producer: producer, log: log, workerID: uuid.NewString()}
}

func (p *OutboxPublisher) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	p.publishDue(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.publishDue(ctx)
		}
	}
}

func (p *OutboxPublisher) publishDue(ctx context.Context) {
	events, err := p.repo.ClaimOutbox(ctx, 25, 30)
	if err != nil {
		p.log.WithError(err).Error().Msg("Failed to claim outbox events")
		return
	}
	for _, outbox := range events {
		var event models.Event
		if err := json.Unmarshal(outbox.Payload, &event); err != nil {
			p.log.WithError(err).WithField("outbox_id", outbox.ID.String()).Error().Msg("Invalid outbox event payload")
			_ = p.repo.ReleaseOutbox(ctx, outbox.ID, outbox.Attempts, err)
			continue
		}
		if err := p.producer.Publish(ctx, outbox.Topic, &event); err != nil {
			p.log.WithError(err).WithField("outbox_id", outbox.ID.String()).Error().Msg("Failed to publish outbox event")
			_ = p.repo.ReleaseOutbox(ctx, outbox.ID, outbox.Attempts, err)
			continue
		}
		if err := p.repo.MarkOutboxPublished(ctx, outbox.ID); err != nil {
			p.log.WithError(err).WithField("outbox_id", outbox.ID.String()).Error().Msg("Published event could not be marked complete")
		}
	}
}
