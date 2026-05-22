package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type process struct {
	pid     int
	port    int
	command string
	name    string
	started time.Time
	uptime  string
	cpu     string
	mem     string
	rss     string
	orphan  bool
	project string
}

type tickMsg time.Time
type refreshMsg []process

type model struct {
	table    table.Model
	procs    []process
	err      error
	quitting bool
	width    int
	height   int
	status   string
}

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("212")).
			MarginBottom(1)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	killStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("82")).
			Bold(true)

	warnStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("208")).
			Bold(true)

	orphanStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))
)

func scanPorts() []process {
	out, err := exec.Command("lsof", "-iTCP", "-sTCP:LISTEN", "-P", "-n").Output()
	if err != nil {
		return nil
	}

	portRe := regexp.MustCompile(`:(\d+)\s`)
	var procs []process
	seen := make(map[int]bool)

	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 9 || fields[0] == "COMMAND" {
			continue
		}

		pid, err := strconv.Atoi(fields[1])
		if err != nil || seen[pid] {
			continue
		}
		seen[pid] = true

		port := 0
		if m := portRe.FindStringSubmatch(fields[8]); len(m) > 1 {
			port, _ = strconv.Atoi(m[1])
		}

		name := fields[0]
		cmd := getCommand(pid)
		started := getStartTime(pid)
		uptime := formatUptime(started)
		cpu, mem, rss := getResources(pid)
		orphan := isOrphan(pid)
		proj := projectDir(cmd)

		procs = append(procs, process{
			pid:     pid,
			port:    port,
			command: cmd,
			name:    name,
			started: started,
			uptime:  uptime,
			cpu:     cpu,
			mem:     mem,
			rss:     rss,
			orphan:  orphan,
			project: proj,
		})
	}

	sort.Slice(procs, func(i, j int) bool {
		return procs[i].port < procs[j].port
	})
	return procs
}

func getCommand(pid int) string {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=").Output()
	if err != nil {
		return "?"
	}
	return strings.TrimSpace(string(out))
}

func getStartTime(pid int) time.Time {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "lstart=").Output()
	if err != nil {
		return time.Time{}
	}
	t, err := time.Parse("Mon Jan  2 15:04:05 2006", strings.TrimSpace(string(out)))
	if err != nil {
		t, _ = time.Parse("Mon Jan 2 15:04:05 2006", strings.TrimSpace(string(out)))
	}
	return t
}

func getResources(pid int) (cpu, mem, rss string) {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "%cpu=,%mem=,rss=").Output()
	if err != nil {
		return "?", "?", "?"
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) >= 3 {
		cpu = strings.TrimSpace(fields[0]) + "%"
		mem = strings.TrimSpace(fields[1]) + "%"
		kbytes, _ := strconv.ParseFloat(fields[2], 64)
		if kbytes >= 1024*1024 {
			rss = fmt.Sprintf("%.1fG", kbytes/1024/1024)
		} else if kbytes >= 1024 {
			rss = fmt.Sprintf("%.0fM", kbytes/1024)
		} else {
			rss = fmt.Sprintf("%.0fK", kbytes)
		}
	}
	return
}

func isOrphan(pid int) bool {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "ppid=").Output()
	if err != nil {
		return false
	}
	ppid, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	if ppid == 1 {
		return true
	}
	// no controlling terminal
	out2, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "tty=").Output()
	if err != nil {
		return false
	}
	tty := strings.TrimSpace(string(out2))
	return tty == "??" || tty == "-" || tty == ""
}

func formatUptime(started time.Time) string {
	if started.IsZero() {
		return "?"
	}
	d := time.Since(started)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	return fmt.Sprintf("%dd%dh", days, hours)
}

func shortenCommand(cmd string) string {
	if idx := strings.Index(cmd, "node_modules/.bin/"); idx >= 0 {
		parts := strings.SplitN(cmd[idx+len("node_modules/.bin/"):], " ", 2)
		return parts[0]
	}
	if idx := strings.Index(cmd, "node_modules/"); idx >= 0 {
		rest := cmd[idx+len("node_modules/"):]
		parts := strings.SplitN(rest, " ", 2)
		segs := strings.Split(parts[0], "/")
		if len(segs) > 0 {
			return segs[len(segs)-1]
		}
	}

	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return cmd
	}
	segs := strings.Split(parts[0], "/")
	bin := segs[len(segs)-1]
	if len(parts) > 1 {
		return bin + " " + strings.Join(parts[1:], " ")
	}
	return bin
}

func projectDir(cmd string) string {
	home, _ := os.UserHomeDir()
	parts := strings.Fields(cmd)
	for _, p := range parts {
		if strings.Contains(p, "node_modules") {
			idx := strings.Index(p, "node_modules")
			dir := p[:idx]
			if strings.HasSuffix(dir, "/") {
				dir = dir[:len(dir)-1]
			}
			if home != "" && strings.HasPrefix(dir, home) {
				dir = "~" + dir[len(home):]
			}
			return dir
		}
	}
	return ""
}

func projectName(proj string) string {
	if proj == "" {
		return ""
	}
	parts := strings.Split(proj, "/")
	return parts[len(parts)-1]
}

func buildTable(procs []process, width int) table.Model {
	columns := []table.Column{
		{Title: "Port", Width: 6},
		{Title: "PID", Width: 7},
		{Title: "CPU", Width: 6},
		{Title: "Mem", Width: 7},
		{Title: "Uptime", Width: 8},
		{Title: "Status", Width: 8},
		{Title: "Project", Width: 24},
		{Title: "Command", Width: 30},
	}

	if width > 0 {
		fixed := 6 + 7 + 6 + 7 + 8 + 8 + 8
		remaining := width - fixed - 4
		projW := remaining * 35 / 100
		cmdW := remaining - projW
		if projW < 15 {
			projW = 15
		}
		if cmdW < 15 {
			cmdW = 15
		}
		columns[6].Width = projW
		columns[7].Width = cmdW
	}

	var rows []table.Row
	for _, p := range procs {
		status := "✓ ok"
		if p.orphan {
			status = "⚠ orphan"
		}

		rows = append(rows, table.Row{
			strconv.Itoa(p.port),
			strconv.Itoa(p.pid),
			p.cpu,
			p.rss,
			p.uptime,
			status,
			projectName(p.project),
			shortenCommand(p.command),
		})
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(min(len(rows)+1, 20)),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	t.SetStyles(s)

	return t
}

func (m model) countOrphans() int {
	n := 0
	for _, p := range m.procs {
		if p.orphan {
			n++
		}
	}
	return n
}

func (m model) selectedProject() string {
	row := m.table.SelectedRow()
	if row == nil {
		return ""
	}
	pid, _ := strconv.Atoi(row[1])
	for _, p := range m.procs {
		if p.pid == pid {
			return p.project
		}
	}
	return ""
}

func (m model) projectPIDs(proj string) []int {
	if proj == "" {
		return nil
	}
	var pids []int
	for _, p := range m.procs {
		if p.project == proj {
			pids = append(pids, p.pid)
		}
	}
	return pids
}

func killPID(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Signal(syscall.SIGTERM)
}

func doRefresh() tea.Msg {
	return refreshMsg(scanPorts())
}

func tickCmd() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func delayedRefresh() tea.Cmd {
	return func() tea.Msg {
		time.Sleep(500 * time.Millisecond)
		return refreshMsg(scanPorts())
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(doRefresh, tickCmd())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.table = buildTable(m.procs, m.width)
		return m, nil

	case refreshMsg:
		cursor := m.table.Cursor()
		m.procs = []process(msg)
		m.table = buildTable(m.procs, m.width)
		if cursor < len(m.procs) {
			m.table.SetCursor(cursor)
		} else if len(m.procs) > 0 {
			m.table.SetCursor(len(m.procs) - 1)
		}
		return m, nil

	case tickMsg:
		return m, tea.Batch(doRefresh, tickCmd())

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("q", "ctrl+c"))):
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, key.NewBinding(key.WithKeys("k", "delete", "x"))):
			row := m.table.SelectedRow()
			if row == nil {
				return m, nil
			}
			pid, _ := strconv.Atoi(row[1])
			if pid > 0 {
				if err := killPID(pid); err != nil {
					m.status = killStyle.Render(fmt.Sprintf("  Failed to kill PID %d: %v", pid, err))
				} else {
					m.status = successStyle.Render(fmt.Sprintf("  Killed PID %d (port %s)", pid, row[0]))
				}
				return m, tea.Batch(delayedRefresh(), tickCmd())
			}
			return m, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("K"))):
			proj := m.selectedProject()
			if proj == "" {
				m.status = dimStyle.Render("  No project detected for this process")
				return m, nil
			}
			pids := m.projectPIDs(proj)
			if len(pids) <= 1 {
				m.status = dimStyle.Render("  Only one process for this project")
				return m, nil
			}
			killed := 0
			for _, pid := range pids {
				if killPID(pid) == nil {
					killed++
				}
			}
			m.status = successStyle.Render(
				fmt.Sprintf("  Killed %d/%d processes for %s", killed, len(pids), projectName(proj)),
			)
			return m, tea.Batch(delayedRefresh(), tickCmd())

		case key.Matches(msg, key.NewBinding(key.WithKeys("O"))):
			killed := 0
			total := 0
			for _, p := range m.procs {
				if p.orphan {
					total++
					if killPID(p.pid) == nil {
						killed++
					}
				}
			}
			if total == 0 {
				m.status = dimStyle.Render("  No orphan processes found")
			} else {
				m.status = successStyle.Render(
					fmt.Sprintf("  Killed %d/%d orphan processes", killed, total),
				)
			}
			return m, tea.Batch(delayedRefresh(), tickCmd())

		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			m.status = ""
			return m, doRefresh
		}
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m model) View() string {
	if m.quitting {
		return ""
	}

	orphans := m.countOrphans()
	title := titleStyle.Render(fmt.Sprintf("  Listening Ports (%d)", len(m.procs)))
	if orphans > 0 {
		title += "  " + orphanStyle.Render(fmt.Sprintf("⚠ %d orphan(s)", orphans))
	}

	var statusLine string
	if m.status != "" {
		statusLine = "\n" + m.status
	}

	help := helpStyle.Render("  ↑/↓ navigate  k kill  K kill project  O kill orphans  r refresh  q quit")

	return fmt.Sprintf("\n%s\n%s%s\n\n%s\n", title, m.table.View(), statusLine, help)
}

func main() {
	p := tea.NewProgram(model{}, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
