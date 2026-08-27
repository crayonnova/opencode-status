package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/nova/opencode-status/internal/poller"
	"github.com/nova/opencode-status/internal/storage"
)

type tickMsg time.Time

type Model struct {
	Store    *storage.Store
	Interval time.Duration
	Window   time.Duration

	snapshot *poller.Snapshot
	width    int
	height   int
	err      error
}

func New(store *storage.Store, refresh time.Duration) *Model {
	return &Model{Store: store, Interval: refresh, Window: 24 * time.Hour}
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.tickCmd(), tea.EnterAltScreen)
}

func (m *Model) tickCmd() tea.Cmd {
	return tea.Tick(m.Interval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tickMsg:
		if err := m.refresh(); err != nil {
			m.err = err
		}
		return m, m.tickCmd()
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "r":
			if err := m.refresh(); err != nil {
				m.err = err
			}
			return m, nil
		}
	}
	return m, nil
}

func (m *Model) refresh() error {
	snap, err := poller.SnapshotFromStore(m.Store, m.Window)
	if err != nil {
		return err
	}
	m.snapshot = snap
	m.err = nil
	return nil
}

func (m *Model) View() string {
	if m.snapshot == nil {
		return lipgloss.NewStyle().Padding(1, 2).Render("Loading...")
	}

	var b strings.Builder
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63")).
		Render("opencode free models · uptime")
	subtitle := lipgloss.NewStyle().Faint(true).Render(
		fmt.Sprintf(" updated %s · %d models · window %s · press r to refresh · q to quit",
			m.snapshot.At.Format("15:04:05"), len(m.snapshot.Models), m.Window))
	b.WriteString(title + "\n" + subtitle + "\n\n")

	// Build table.
	headerStyle := lipgloss.NewStyle().Bold(true).Underline(true)
	row := func(cells ...string) string {
		return strings.Join(cells, "  ")
	}
	b.WriteString(headerStyle.Render(row("STATUS", "PROVIDER", "MODEL", "UPTIME", "SAMPLES", "SPARKLINE")) + "\n")

	for _, md := range m.snapshot.Models {
		up := m.snapshot.Uptimes[md.ModelID]
		samples := m.snapshot.Samples[md.ModelID]
		status := "  "
		rowStyle := lipgloss.NewStyle()

		// Highlight rules:
		// - free AND available now  → green dot
		// - free AND unavailable    → red dot + bold (was free, now gone)
		// - paid                    → faint
		if md.IsFree {
			if md.Available {
				status = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("● FREE")
				rowStyle = rowStyle.Foreground(lipgloss.Color("15"))
			} else {
				status = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Render("✗ GONE")
				rowStyle = rowStyle.Foreground(lipgloss.Color("203"))
			}
		} else {
			status = lipgloss.NewStyle().Faint(true).Render("· paid")
			rowStyle = rowStyle.Faint(true)
		}

		upStr := "  -  "
		if samples > 0 {
			upStr = fmt.Sprintf("%5.1f%%", up*100)
		}
		samStr := fmt.Sprintf("%5d", samples)
		spark := sparkline(m.Store, md.ModelID, m.Window, 24)

		b.WriteString(rowStyle.Render(row(
			status,
			truncate(md.ProviderID, 14),
			truncate(md.Name, 32),
			upStr,
			samStr,
			spark,
		)) + "\n")
	}

	if m.err != nil {
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("err: "+m.err.Error()))
	}
	return b.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

// sparkline renders 24-bucket presence for the given model within window.
func sparkline(store *storage.Store, modelID string, window time.Duration, buckets int) string {
	hist, err := store.History(modelID, time.Now().Add(-window))
	if err != nil || len(hist) == 0 {
		return strings.Repeat("·", buckets)
	}
	// Bucketize by index.
	type b struct {
		ups, total int
	}
	bs := make([]b, buckets)
	now := time.Now()
	bucketDur := window / time.Duration(buckets)
	for _, h := range hist {
		age := now.Sub(h.CheckedAt)
		idx := buckets - 1 - int(age/bucketDur)
		if idx < 0 {
			idx = 0
		}
		if idx >= buckets {
			idx = buckets - 1
		}
		bs[idx].total++
		if h.Available {
			bs[idx].ups++
		}
	}
	chars := []string{" ", "░", "▒", "▓", "█"}
	var out strings.Builder
	for _, b := range bs {
		if b.total == 0 {
			out.WriteString("·")
			continue
		}
		ratio := float64(b.ups) / float64(b.total)
		idx := int(ratio * float64(len(chars)-1))
		if idx < 0 {
			idx = 0
		}
		if idx > len(chars)-1 {
			idx = len(chars) - 1
		}
		out.WriteString(chars[idx])
	}
	return out.String()
}

func Run(ctx context.Context, store *storage.Store, refresh time.Duration) error {
	m := New(store, refresh)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}
