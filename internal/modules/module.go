package modules

import (
	"context"
	"nsa/internal/llm"
	"nsa/internal/model"
	"nsa/internal/repository"
)

// Module adalah kontrak untuk setiap tahap SLR (M1 sampai M9)
type Module interface {
	Execute(ctx context.Context, session *model.SLRSession) error
	Name() string
}

// ModuleDeps berisi dependency injection yang dibutuhkan oleh setiap modul
type ModuleDeps struct {
	MongoRepo  *repository.MongoRepository
	LLMFactory *llm.LLMFactory
}
