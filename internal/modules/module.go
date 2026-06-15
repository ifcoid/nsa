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

// ProposalModule adalah kontrak untuk setiap tahap pipeline proposal (P0, P1, dst)
type ProposalModule interface {
	Execute(ctx context.Context, session *model.ProposalSession) error
	Name() string
}

// ModuleDeps berisi dependency injection yang dibutuhkan oleh setiap modul
type ModuleDeps struct {
	MongoRepo    *repository.MongoRepository
	Neo4jRepo    *repository.Neo4jRepository
	Neo4jConnErr string // menyimpan error koneksi Neo4j saat startup untuk diagnostik
	LLMFactory   *llm.LLMFactory
}
