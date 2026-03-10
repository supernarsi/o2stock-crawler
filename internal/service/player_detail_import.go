package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"o2stock-crawler/internal/db/repositories"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type playerDetailResponse struct {
	Data []struct {
		PlayerTemplateID string `json:"PlayerTemplateId"`
	} `json:"data"`
}

type playerDetailImportStats struct {
	Updated  int
	NotFound int
	Invalid  int
}

// ImportPlayerDetailJSONFromDir 解析 data 目录中的文件，并将完整 JSON 写入 players.detail_json。
func (s *PlayersService) ImportPlayerDetailJSONFromDir(ctx context.Context, dataDir string) (playerDetailImportStats, error) {
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return playerDetailImportStats{}, fmt.Errorf("read data dir: %w", err)
	}

	repo := repositories.NewPlayerRepository(s.db.DB)
	detailsByPlayerID := make(map[uint]string)
	stats := playerDetailImportStats{}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		filePath := filepath.Join(dataDir, entry.Name())
		raw, readErr := os.ReadFile(filePath)
		if readErr != nil {
			return stats, fmt.Errorf("read file %s: %w", filePath, readErr)
		}

		blocks := extractJSONObjectBlocks(string(raw))
		for _, block := range blocks {
			playerID, parseErr := parsePlayerIDFromDetailJSON(block)
			if parseErr != nil {
				stats.Invalid++
				continue
			}
			detailsByPlayerID[playerID] = block
		}
	}

	for playerID, detailJSON := range detailsByPlayerID {
		rows, err := repo.UpdateDetailJSON(ctx, playerID, detailJSON)
		if err != nil {
			return stats, fmt.Errorf("update detail_json for player_id=%d: %w", playerID, err)
		}
		if rows == 0 {
			stats.NotFound++
			continue
		}
		stats.Updated++
	}

	log.Printf("detail_json 导入完成: updated=%d, not_found=%d, invalid=%d", stats.Updated, stats.NotFound, stats.Invalid)
	return stats, nil
}

func parsePlayerIDFromDetailJSON(raw string) (uint, error) {
	var resp playerDetailResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return 0, fmt.Errorf("unmarshal detail json: %w", err)
	}
	if len(resp.Data) == 0 {
		return 0, fmt.Errorf("empty data")
	}
	playerID, err := strconv.ParseUint(strings.TrimSpace(resp.Data[0].PlayerTemplateID), 10, 64)
	if err != nil || playerID == 0 {
		return 0, fmt.Errorf("invalid player template id: %s", resp.Data[0].PlayerTemplateID)
	}
	return uint(playerID), nil
}

func extractJSONObjectBlocks(text string) []string {
	out := make([]string, 0)
	bytes := []byte(text)
	for i := 0; i < len(bytes); i++ {
		if bytes[i] != '{' {
			continue
		}
		end, ok := findJSONObjectEnd(bytes, i)
		if !ok {
			break
		}
		out = append(out, string(bytes[i:end+1]))
		i = end
	}
	return out
}

func findJSONObjectEnd(data []byte, start int) (int, bool) {
	depth := 0
	inString := false
	escaped := false

	for i := start; i < len(data); i++ {
		ch := data[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}

		if ch == '"' {
			inString = true
			continue
		}
		if ch == '{' {
			depth++
			continue
		}
		if ch == '}' {
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}
