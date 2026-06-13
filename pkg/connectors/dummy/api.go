package dummy

import (
	"context"

	"degen/pkg/models"
)

func (d *Dummy) GetAccountInfo(_ context.Context) (*models.AccountInfo, error) {
	ai := models.AccountInfo{
		Balances:  d.balances.Snapshot(),
		Positions: d.positions.Snapshot(),
	}

	return &ai, nil
}
