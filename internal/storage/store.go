package storage

import (
	"context"

	"github.com/bhavyavj/oi-assistant/internal/models"
)

// Store defines the storage operations for option records and analysis cache.
type Store interface {
	SaveRecords(ctx context.Context, symbol string, records []models.OptionRecord) error
	GetRecords(ctx context.Context, symbol string) ([]models.OptionRecord, error)
	SaveSpotPrice(ctx context.Context, symbol string, price float64) error
	GetSpotPrice(ctx context.Context, symbol string) (float64, error)
	SaveAnalysis(ctx context.Context, symbol string, resp *models.AnalyseResponse) error
	GetAnalysis(ctx context.Context, symbol string) (*models.AnalyseResponse, error)
	DeleteSignals(ctx context.Context, symbol string) error
	Close() error
}
