package main

import (
	"database/sql"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"unicode/utf8"

	"rehab-app/internal/config"
	dbpkg "rehab-app/internal/db"
)

const (
	nutritionDaysPerWeek              = 7
	nutritionMealSlotsPerDay          = 4
	nutritionHydrationRemindersPerDay = 3
)

type numericStats struct {
	Average float64
	Max     int
}

type parameterRow struct {
	Symbol      string
	Value       string
	Description string
}

type storageRow struct {
	Section string
	Tables  string
	Size    int64
}

type reportStats struct {
	Users               int
	Sessions            int
	MealAssignments     int
	DayProgressRows     int
	HydrationLogs       int
	RewardRequests      int
	SupportTickets      int
	SupportMessages     int
	PointsLedgerEvents  int
	AuditEvents         int
	MealsPerUserDay     numericStats
	HydrationPerUserDay numericStats
	MessagesPerTicket   numericStats
	RewardsPerUser      numericStats
	LedgerEventsPerUser numericStats
	Storage             []storageRow
}

func main() {
	cfg := config.Load()
	db, err := dbpkg.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	stats, err := collectReportStats(db)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Print(renderReport(stats))
}

func collectReportStats(db *sql.DB) (reportStats, error) {
	stats := reportStats{}
	var err error

	if stats.Users, err = queryInt(db, `select count(*) from users`); err != nil {
		return stats, err
	}
	if stats.Sessions, err = queryInt(db, `select count(*) from sessions`); err != nil {
		return stats, err
	}
	if stats.MealAssignments, err = queryInt(db, `select count(*) from nutrition_plan_meals`); err != nil {
		return stats, err
	}
	if stats.DayProgressRows, err = queryInt(db, `select count(*) from nutrition_day_progress`); err != nil {
		return stats, err
	}
	if stats.HydrationLogs, err = queryInt(db, `select count(*) from nutrition_hydration_logs`); err != nil {
		return stats, err
	}
	if stats.RewardRequests, err = queryInt(db, `select count(*) from nutrition_reward_redemptions`); err != nil {
		return stats, err
	}
	if stats.SupportTickets, err = queryInt(db, `select count(*) from support_tickets`); err != nil {
		return stats, err
	}
	if stats.SupportMessages, err = queryInt(db, `select count(*) from support_ticket_messages`); err != nil {
		return stats, err
	}
	if stats.PointsLedgerEvents, err = queryInt(db, `select count(*) from nutrition_points_ledger`); err != nil {
		return stats, err
	}
	if stats.AuditEvents, err = queryInt(db, `select count(*) from nutrition_action_audit`); err != nil {
		return stats, err
	}

	if stats.MealsPerUserDay, err = queryGroupedStats(db, `
		select coalesce(avg(cnt), 0), coalesce(max(cnt), 0)
		from (
			select user_id, day_date, count(*) as cnt
			from nutrition_plan_meals
			group by user_id, day_date
		) q
		where cnt > 0`); err != nil {
		return stats, err
	}

	if stats.HydrationPerUserDay, err = queryGroupedStats(db, `
		select coalesce(avg(cnt), 0), coalesce(max(cnt), 0)
		from (
			select user_id, day_date, count(*) filter (where status = 'completed') as cnt
			from nutrition_hydration_logs
			group by user_id, day_date
		) q
		where cnt > 0`); err != nil {
		return stats, err
	}

	if stats.MessagesPerTicket, err = queryGroupedStats(db, `
		select coalesce(avg(cnt), 0), coalesce(max(cnt), 0)
		from (
			select ticket_id, count(*) as cnt
			from support_ticket_messages
			group by ticket_id
		) q
		where cnt > 0`); err != nil {
		return stats, err
	}

	if stats.RewardsPerUser, err = queryGroupedStats(db, `
		select coalesce(avg(cnt), 0), coalesce(max(cnt), 0)
		from (
			select user_id, count(*) as cnt
			from nutrition_reward_redemptions
			group by user_id
		) q
		where cnt > 0`); err != nil {
		return stats, err
	}

	if stats.LedgerEventsPerUser, err = queryGroupedStats(db, `
		select coalesce(avg(cnt), 0), coalesce(max(cnt), 0)
		from (
			select user_id, count(*) as cnt
			from nutrition_points_ledger
			group by user_id
		) q
		where cnt > 0`); err != nil {
		return stats, err
	}

	storage := []storageRow{
		{
			Section: "Пользователи и доступ",
			Tables:  "users, user_profiles, sessions, password_reset_requests",
		},
		{
			Section: "Питание и прогресс",
			Tables:  "nutrition_plan_meals, nutrition_day_progress, nutrition_hydration_logs, nutrition_reminder_settings, nutrition_questionnaire_responses, nutrition_user_stats, nutrition_day_event_history, nutrition_events",
		},
		{
			Section: "Поощрения и баллы",
			Tables:  "user_points, nutrition_points_ledger, nutrition_reward_redemptions, nutrition_reward_limits",
		},
		{
			Section: "Достижения",
			Tables:  "nutrition_achievement_rules, nutrition_achievement_catalog, nutrition_user_achievements",
		},
		{
			Section: "Поддержка и аудит",
			Tables:  "support_tickets, support_ticket_messages, nutrition_action_audit",
		},
	}

	for i := range storage {
		storage[i].Size, err = sumRelationSizes(db, splitTables(storage[i].Tables))
		if err != nil {
			return stats, err
		}
	}
	stats.Storage = storage

	return stats, nil
}

func renderReport(stats reportStats) string {
	planWeekMealsUpper := nutritionDaysPerWeek * nutritionMealSlotsPerDay
	planWeekHydrationUpper := nutritionDaysPerWeek * nutritionHydrationRemindersPerDay
	planWeekAvg := float64(nutritionDaysPerWeek) * (stats.MealsPerUserDay.Average + stats.HydrationPerUserDay.Average)
	leaderboardAvg := leaderboardEstimate(stats.Users, stats.MealAssignments, stats.DayProgressRows, stats.HydrationLogs)

	parameters := []parameterRow{
		{Symbol: "U", Value: fmt.Sprintf("%d", stats.Users), Description: "количество пользователей"},
		{Symbol: "T", Value: fmt.Sprintf("%d", stats.Sessions), Description: "количество активных сессий"},
		{Symbol: "M", Value: fmt.Sprintf("%d", stats.MealAssignments), Description: "количество назначений приемов пищи"},
		{Symbol: "D", Value: fmt.Sprintf("%d", stats.DayProgressRows), Description: "количество записей дневного прогресса"},
		{Symbol: "H", Value: fmt.Sprintf("%d", stats.HydrationLogs), Description: "количество логов гидратации"},
		{Symbol: "Rq", Value: fmt.Sprintf("%d", stats.RewardRequests), Description: "количество заявок на поощрения"},
		{Symbol: "Tk", Value: fmt.Sprintf("%d", stats.SupportTickets), Description: "количество обращений в поддержку"},
		{Symbol: "Msg", Value: fmt.Sprintf("%d", stats.SupportMessages), Description: "количество сообщений поддержки"},
		{Symbol: "L", Value: fmt.Sprintf("%d", stats.PointsLedgerEvents), Description: "количество операций в журнале баллов"},
		{Symbol: "A", Value: fmt.Sprintf("%d", stats.AuditEvents), Description: "количество записей аудита"},
		{Symbol: "MdAvg", Value: formatFloat(stats.MealsPerUserDay.Average), Description: "среднее число приемов пищи на пользователя в день"},
		{Symbol: "MdMax", Value: fmt.Sprintf("%d", stats.MealsPerUserDay.Max), Description: "максимальное число приемов пищи на пользователя в день"},
		{Symbol: "HdAvg", Value: formatFloat(stats.HydrationPerUserDay.Average), Description: "среднее число отметок гидратации на пользователя в день"},
		{Symbol: "HdMax", Value: fmt.Sprintf("%d", stats.HydrationPerUserDay.Max), Description: "максимальное число отметок гидратации на пользователя в день"},
		{Symbol: "MsgAvg", Value: formatFloat(stats.MessagesPerTicket.Average), Description: "среднее число сообщений в обращении"},
		{Symbol: "MsgMax", Value: fmt.Sprintf("%d", stats.MessagesPerTicket.Max), Description: "максимальное число сообщений в обращении"},
		{Symbol: "RuAvg", Value: formatFloat(stats.RewardsPerUser.Average), Description: "среднее число заявок на поощрение на пользователя"},
		{Symbol: "RuMax", Value: fmt.Sprintf("%d", stats.RewardsPerUser.Max), Description: "максимальное число заявок на поощрение на пользователя"},
		{Symbol: "LuAvg", Value: formatFloat(stats.LedgerEventsPerUser.Average), Description: "среднее число операций баллов на пользователя"},
		{Symbol: "LuMax", Value: fmt.Sprintf("%d", stats.LedgerEventsPerUser.Max), Description: "максимальное число операций баллов на пользователя"},
		{Symbol: "MwLimit", Value: fmt.Sprintf("%d", planWeekMealsUpper), Description: "теоретический предел приемов пищи в недельном плане (7×4)"},
		{Symbol: "HwLimit", Value: fmt.Sprintf("%d", planWeekHydrationUpper), Description: "теоретический предел гидратации в неделю (7×3)"},
	}

	storageRows := make([][]string, 0, len(stats.Storage)+1)
	totalStorage := int64(0)
	for _, row := range stats.Storage {
		totalStorage += row.Size
		storageRows = append(storageRows, []string{row.Section, row.Tables, humanBytes(row.Size)})
	}
	storageRows = append(storageRows, []string{"Итого", "все основные таблицы системы", humanBytes(totalStorage)})

	complexityRows := [][]string{
		{
			"Аутентификация пользователя",
			"O(log U)",
			"O(1)",
			fmt.Sprintf("U=%d", stats.Users),
			fmt.Sprintf("log2(U)=%s", formatFloat(log2Estimate(stats.Users))),
		},
		{
			"Проверка сессии",
			"O(log T)",
			"O(1)",
			fmt.Sprintf("T=%d", stats.Sessions),
			fmt.Sprintf("log2(T)=%s", formatFloat(log2Estimate(stats.Sessions))),
		},
		{
			"Загрузка недельного плана питания",
			"O(Mw+Hw)",
			"O(Mw+Hw)",
			fmt.Sprintf("MdAvg=%s, HdAvg=%s, MwLimit=%d, HwLimit=%d", formatFloat(stats.MealsPerUserDay.Average), formatFloat(stats.HydrationPerUserDay.Average), planWeekMealsUpper, planWeekHydrationUpper),
			fmt.Sprintf("avg≈%s, upper≈%d", formatFloat(planWeekAvg), planWeekMealsUpper+planWeekHydrationUpper),
		},
		{
			"Обновление прогресса дня",
			"O(Md)",
			"O(1)",
			fmt.Sprintf("MdAvg=%s, MdMax=%d", formatFloat(stats.MealsPerUserDay.Average), stats.MealsPerUserDay.Max),
			fmt.Sprintf("avg≈%s, upper≈%d", formatFloat(stats.MealsPerUserDay.Average), maxInt(stats.MealsPerUserDay.Max, nutritionMealSlotsPerDay)),
		},
		{
			"Загрузка треда поддержки",
			"O(MsgTicket)",
			"O(MsgTicket)",
			fmt.Sprintf("MsgAvg=%s, MsgMax=%d", formatFloat(stats.MessagesPerTicket.Average), stats.MessagesPerTicket.Max),
			fmt.Sprintf("avg≈%s, upper≈%d", formatFloat(stats.MessagesPerTicket.Average), stats.MessagesPerTicket.Max),
		},
		{
			"Формирование лидерборда",
			"O(U log U + M + D + H)",
			"O(U)",
			fmt.Sprintf("U=%d, M=%d, D=%d, H=%d", stats.Users, stats.MealAssignments, stats.DayProgressRows, stats.HydrationLogs),
			fmt.Sprintf("avg≈%s", formatFloat(leaderboardAvg)),
		},
		{
			"Проверка лимита поощрения",
			"O(Ru)",
			"O(1)",
			fmt.Sprintf("RuAvg=%s, RuMax=%d", formatFloat(stats.RewardsPerUser.Average), stats.RewardsPerUser.Max),
			fmt.Sprintf("avg≈%s, upper≈%d", formatFloat(stats.RewardsPerUser.Average), stats.RewardsPerUser.Max),
		},
	}

	sort.Slice(parameters, func(i, j int) bool {
		return parameters[i].Symbol < parameters[j].Symbol
	})

	var b strings.Builder
	b.WriteString("Расчет вычислительной и ёмкостной сложности серверной части\n\n")
	b.WriteString("Листинг X.X – Расчет параметров системы\n")
	paramRows := make([][]string, 0, len(parameters))
	for _, row := range parameters {
		paramRows = append(paramRows, []string{row.Symbol, row.Value, row.Description})
	}
	b.WriteString(renderTable([]string{"Обозначение", "Значение", "Описание"}, paramRows))
	b.WriteString("\n")

	b.WriteString("Листинг X.X – Емкостная сложность по основным подсистемам\n")
	b.WriteString(renderTable([]string{"Подсистема", "Таблицы", "Объем"}, storageRows))
	b.WriteString("\n")

	b.WriteString("Листинг X.X – Вычислительная сложность основных операций\n")
	b.WriteString(renderTable([]string{"Операция", "Время", "Память", "Параметры", "Текущая оценка"}, complexityRows))
	b.WriteString("\n")
	b.WriteString("Примечание: MwLimit=7×4=28 (приемы пищи в неделю), HwLimit=7×3=21 (напоминания гидратации в неделю).\n")

	return b.String()
}

func queryInt(db *sql.DB, query string) (int, error) {
	var value int
	if err := db.QueryRow(query).Scan(&value); err != nil {
		return 0, err
	}
	return value, nil
}

func queryGroupedStats(db *sql.DB, query string) (numericStats, error) {
	var avg sql.NullFloat64
	var max sql.NullInt64
	if err := db.QueryRow(query).Scan(&avg, &max); err != nil {
		return numericStats{}, err
	}
	stats := numericStats{}
	if avg.Valid {
		stats.Average = avg.Float64
	}
	if max.Valid {
		stats.Max = int(max.Int64)
	}
	return stats, nil
}

func sumRelationSizes(db *sql.DB, tables []string) (int64, error) {
	var total int64
	for _, table := range tables {
		var size sql.NullInt64
		if err := db.QueryRow(`select coalesce(pg_total_relation_size(to_regclass($1)), 0)`, table).Scan(&size); err != nil {
			return 0, err
		}
		if size.Valid {
			total += size.Int64
		}
	}
	return total, nil
}

func splitTables(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func renderTable(headers []string, rows [][]string) string {
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = runeLen(header)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && runeLen(cell) > widths[i] {
				widths[i] = runeLen(cell)
			}
		}
	}

	var b strings.Builder
	b.WriteString(tableBorder(widths))
	b.WriteString(tableRow(headers, widths))
	b.WriteString(tableBorder(widths))
	for _, row := range rows {
		b.WriteString(tableRow(row, widths))
	}
	b.WriteString(tableBorder(widths))
	return b.String()
}

func tableBorder(widths []int) string {
	parts := make([]string, 0, len(widths))
	for _, width := range widths {
		parts = append(parts, strings.Repeat("-", width+2))
	}
	return "+" + strings.Join(parts, "+") + "+\n"
}

func tableRow(cells []string, widths []int) string {
	parts := make([]string, len(widths))
	for i := range widths {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		parts[i] = " " + padRight(cell, widths[i]) + " "
	}
	return "|" + strings.Join(parts, "|") + "|\n"
}

func padRight(value string, width int) string {
	padding := width - runeLen(value)
	if padding <= 0 {
		return value
	}
	return value + strings.Repeat(" ", padding)
}

func runeLen(value string) int {
	return utf8.RuneCountInString(value)
}

func humanBytes(size int64) string {
	units := []string{"B", "KB", "MB", "GB"}
	value := float64(size)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d %s", size, units[unit])
	}
	return fmt.Sprintf("%.2f %s", value, units[unit])
}

func formatFloat(value float64) string {
	return fmt.Sprintf("%.2f", value)
}

func log2Estimate(value int) float64 {
	if value <= 1 {
		return 0
	}
	return math.Log2(float64(value))
}

func leaderboardEstimate(users, meals, dayProgress, hydration int) float64 {
	return float64(meals+dayProgress+hydration) + float64(users)*log2Estimate(users)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
