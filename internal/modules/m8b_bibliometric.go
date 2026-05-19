package modules
import (
	"context"
	"fmt"
	"nsa/internal/model"
)
type M8bBibliometric struct { deps *ModuleDeps }
func NewM8bBibliometric(deps *ModuleDeps) *M8bBibliometric { return &M8bBibliometric{deps: deps} }
func (m *M8bBibliometric) Name() string { return "M8B_BIBLIO" }
func (m *M8bBibliometric) Execute(ctx context.Context, session *model.SLRSession) error {
	fmt.Println(">> [MODUL 8b: BIBLIOMETRIC] Analisis opsional menggunakan SLNA & VOSviewer...")
	session.Status = "M9_MANUSCRIPT"
	return m.deps.MongoRepo.UpdateSession(ctx, session)
}
