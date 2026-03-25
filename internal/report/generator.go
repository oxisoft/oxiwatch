package report

import (
	"bytes"
	"fmt"
	"html"
	"time"

	"github.com/oxisoft/oxiwatch/internal/storage"
	"github.com/oxisoft/oxiwatch/internal/version"
)

type Generator struct {
	storage        *storage.Storage
	serverName     string
	currentVersion string
}

func NewGenerator(storage *storage.Storage, serverName, currentVersion string) *Generator {
	return &Generator{
		storage:        storage,
		serverName:     serverName,
		currentVersion: currentVersion,
	}
}

func (g *Generator) GenerateDailyReport(date time.Time) (string, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)
	_ = endOfDay

	stats, err := g.storage.GetFailedStats(startOfDay)
	if err != nil {
		return "", err
	}

	topUsers, err := g.storage.GetTopUsernames(startOfDay, 10)
	if err != nil {
		return "", err
	}

	topIPs, err := g.storage.GetTopIPs(startOfDay, 10)
	if err != nil {
		return "", err
	}

	successCount, err := g.storage.GetSuccessCount(startOfDay)
	if err != nil {
		return "", err
	}

	reportText := g.formatReport(date, stats, topUsers, topIPs, successCount)

	if g.currentVersion != "" {
		reportText += g.checkVersionUpdate()
	}

	return reportText, nil
}

func (g *Generator) formatReport(date time.Time, stats *storage.Stats, topUsers []storage.UsernameCount, topIPs []storage.IPCount, successCount int) string {
	var buf bytes.Buffer

	buf.WriteString("📊 <b>Daily SSH Report</b>\n")
	buf.WriteString(fmt.Sprintf("🖥️ Server: %s\n", html.EscapeString(g.serverName)))
	buf.WriteString(fmt.Sprintf("📅 %s\n\n", date.Format("2006-01-02")))

	buf.WriteString("📈 <b>Summary</b>\n")
	buf.WriteString(fmt.Sprintf("• Successful logins: %s\n", formatNumber(successCount)))
	buf.WriteString(fmt.Sprintf("• Failed attempts: %s\n", formatNumber(stats.TotalAttempts)))
	buf.WriteString(fmt.Sprintf("• Unique IPs: %s\n", formatNumber(stats.UniqueIPs)))
	buf.WriteString(fmt.Sprintf("• Unique usernames: %s\n\n", formatNumber(stats.UniqueUsernames)))

	if len(topUsers) > 0 {
		buf.WriteString("👤 <b>Top 10 Usernames</b>\n")
		for i, u := range topUsers {
			buf.WriteString(fmt.Sprintf("%d. %s - %s\n", i+1, html.EscapeString(u.Username), formatNumber(u.Count)))
		}
		buf.WriteString("\n")
	}

	if len(topIPs) > 0 {
		buf.WriteString("🌐 <b>Top 10 IPs</b>\n")
		for i, ip := range topIPs {
			location := formatLocation(ip.Country, ip.City)
			if location != "" {
				buf.WriteString(fmt.Sprintf("%d. %s (%s) - %s\n", i+1, html.EscapeString(ip.IP), html.EscapeString(location), formatNumber(ip.Count)))
			} else {
				buf.WriteString(fmt.Sprintf("%d. %s - %s\n", i+1, html.EscapeString(ip.IP), formatNumber(ip.Count)))
			}
		}
	}

	return buf.String()
}

func (g *Generator) GenerateStats(days int) (string, error) {
	since := time.Now().AddDate(0, 0, -days)

	stats, err := g.storage.GetOverallStats(since)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("SSH Statistics (last %d days)\n", days))
	buf.WriteString(fmt.Sprintf("Server: %s\n\n", g.serverName))
	buf.WriteString(fmt.Sprintf("Successful logins: %d\n", stats.SuccessCount))
	buf.WriteString(fmt.Sprintf("Failed attempts: %d\n", stats.FailedCount))
	buf.WriteString(fmt.Sprintf("Unique IPs: %d\n", stats.UniqueIPs))
	buf.WriteString(fmt.Sprintf("Unique usernames: %d\n", stats.UniqueUsernames))

	return buf.String(), nil
}

func (g *Generator) GenerateLoginsReport(days int) (string, error) {
	since := time.Now().AddDate(0, 0, -days)
	logins, err := g.storage.GetSuccessfulLogins(since)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("Successful SSH Logins (last %d days)\n", days))
	buf.WriteString(fmt.Sprintf("Server: %s\n\n", g.serverName))

	if len(logins) == 0 {
		buf.WriteString("No successful logins in this period.\n")
		return buf.String(), nil
	}

	for _, login := range logins {
		location := formatLocation(login.Country, login.City)
		if location != "" {
			buf.WriteString(fmt.Sprintf("%s  %-15s  %-12s  %s (%s)\n",
				login.Timestamp.Format("2006-01-02 15:04:05"),
				login.Username,
				login.Method,
				login.IP,
				location,
			))
		} else {
			buf.WriteString(fmt.Sprintf("%s  %-15s  %-12s  %s\n",
				login.Timestamp.Format("2006-01-02 15:04:05"),
				login.Username,
				login.Method,
				login.IP,
			))
		}
	}

	return buf.String(), nil
}

func formatLocation(country, city string) string {
	if city != "" && country != "" {
		return fmt.Sprintf("%s, %s", city, country)
	}
	if country != "" {
		return country
	}
	return city
}

func formatNumber(n int) string {
	s := fmt.Sprintf("%d", n)
	var result bytes.Buffer
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result.WriteRune(',')
		}
		result.WriteRune(c)
	}
	return result.String()
}

func (g *Generator) checkVersionUpdate() string {
	checker := version.NewChecker(g.currentVersion)
	available, latest, err := checker.IsUpdateAvailable()
	if err != nil {
		return ""
	}

	if !available {
		return ""
	}

	var buf bytes.Buffer
	buf.WriteString("\n⬆️ <b>Update Available</b>\n")
	buf.WriteString(fmt.Sprintf("Current: %s | Latest: %s\n", html.EscapeString(g.currentVersion), html.EscapeString(latest)))
	buf.WriteString("Run: <code>sudo oxiwatch upgrade</code>\n")
	return buf.String()
}
