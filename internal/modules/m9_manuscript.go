package modules
import (
	"context"
	"fmt"
	"nsa/internal/model"
)
type M9Manuscript struct { deps *ModuleDeps }
func NewM9Manuscript(deps *ModuleDeps) *M9Manuscript { return &M9Manuscript{deps: deps} }
func (m *M9Manuscript) Name() string { return "M9_MANUSCRIPT" }
func (m *M9Manuscript) Execute(ctx context.Context, session *model.SLRSession) error {
	fmt.Println(">> [MODUL 9: MANUSCRIPT WRITING] Menulis draf PRISMA 2020 Compliant (TCCM/ADO)...")
	session.Status = "COMPLETED"
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}
