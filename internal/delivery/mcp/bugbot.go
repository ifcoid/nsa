package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// BugBotMCPServer exposes Telegram-based bug reporting tools via MCP.
type BugBotMCPServer struct {
	token     string
	dataDir   string
	mu        sync.Mutex // protects concurrent access to inbox.jsonl, offset, and solved.txt
	MCPServer *server.MCPServer
}

// NewBugBotMCPServer creates a new BugBot MCP server with the given Telegram bot
// token and data directory. It creates necessary directories and registers tools.
// Returns an error if the data directories cannot be created.
func NewBugBotMCPServer(token string, dataDir string) (*BugBotMCPServer, error) {
	// Ensure data directories exist
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create bugbot data directory %s: %w", dataDir, err)
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "files"), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create bugbot files directory: %w", err)
	}

	mcpSrv := server.NewMCPServer(
		"BugBot MCP",
		"1.0.0",
		server.WithLogging(),
	)

	s := &BugBotMCPServer{
		token:     token,
		dataDir:   dataDir,
		MCPServer: mcpSrv,
	}

	s.registerTools()
	return s, nil
}

func (s *BugBotMCPServer) registerTools() {
	// Tool 1: bugbot_poll
	s.MCPServer.AddTool(mcp.NewTool("bugbot_poll",
		mcp.WithDescription("Poll Telegram getUpdates API for new bug reports. Downloads file attachments, logs messages to inbox.jsonl, and sends auto-reply confirmations."),
	), s.handlePoll)

	// Tool 2: bugbot_get_unresolved
	s.MCPServer.AddTool(mcp.NewTool("bugbot_get_unresolved",
		mcp.WithDescription("Get unresolved bug reports from inbox.jsonl (filtered by solved.txt)."),
	), s.handleGetUnresolved)

	// Tool 3: bugbot_reply
	s.MCPServer.AddTool(mcp.NewTool("bugbot_reply",
		mcp.WithDescription("Send a reply message to a Telegram chat via the bug report bot."),
		mcp.WithString("chat_id", mcp.Required(), mcp.Description("Telegram chat ID to reply to")),
		mcp.WithString("message", mcp.Required(), mcp.Description("Message text to send")),
	), s.handleReply)

	// Tool 4: bugbot_mark_solved
	s.MCPServer.AddTool(mcp.NewTool("bugbot_mark_solved",
		mcp.WithDescription("Mark a bug report as solved by appending its update_id to solved.txt."),
		mcp.WithString("update_id", mcp.Required(), mcp.Description("The update_id of the report to mark as solved")),
	), s.handleMarkSolved)
}

// handlePoll calls Telegram getUpdates, processes messages, downloads files,
// logs to inbox.jsonl, sends auto-reply, and updates the offset.
func (s *BugBotMCPServer) handlePoll(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	api := fmt.Sprintf("https://api.telegram.org/bot%s", s.token)

	// Read current offset
	offsetFile := filepath.Join(s.dataDir, "offset")
	offset := 0
	if data, err := os.ReadFile(offsetFile); err == nil {
		if v, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			offset = v
		}
	}

	// Call getUpdates
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

	inboxPath := filepath.Join(s.dataDir, "inbox.jsonl")
	inboxFile, err := os.OpenFile(inboxPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to open inbox.jsonl: %v", err)), nil
	}
	defer inboxFile.Close()

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

		if upd.Message == nil {
			// Update offset even for non-message updates
			os.WriteFile(offsetFile, []byte(strconv.Itoa(upd.UpdateID+1)), 0o644)
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

		// Download file attachment if present
		saved := ""
		if msg.Document != nil && msg.Document.FileID != "" {
			saved = s.downloadFile(ctx, api, upd.UpdateID, msg.Document.FileID, msg.Document.FileName)
		}

		// Log to inbox.jsonl
		entry := map[string]interface{}{
			"update_id": upd.UpdateID,
			"received":  time.Now().UTC().Format(time.RFC3339),
			"chat":      chatID,
			"name":      name,
			"username":  username,
			"text":      msg.Text,
			"caption":   msg.Caption,
			"file":      "",
			"saved":     saved,
			"date":      msg.Date,
		}
		if msg.Document != nil {
			entry["file"] = msg.Document.FileName
		}
		jsonLine, _ := json.Marshal(entry)
		inboxFile.Write(jsonLine)
		inboxFile.Write([]byte("\n"))

		// Auto-reply confirmation
		if chatID != 0 {
			var reply string
			if msg.Document != nil || (msg.Text != "" && !strings.HasPrefix(msg.Text, "/start")) {
				reply = fmt.Sprintf("✅ Laporan bug DITERIMA, terima kasih%s! Developer akan memeriksanya. (Ref #%d)", nameStr(name), upd.UpdateID)
			} else {
				reply = fmt.Sprintf("👋 Halo%s! Lampirkan FILE laporan bug di sini (dari tombol Report Bug aplikasi), atau ketik detailnya. Saya teruskan ke developer.", nameStr(name))
			}
			s.sendMessage(ctx, api, chatID, reply)
		}

		// Update offset
		os.WriteFile(offsetFile, []byte(strconv.Itoa(upd.UpdateID+1)), 0o644)
		processed++
	}

	return mcp.NewToolResultText(fmt.Sprintf("Processed %d update(s).", processed)), nil
}

// handleGetUnresolved reads inbox.jsonl and returns entries NOT in solved.txt.
func (s *BugBotMCPServer) handleGetUnresolved(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	inboxPath := filepath.Join(s.dataDir, "inbox.jsonl")
	solvedPath := filepath.Join(s.dataDir, "solved.txt")

	inboxData, err := os.ReadFile(inboxPath)
	if err != nil {
		if os.IsNotExist(err) {
			return mcp.NewToolResultText("inbox.jsonl kosong"), nil
		}
		return mcp.NewToolResultError(fmt.Sprintf("Failed to read inbox.jsonl: %v", err)), nil
	}

	// Load solved IDs into a set
	solvedSet := make(map[string]bool)
	if solvedData, err := os.ReadFile(solvedPath); err == nil {
		for _, line := range strings.Split(string(solvedData), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				solvedSet[line] = true
			}
		}
	}

	var unresolved []string
	for _, line := range strings.Split(string(inboxData), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Extract update_id from JSON line
		var entry struct {
			UpdateID json.Number `json:"update_id"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		uid := entry.UpdateID.String()
		if uid == "" {
			continue
		}
		if solvedSet[uid] {
			continue
		}
		unresolved = append(unresolved, line)
	}

	if len(unresolved) == 0 {
		return mcp.NewToolResultText("Tidak ada laporan bug yang belum diselesaikan."), nil
	}

	return mcp.NewToolResultText(strings.Join(unresolved, "\n")), nil
}

// handleReply sends a message to a Telegram chat.
func (s *BugBotMCPServer) handleReply(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		return mcp.NewToolResultError("invalid arguments"), nil
	}

	chatIDStr, _ := args["chat_id"].(string)
	if chatIDStr == "" {
		return mcp.NewToolResultError("chat_id is required"), nil
	}
	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid chat_id: %v", err)), nil
	}

	message, _ := args["message"].(string)
	if message == "" {
		return mcp.NewToolResultError("message is required"), nil
	}

	api := fmt.Sprintf("https://api.telegram.org/bot%s", s.token)
	if err := s.sendMessage(ctx, api, chatID, message); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to send message: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Balasan terkirim ke chat %d", chatID)), nil
}

// handleMarkSolved appends an update_id to solved.txt.
func (s *BugBotMCPServer) handleMarkSolved(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		return mcp.NewToolResultError("invalid arguments"), nil
	}

	updateID, _ := args["update_id"].(string)
	if updateID == "" {
		return mcp.NewToolResultError("update_id is required"), nil
	}

	solvedPath := filepath.Join(s.dataDir, "solved.txt")
	f, err := os.OpenFile(solvedPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to open solved.txt: %v", err)), nil
	}
	defer f.Close()

	_, err = f.WriteString(updateID + "\n")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to write to solved.txt: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Update %s ditandai sebagai solved.", updateID)), nil
}

// --- Helper functions ---

func nameStr(name string) string {
	if name == "" {
		return ""
	}
	return " " + name
}

func (s *BugBotMCPServer) downloadFile(ctx context.Context, api string, updateID int, fileID, fileName string) string {
	// Get file path from Telegram
	getFileURL := fmt.Sprintf("%s/getFile?file_id=%s", api, url.QueryEscape(fileID))
	resp, err := httpGetWithContext(ctx, getFileURL)
	if err != nil {
		log.Printf("[bugbot] getFile error: %v", err)
		return ""
	}
	defer resp.Body.Close()

	var fileResp struct {
		OK     bool `json:"ok"`
		Result struct {
			FilePath string `json:"file_path"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&fileResp); err != nil || !fileResp.OK || fileResp.Result.FilePath == "" {
		return ""
	}

	// Download the file
	if fileName == "" {
		fileName = "report.txt"
	}
	// Sanitize fileName to prevent path traversal via malicious Telegram file names.
	fileName = filepath.Base(fileName)
	outPath := filepath.Join(s.dataDir, "files", fmt.Sprintf("%d_%s", updateID, fileName))
	downloadURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", s.token, fileResp.Result.FilePath)

	dlResp, err := httpGetWithContext(ctx, downloadURL)
	if err != nil {
		log.Printf("[bugbot] download error: %v", err)
		return ""
	}
	defer dlResp.Body.Close()

	outFile, err := os.Create(outPath)
	if err != nil {
		log.Printf("[bugbot] create file error: %v", err)
		return ""
	}
	defer outFile.Close()

	if _, err := io.Copy(outFile, dlResp.Body); err != nil {
		log.Printf("[bugbot] write file error: %v", err)
		return ""
	}

	return outPath
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
