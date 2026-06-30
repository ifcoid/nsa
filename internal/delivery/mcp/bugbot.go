package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"nsa/internal/model"
	"nsa/internal/repository"
)

// BugBotMCPServer exposes Telegram-based bug reporting tools via MCP,
// backed by MongoDB for persistent storage of tickets and poll state.
type BugBotMCPServer struct {
	token     string
	repo      *repository.MongoRepository
	MCPServer *server.MCPServer
}

// NewBugBotMCPServer creates a new BugBot MCP server with the given Telegram bot
// token and MongoDB repository. It registers all bugbot MCP tools.
func NewBugBotMCPServer(token string, repo *repository.MongoRepository) *BugBotMCPServer {
	mcpSrv := server.NewMCPServer(
		"BugBot MCP",
		"1.0.0",
		server.WithLogging(),
	)

	s := &BugBotMCPServer{
		token:     token,
		repo:      repo,
		MCPServer: mcpSrv,
	}

	s.registerTools()
	return s
}

func (s *BugBotMCPServer) registerTools() {
	// Tool 1: bugbot_poll
	s.MCPServer.AddTool(mcp.NewTool("bugbot_poll",
		mcp.WithDescription("Poll Telegram getUpdates API for new bug reports. Saves new tickets to MongoDB, sends auto-reply confirmations, and advances the poll offset."),
	), s.handlePoll)

	// Tool 2: bugbot_get_unresolved
	s.MCPServer.AddTool(mcp.NewTool("bugbot_get_unresolved",
		mcp.WithDescription("Get unresolved bug tickets from MongoDB (status != 'closed' and status != 'resolved')."),
	), s.handleGetUnresolved)

	// Tool 3: bugbot_reply
	s.MCPServer.AddTool(mcp.NewTool("bugbot_reply",
		mcp.WithDescription("Send a reply message to a specific bug ticket reporter via Telegram. Saves the reply to the ticket's replies array and updates status to 'in_progress' if currently 'open'."),
		mcp.WithString("ticket_id", mcp.Required(), mcp.Description("Ticket ObjectID (hex) or 'last' to reply to the most recent ticket")),
		mcp.WithString("message", mcp.Required(), mcp.Description("Message text to send as reply")),
	), s.handleReply)

	// Tool 4: bugbot_mark_resolved
	s.MCPServer.AddTool(mcp.NewTool("bugbot_mark_resolved",
		mcp.WithDescription("Mark a bug ticket as 'resolved'. Sets status and resolved_at timestamp."),
		mcp.WithString("ticket_id", mcp.Required(), mcp.Description("Ticket ObjectID (hex) or 'last' to resolve the most recent ticket")),
	), s.handleMarkResolved)

	// Tool 5: bugbot_close
	s.MCPServer.AddTool(mcp.NewTool("bugbot_close",
		mcp.WithDescription("Mark a bug ticket as 'closed'."),
		mcp.WithString("ticket_id", mcp.Required(), mcp.Description("Ticket ObjectID (hex) or 'last' to close the most recent ticket")),
	), s.handleClose)
}

// handlePoll calls Telegram getUpdates, processes messages, saves tickets to MongoDB,
// sends auto-reply confirmations, and updates the poll offset.
func (s *BugBotMCPServer) handlePoll(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	api := fmt.Sprintf("https://api.telegram.org/bot%s", s.token)

	// Get current offset from MongoDB
	offset, err := s.repo.GetPollOffset(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get poll offset: %v", err)), nil
	}

	// Call Telegram getUpdates
	updateURL := fmt.Sprintf("%s/getUpdates?offset=%d&timeout=2", api, offset)
	resp, err := httpGetWithContext(ctx, updateURL)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to call getUpdates: %v", err)), nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to read getUpdates response: %v", err)), nil
	}

	var tgResp struct {
		OK     bool              `json:"ok"`
		Result []json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(body, &tgResp); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to parse getUpdates response: %v", err)), nil
	}
	if !tgResp.OK {
		return mcp.NewToolResultError(fmt.Sprintf("Telegram API returned ok=false: %s", string(body))), nil
	}

	if len(tgResp.Result) == 0 {
		return mcp.NewToolResultText("No new messages."), nil
	}

	processed := 0
	for _, raw := range tgResp.Result {
		var upd struct {
			UpdateID int `json:"update_id"`
			Message  *struct {
				Chat struct {
					ID int64 `json:"id"`
				} `json:"chat"`
				From *struct {
					FirstName string `json:"first_name"`
					Username  string `json:"username"`
				} `json:"from"`
				Text     string `json:"text"`
				Caption  string `json:"caption"`
				Date     int64  `json:"date"`
				Document *struct {
					FileID   string `json:"file_id"`
					FileName string `json:"file_name"`
				} `json:"document"`
			} `json:"message"`
		}
		if err := json.Unmarshal(raw, &upd); err != nil {
			continue
		}

		// Advance offset regardless of message type
		newOffset := int64(upd.UpdateID + 1)
		if err := s.repo.SetPollOffset(ctx, newOffset); err != nil {
			log.Printf("[bugbot] failed to update offset: %v", err)
		}

		if upd.Message == nil {
			continue
		}

		msg := upd.Message
		chatID := msg.Chat.ID
		name := ""
		username := ""
		if msg.From != nil {
			name = msg.From.FirstName
			username = msg.From.Username
		}

		// Build and save ticket to MongoDB
		ticket := &model.BugTicket{
			TelegramUpdateID: upd.UpdateID,
			ChatID:           chatID,
			ReporterName:     name,
			ReporterUsername: username,
			MessageText:      msg.Text,
			Caption:          msg.Caption,
			Status:           "open",
		}

		if msg.Document != nil {
			ticket.FileName = msg.Document.FileName
			ticket.FileID = msg.Document.FileID
		}

		if err := s.repo.CreateBugTicket(ctx, ticket); err != nil {
			log.Printf("[bugbot] failed to save ticket: %v", err)
			continue
		}

		// Auto-reply confirmation
		if chatID != 0 {
			var reply string
			if msg.Document != nil || (msg.Text != "" && !strings.HasPrefix(msg.Text, "/start")) {
				reply = fmt.Sprintf("Laporan bug DITERIMA, terima kasih%s! Developer akan memeriksanya. (Ref #%s)", nameStr(name), ticket.ID.Hex())
			} else {
				reply = fmt.Sprintf("Halo%s! Lampirkan FILE laporan bug di sini (dari tombol Report Bug aplikasi), atau ketik detailnya. Saya teruskan ke developer.", nameStr(name))
			}
			if err := s.sendMessage(ctx, api, chatID, reply); err != nil {
				log.Printf("[bugbot] auto-reply failed for chat %d: %v", chatID, err)
			}
		}

		processed++
	}

	return mcp.NewToolResultText(fmt.Sprintf("Processed %d update(s).", processed)), nil
}

// handleGetUnresolved returns all tickets that are not closed or resolved.
func (s *BugBotMCPServer) handleGetUnresolved(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	tickets, err := s.repo.GetUnresolvedTickets(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to query unresolved tickets: %v", err)), nil
	}

	if len(tickets) == 0 {
		return mcp.NewToolResultText("Tidak ada laporan bug yang belum diselesaikan."), nil
	}

	// Format tickets as JSON for readability
	data, err := json.MarshalIndent(tickets, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal tickets: %v", err)), nil
	}

	return mcp.NewToolResultText(string(data)), nil
}

// handleReply sends a reply to a ticket's reporter and saves it to the ticket.
func (s *BugBotMCPServer) handleReply(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		return mcp.NewToolResultError("invalid arguments"), nil
	}

	ticketID, _ := args["ticket_id"].(string)
	if ticketID == "" {
		return mcp.NewToolResultError("ticket_id is required"), nil
	}

	message, _ := args["message"].(string)
	if message == "" {
		return mcp.NewToolResultError("message is required"), nil
	}

	// Resolve ticket (supports "last" keyword)
	ticket, err := s.resolveTicket(ctx, ticketID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to find ticket: %v", err)), nil
	}

	// Send message via Telegram
	api := fmt.Sprintf("https://api.telegram.org/bot%s", s.token)
	if err := s.sendMessage(ctx, api, ticket.ChatID, message); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to send message: %v", err)), nil
	}

	// Save reply to ticket
	reply := model.BugTicketReply{
		Text:      message,
		SentAt:    time.Now(),
		Direction: "outbound",
	}
	if err := s.repo.AddReplyToTicket(ctx, ticket.ID.Hex(), reply); err != nil {
		log.Printf("[bugbot] reply saved to Telegram but failed to persist in DB: %v", err)
		return mcp.NewToolResultText(fmt.Sprintf("Balasan terkirim ke chat %d (warning: gagal simpan ke DB: %v)", ticket.ChatID, err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Balasan terkirim ke %s (chat %d), ticket %s status -> in_progress", ticket.ReporterName, ticket.ChatID, ticket.ID.Hex())), nil
}

// handleMarkResolved sets a ticket's status to "resolved".
func (s *BugBotMCPServer) handleMarkResolved(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		return mcp.NewToolResultError("invalid arguments"), nil
	}

	ticketID, _ := args["ticket_id"].(string)
	if ticketID == "" {
		return mcp.NewToolResultError("ticket_id is required"), nil
	}

	ticket, err := s.resolveTicket(ctx, ticketID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to find ticket: %v", err)), nil
	}

	if err := s.repo.UpdateTicketStatus(ctx, ticket.ID.Hex(), "resolved"); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to update ticket status: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Ticket %s ditandai sebagai resolved.", ticket.ID.Hex())), nil
}

// handleClose sets a ticket's status to "closed".
func (s *BugBotMCPServer) handleClose(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		return mcp.NewToolResultError("invalid arguments"), nil
	}

	ticketID, _ := args["ticket_id"].(string)
	if ticketID == "" {
		return mcp.NewToolResultError("ticket_id is required"), nil
	}

	ticket, err := s.resolveTicket(ctx, ticketID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to find ticket: %v", err)), nil
	}

	if err := s.repo.UpdateTicketStatus(ctx, ticket.ID.Hex(), "closed"); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to update ticket status: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Ticket %s ditandai sebagai closed.", ticket.ID.Hex())), nil
}

// --- Helper functions ---

// resolveTicket retrieves a ticket by ID or returns the last ticket if id is "last".
func (s *BugBotMCPServer) resolveTicket(ctx context.Context, id string) (*model.BugTicket, error) {
	if strings.ToLower(id) == "last" {
		return s.repo.GetLastTicket(ctx)
	}
	return s.repo.GetTicketByID(ctx, id)
}

func nameStr(name string) string {
	if name == "" {
		return ""
	}
	return " " + name
}

func (s *BugBotMCPServer) sendMessage(ctx context.Context, api string, chatID int64, text string) error {
	payload, _ := json.Marshal(map[string]interface{}{
		"chat_id": chatID,
		"text":    text,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", api+"/sendMessage", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		OK bool `json:"ok"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode sendMessage response: %v", err)
	}
	if !result.OK {
		return fmt.Errorf("sendMessage returned ok=false")
	}
	return nil
}

func httpGetWithContext(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	return client.Do(req)
}
