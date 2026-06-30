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
		mcp.WithDescription("Mark a bug ticket as 'resolved'. Sets status and resolved_at timestamp. Optionally records fix_commit and fix_repo for deploy verification."),
		mcp.WithString("ticket_id", mcp.Required(), mcp.Description("Ticket ObjectID (hex) or 'last' to resolve the most recent ticket")),
		mcp.WithString("fix_commit", mcp.Description("SHA of the commit containing the fix (optional, enables deploy verification)")),
		mcp.WithString("fix_repo", mcp.Description("Which repo was fixed: 'nsa' or 'slr' (optional, defaults to 'nsa')")),
	), s.handleMarkResolved)

	// Tool 5: bugbot_close
	s.MCPServer.AddTool(mcp.NewTool("bugbot_close",
		mcp.WithDescription("Mark a bug ticket as 'closed'."),
		mcp.WithString("ticket_id", mcp.Required(), mcp.Description("Ticket ObjectID (hex) or 'last' to close the most recent ticket")),
	), s.handleClose)

	// Tool 6: bugbot_check_stale
	s.MCPServer.AddTool(mcp.NewTool("bugbot_check_stale",
		mcp.WithDescription("Check for tickets that have been 'resolved' for more than 7 days but haven't been deployed yet. Helps flag potential CI failures or forgotten deploys."),
	), s.handleCheckStale)
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

	pollResult := fmt.Sprintf("Processed %d update(s).", processed)

	// --- Deploy verification: check resolved tickets awaiting deploy ---
	deployResult := s.checkDeployAndNotify(ctx)
	if deployResult != "" {
		pollResult += "\n" + deployResult
	}

	return mcp.NewToolResultText(pollResult), nil
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

// handleMarkResolved sets a ticket's status to "resolved" and optionally records fix info.
func (s *BugBotMCPServer) handleMarkResolved(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		return mcp.NewToolResultError("invalid arguments"), nil
	}

	ticketID, _ := args["ticket_id"].(string)
	if ticketID == "" {
		return mcp.NewToolResultError("ticket_id is required"), nil
	}

	fixCommit, _ := args["fix_commit"].(string)
	fixRepo, _ := args["fix_repo"].(string)

	ticket, err := s.resolveTicket(ctx, ticketID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to find ticket: %v", err)), nil
	}

	if err := s.repo.UpdateTicketStatus(ctx, ticket.ID.Hex(), "resolved"); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to update ticket status: %v", err)), nil
	}

	// Save fix_commit and fix_repo if provided
	if fixCommit != "" {
		if fixRepo == "" {
			fixRepo = "nsa"
		}
		if err := s.repo.UpdateTicketFixInfo(ctx, ticket.ID.Hex(), fixCommit, fixRepo); err != nil {
			log.Printf("[bugbot] failed to save fix info for ticket %s: %v", ticket.ID.Hex(), err)
		}
	}

	result := fmt.Sprintf("Ticket %s ditandai sebagai resolved.", ticket.ID.Hex())
	if fixCommit != "" {
		result += fmt.Sprintf(" Fix commit: %s (repo: %s). Deploy verification will run on next poll.", fixCommit, fixRepo)
	}
	return mcp.NewToolResultText(result), nil
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

// --- Deploy verification and notification ---

// checkDeployAndNotify verifies deployment status for resolved tickets and sends notifications.
// This is best-effort: failures are logged but don't block the poll.
func (s *BugBotMCPServer) checkDeployAndNotify(ctx context.Context) string {
	var results []string

	// Step 1: Check resolved tickets awaiting deploy
	tickets, err := s.repo.GetResolvedTicketsAwaitingDeploy(ctx)
	if err != nil {
		log.Printf("[bugbot] deploy check: failed to query resolved tickets: %v", err)
	} else {
		for _, ticket := range tickets {
			deployed, err := s.verifyDeploy(ctx, ticket)
			if err != nil {
				log.Printf("[bugbot] deploy check: failed for ticket %s: %v", ticket.ID.Hex(), err)
				continue
			}
			if deployed {
				// Mark as deployed
				if err := s.repo.MarkTicketDeployed(ctx, ticket.ID.Hex()); err != nil {
					log.Printf("[bugbot] deploy check: failed to mark ticket %s as deployed: %v", ticket.ID.Hex(), err)
					continue
				}
				// Send notification immediately
				if err := s.sendDeployNotification(ctx, &ticket); err != nil {
					log.Printf("[bugbot] deploy check: notification failed for ticket %s: %v", ticket.ID.Hex(), err)
					if markErr := s.repo.MarkTicketNotifyFailed(ctx, ticket.ID.Hex(), true); markErr != nil {
						log.Printf("[bugbot] deploy check: failed to mark notify_failed for ticket %s: %v", ticket.ID.Hex(), markErr)
					}
					results = append(results, fmt.Sprintf("Deploy confirmed for #%s but notification failed", ticket.ID.Hex()))
				} else {
					// Notification sent successfully, close the ticket
					if err := s.repo.UpdateTicketStatus(ctx, ticket.ID.Hex(), "closed"); err != nil {
						log.Printf("[bugbot] deploy check: failed to close ticket %s: %v", ticket.ID.Hex(), err)
					}
					results = append(results, fmt.Sprintf("Deploy confirmed and notified for #%s -> closed", ticket.ID.Hex()))
				}
			}
		}
	}

	// Step 2: Retry notifications for deployed tickets that failed to notify
	pendingTickets, err := s.repo.GetTicketsPendingNotification(ctx)
	if err != nil {
		log.Printf("[bugbot] notify retry: failed to query pending notifications: %v", err)
	} else {
		for _, ticket := range pendingTickets {
			if !ticket.NotifyFailed {
				// This is a deployed ticket that hasn't been notified yet (shouldn't normally happen)
				// Try to notify
			}
			if err := s.sendDeployNotification(ctx, &ticket); err != nil {
				log.Printf("[bugbot] notify retry: failed for ticket %s: %v", ticket.ID.Hex(), err)
				continue
			}
			// Notification succeeded, clear flag and close
			if err := s.repo.MarkTicketNotifyFailed(ctx, ticket.ID.Hex(), false); err != nil {
				log.Printf("[bugbot] notify retry: failed to clear notify_failed for ticket %s: %v", ticket.ID.Hex(), err)
			}
			if err := s.repo.UpdateTicketStatus(ctx, ticket.ID.Hex(), "closed"); err != nil {
				log.Printf("[bugbot] notify retry: failed to close ticket %s: %v", ticket.ID.Hex(), err)
			} else {
				results = append(results, fmt.Sprintf("Retry notification succeeded for #%s -> closed", ticket.ID.Hex()))
			}
		}
	}

	return strings.Join(results, "; ")
}

// verifyDeploy checks whether a ticket's fix has been deployed via GitHub Pages.
func (s *BugBotMCPServer) verifyDeploy(ctx context.Context, ticket model.BugTicket) (bool, error) {
	if ticket.FixCommit == "" {
		return false, nil
	}

	repo := ticket.FixRepo
	if repo == "" {
		repo = "nsa"
	}

	if repo == "slr" {
		return s.verifySLRDeploy(ctx, ticket.FixCommit)
	}
	return s.verifyNSADeploy(ctx, ticket.FixCommit)
}

// verifyNSADeploy checks if the NSA fix commit has been deployed.
// Flow: check ifcoid/download pages build is "built", then verify the latest commit
// on main contains the fix_commit SHA in its message.
func (s *BugBotMCPServer) verifyNSADeploy(ctx context.Context, fixCommit string) (bool, error) {
	// Check if GitHub Pages build is "built"
	pagesBuild, err := s.getPagesBuild(ctx, "ifcoid/download")
	if err != nil {
		return false, fmt.Errorf("failed to check pages build: %w", err)
	}
	if pagesBuild.Status != "built" {
		return false, nil
	}

	// Check if the latest commit on main of ifcoid/download contains the fix_commit SHA
	commitMsg, err := s.getCommitMessage(ctx, "ifcoid/download", "main")
	if err != nil {
		return false, fmt.Errorf("failed to check commit message: %w", err)
	}

	// The compile workflow creates commit message: "Update SLR backend binaries (commit: <NSA_SHA>)"
	return strings.Contains(commitMsg, fixCommit), nil
}

// verifySLRDeploy checks if the SLR fix commit has been deployed.
// For SLR: check if the Pages build commit SHA matches or is newer than fix_commit.
func (s *BugBotMCPServer) verifySLRDeploy(ctx context.Context, fixCommit string) (bool, error) {
	// Check if GitHub Pages build is "built"
	pagesBuild, err := s.getPagesBuild(ctx, "ifcoid/slr")
	if err != nil {
		return false, fmt.Errorf("failed to check pages build: %w", err)
	}
	if pagesBuild.Status != "built" {
		return false, nil
	}

	// For SLR, check if the pages build commit matches or contains the fix commit
	// The pages build commit is the deployed commit - if it matches, it's deployed
	if strings.HasPrefix(pagesBuild.Commit, fixCommit) || strings.HasPrefix(fixCommit, pagesBuild.Commit) {
		return true, nil
	}

	// Also check if the latest commit message on slr/main references the fix commit
	// or if the build happened after the fix (by checking if fix_commit is an ancestor)
	// Simple approach: check the latest main commit
	commitMsg, err := s.getCommitMessage(ctx, "ifcoid/slr", "main")
	if err != nil {
		// If we can't check the commit, just compare SHAs
		return pagesBuild.Commit == fixCommit, nil
	}

	// If the build commit itself is the fix commit or contains reference to it
	return strings.Contains(commitMsg, fixCommit) || pagesBuild.Commit == fixCommit, nil
}

// pagesBuildInfo represents the GitHub Pages build API response.
type pagesBuildInfo struct {
	Status string `json:"status"`
	Commit string `json:"commit"`
}

// getPagesBuild fetches the latest GitHub Pages build status for a repository.
func (s *BugBotMCPServer) getPagesBuild(ctx context.Context, repo string) (*pagesBuildInfo, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/pages/builds/latest", repo)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "nsa-bugbot")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub Pages API returned status %d", resp.StatusCode)
	}

	var build pagesBuildInfo
	if err := json.NewDecoder(resp.Body).Decode(&build); err != nil {
		return nil, fmt.Errorf("failed to decode pages build response: %w", err)
	}
	return &build, nil
}

// getCommitMessage fetches the commit message of the latest commit on a branch.
func (s *BugBotMCPServer) getCommitMessage(ctx context.Context, repo string, branch string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/commits/%s", repo, branch)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "nsa-bugbot")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub Commits API returned status %d", resp.StatusCode)
	}

	var commitResp struct {
		Commit struct {
			Message string `json:"message"`
		} `json:"commit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&commitResp); err != nil {
		return "", fmt.Errorf("failed to decode commit response: %w", err)
	}
	return commitResp.Commit.Message, nil
}

// sendDeployNotification sends a Telegram message to the reporter notifying them of deploy.
func (s *BugBotMCPServer) sendDeployNotification(ctx context.Context, ticket *model.BugTicket) error {
	if ticket.ChatID == 0 {
		return fmt.Errorf("ticket %s has no chat_id", ticket.ID.Hex())
	}

	message := fmt.Sprintf(
		"\u2705 Bug Anda (Ref #%s) sudah diperbaiki dan tersedia di update terbaru. Silakan download di https://if.co.id/download",
		ticket.ID.Hex(),
	)

	api := fmt.Sprintf("https://api.telegram.org/bot%s", s.token)
	return s.sendMessage(ctx, api, ticket.ChatID, message)
}

// handleCheckStale returns tickets that have been "resolved" for more than 7 days without deploy.
func (s *BugBotMCPServer) handleCheckStale(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	staleDuration := 7 * 24 * time.Hour
	tickets, err := s.repo.GetStaleResolvedTickets(ctx, staleDuration)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to query stale tickets: %v", err)), nil
	}

	if len(tickets) == 0 {
		return mcp.NewToolResultText("Tidak ada ticket yang stale (resolved > 7 hari tanpa deploy)."), nil
	}

	var lines []string
	for _, t := range tickets {
		resolvedStr := ""
		if t.ResolvedAt != nil {
			resolvedStr = t.ResolvedAt.Format("2006-01-02 15:04")
		}
		line := fmt.Sprintf("- #%s | Reporter: %s | Resolved: %s | FixCommit: %s | FixRepo: %s",
			t.ID.Hex(), t.ReporterName, resolvedStr, t.FixCommit, t.FixRepo)
		lines = append(lines, line)
	}

	header := fmt.Sprintf("Ditemukan %d ticket stale (resolved > 7 hari tanpa deploy):\n", len(tickets))
	return mcp.NewToolResultText(header + strings.Join(lines, "\n")), nil
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
