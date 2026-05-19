package modules
import (
	"context"
	"fmt"
	"nsa/internal/model"
)
type M4Mining struct { deps *ModuleDeps }
func NewM4Mining(deps *ModuleDeps) *M4Mining { return &M4Mining{deps: deps} }
func (m *M4Mining) Name() string { return "M4_MINING" }
func (m *M4Mining) Execute(ctx context.Context, session *model.SLRSession) error {
	fmt.Println(">> [MODUL 4: DATA MINING] Mengeksekusi pencarian ke Scopus/IEEE & Deduplikasi...")
	session.Status = "M5_SCREENING"
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}
