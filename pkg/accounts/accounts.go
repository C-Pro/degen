package accounts

import (
	"sync"

	"degen/pkg/models"
)

type Accounts struct {
	data map[string]*models.Account
	mux  sync.RWMutex
}

func NewAccounts() *Accounts {
	return &Accounts{
		data: make(map[string]*models.Account),
	}
}

func (a *Accounts) AddAccount(id, exchange string) {
	a.mux.Lock()
	defer a.mux.Unlock()

	a.data[id] = models.NewAccount(id, exchange)
}

func (a *Accounts) GetAccount(id, exchange string) *models.Account {
	a.mux.RLock()
	defer a.mux.RUnlock()

	return a.data[id]
}
