// Package statusline renders the human-readable status line text IA METER
// prints to stdout when installed as (or chained from) Claude Code's
// statusLine command. It never touches the network.
package statusline

import (
	"fmt"
	"math"
	"strings"

	"github.com/valleedev/iameter-collector/internal/model"
)

// Render builds the "IA METER · 5h 68% · 7d 54%" line (section 12),
// degrading gracefully when one or both windows are absent.
func Render(rl model.RateLimits) string {
	var parts []string
	if rl.FiveHour != nil {
		parts = append(parts, fmt.Sprintf("5h %.0f%%", math.Round(rl.FiveHour.UsedPercentage)))
	}
	if rl.SevenDay != nil {
		parts = append(parts, fmt.Sprintf("7d %.0f%%", math.Round(rl.SevenDay.UsedPercentage)))
	}
	if len(parts) == 0 {
		return "IA METER · Consumo no disponible"
	}
	return "IA METER · " + strings.Join(parts, " · ")
}
