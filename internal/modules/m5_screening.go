package modules
import (
	"context"
	"fmt"
	"nsa/internal/model"
)
type M5Screening struct { deps *ModuleDeps }
func NewM5Screening(deps *ModuleDeps) *M5Screening { return &M5Screening{deps: deps} }
func (m *M5Screening) Name() string { return "M5_SCREENING" }
func (m *M5Screening) Execute(ctx context.Context, session *model.SLRSession) error {
	fmt.Println(">> [MODUL 5: SCREENING] Melakukan kalibrasi Kappa & Batch Screening massal...")
	session.Status = "M6_FULLTEXT"
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}
