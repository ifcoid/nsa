package mcp

import (
	"context"
	"fmt"
	"log"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"nsa/internal/repository"
)

type MCPServer struct {
	repo      *repository.MongoRepository
	MCPServer *server.MCPServer
}

func NewMCPServer(repo *repository.MongoRepository) *MCPServer {
	mcpSrv := server.NewMCPServer(
		"NSA Supervisor MCP",
		"1.0.0",
		server.WithLogging(),
	)

	s := &MCPServer{
		repo:      repo,
		MCPServer: mcpSrv,
	}

	s.registerTools()
	return s
}

func (s *MCPServer) registerTools() {
	// Tool 1: get_screener_briefing
	s.MCPServer.AddTool(mcp.NewTool("get_screener_briefing",
		mcp.WithDescription("Mengambil dokumen Screener Briefing (kriteria Inklusi/Eksklusi) dari sesi SLR tertentu."),
		mcp.WithString("session_id", mcp.Required(), mcp.Description("ID sesi SLR")),
	), s.handleGetBriefing)

	// Tool 2: get_pending_disagreements
	s.MCPServer.AddTool(mcp.NewTool("get_pending_disagreements",
		mcp.WithDescription("Mengambil daftar paper yang tertunda/konflik antara R1 dan R2 yang membutuhkan keputusan Supervisor."),
		mcp.WithString("session_id", mcp.Required(), mcp.Description("ID sesi SLR")),
	), s.handleGetPending)

	// Tool 3: submit_supervisor_resolution
	s.MCPServer.AddTool(mcp.NewTool("submit_supervisor_resolution",
		mcp.WithDescription("Mengirimkan keputusan akhir (INCLUDE/EXCLUDE) beserta alasannya untuk suatu paper."),
		mcp.WithString("session_id", mcp.Required(), mcp.Description("ID sesi SLR")),
		mcp.WithString("paper_id", mcp.Required(), mcp.Description("ID Paper yang dikonflik")),
		mcp.WithString("final_decision", mcp.Required(), mcp.Description("Keputusan akhir (harus 'INCLUDE' atau 'EXCLUDE')")),
		mcp.WithString("reasoning", mcp.Required(), mcp.Description("Analisis dan alasan keputusan")),
	), s.handleSubmitResolution)
}

func (s *MCPServer) handleGetBriefing(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		return mcp.NewToolResultError("invalid arguments"), nil
	}
	sessionID, _ := args["session_id"].(string)
	if sessionID == "" {
		return mcp.NewToolResultError("session_id is required"), nil
	}

	session, err := s.repo.GetSession(ctx, sessionID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get session: %v", err)), nil
	}

	if session.ScreenerBriefing == nil {
		return mcp.NewToolResultError("Belum ada Screener Briefing di sesi ini"), nil
	}

	return mcp.NewToolResultText(session.ScreenerBriefing.BriefingDoc), nil
}

func (s *MCPServer) handleGetPending(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		return mcp.NewToolResultError("invalid arguments"), nil
	}
	sessionID, _ := args["session_id"].(string)
	if sessionID == "" {
		return mcp.NewToolResultError("session_id is required"), nil
	}

	// This duplicates the logic from session_handler.go somewhat, but it's okay for an MCP wrapper.
	papers, err := s.repo.GetDisagreedPapers(ctx, sessionID)
	if err != nil || len(papers) == 0 {
		// Fallback for fulltext phase (Modul 6) if no disagreements in screening phase
		papers, err = s.repo.GetDisagreedFullTextPapers(ctx, sessionID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Gagal mengambil disagreements: %v", err)), nil
		}
	}

	if len(papers) == 0 {
		return mcp.NewToolResultText("Tidak ada paper yang membutuhkan resolusi saat ini."), nil
	}

	var output string
	for i, p := range papers {
		idStr := ""
		if oid, ok := p["_id"].(primitive.ObjectID); ok {
			idStr = oid.Hex()
		} else if val, ok := p["_id"]; ok {
			idStr = fmt.Sprintf("%v", val)
		}
		
		output += fmt.Sprintf("=== Kasus %d ===\nPaper ID: %s\nJudul: %s\nAbstrak: %s\nR1 Decision: %v\nR1 Notes: %v\nR2 Decision: %v\nR2 Notes: %v\n\n",
			i+1, idStr, p["title"], p["abstract"], p["Screener_1_Decision"], p["Screener_1_Notes_Full"], p["Screener_2_Decision"], p["Screener_2_Notes_Full"])
	}

	return mcp.NewToolResultText(output), nil
}

func (s *MCPServer) handleSubmitResolution(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		return mcp.NewToolResultError("invalid arguments"), nil
	}
	
	sessionID, _ := args["session_id"].(string)
	if sessionID == "" {
		return mcp.NewToolResultError("session_id is required"), nil
	}

	paperID, _ := args["paper_id"].(string)
	if paperID == "" {
		return mcp.NewToolResultError("paper_id is required"), nil
	}

	decision, _ := args["final_decision"].(string)
	if decision != "INCLUDE" && decision != "EXCLUDE" {
		return mcp.NewToolResultError("final_decision must be 'INCLUDE' or 'EXCLUDE'"), nil
	}

	reasoning, _ := args["reasoning"].(string)
	if reasoning == "" {
		return mcp.NewToolResultError("reasoning is required"), nil
	}

	// Determine if this is a Full-Text (Modul 6) conflict
	isFullText := false
	fullPapers, _ := s.repo.GetDisagreedFullTextPapers(ctx, sessionID)
	for _, p := range fullPapers {
		if oid, ok := p["_id"].(primitive.ObjectID); ok && oid.Hex() == paperID {
			isFullText = true
			break
		} else if val, ok := p["_id"]; ok && fmt.Sprintf("%v", val) == paperID {
			isFullText = true
			break
		}
	}

	var err error
	if isFullText {
		err = s.repo.UpdateScreeningPaperResolutionFull(ctx, sessionID, paperID, decision, "hitl_mcp: "+reasoning)
	} else {
		err = s.repo.UpdateScreeningPaperResolution(ctx, sessionID, paperID, decision, "hitl_mcp: "+reasoning)
	}

	if err != nil {
		log.Printf("MCP resolve error: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("Gagal menyimpan resolusi: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Resolusi %s berhasil disimpan untuk paper %s.", decision, paperID)), nil
}
