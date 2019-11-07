package timescale

import (
	"gitlab.com/thorchain/bepswap/chain-service/internal/config"
	"gitlab.com/thorchain/bepswap/chain-service/internal/models"
)

type DB struct {
	cfg config.TimeScaleConfiguration
}

func New(cfg config.TimeScaleConfiguration) (*DB, error) {
	return &DB{
		cfg:cfg,
	}, nil
}

func (db *DB) GetPool(ticker models.Asset) (models.Pool, error) {
	return models.Pool{}, nil
}

func (db *DB) GetMaxIDStakes() (int64, error) {
	return 0, nil
}

func (db *DB) GetMaxIDSwaps() (int64, error) {
	return 0, nil
}