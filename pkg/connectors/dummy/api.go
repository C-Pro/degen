package dummy

import (
	"context"

	"degen/pkg/models"
)

func (d *Dummy) GetAccountInfo(_ context.Context) (*models.AccountInfo, error) {
	ai := models.AccountInfo{
		Balances:  make(map[string]models.Balance),
		Positions: make(map[string]models.Position),
	}
	for k, v := range d.balances.Snapshot() {
		ai.Balances[k] = v
	}
	for k, v := range d.positions.Snapshot() {
		ai.Positions[k] = v
	}

	return &ai, nil
}
