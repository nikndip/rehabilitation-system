package site

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

var errNutritionInsufficientPoints = errors.New("nutrition: insufficient points")

type nutritionReminderSettings struct {
	MealReminderLeadMinutes int
	MealSLAMinutes          int
	Hydration1030Enabled    bool
	Hydration1500Enabled    bool
	Hydration1800Enabled    bool
}

type nutritionAchievementRecord struct {
	ID           string
	Code         string
	Title        string
	Description  string
	Icon         string
	PointsReward int
	MetricKey    string
	WindowDays   int
	TargetValue  int
}

type nutritionPointsLedgerView struct {
	UserID      string
	EmployeeID  string
	UserName    string
	Change      int
	Balance     int
	ReasonCode  string
	Reason      string
	SourceType  string
	CreatedAt   string
	CreatedBy   string
	CreatedByID string
}

func defaultNutritionReminderSettings() nutritionReminderSettings {
	return nutritionReminderSettings{
		MealReminderLeadMinutes: 20,
		MealSLAMinutes:          nutritionReminderSLAMinutes,
		Hydration1030Enabled:    true,
		Hydration1500Enabled:    true,
		Hydration1800Enabled:    true,
	}
}

func clampNutritionReminderSettings(settings nutritionReminderSettings) nutritionReminderSettings {
	defaults := defaultNutritionReminderSettings()
	if settings.MealReminderLeadMinutes < 0 || settings.MealReminderLeadMinutes > 240 {
		settings.MealReminderLeadMinutes = defaults.MealReminderLeadMinutes
	}
	if settings.MealSLAMinutes < 15 || settings.MealSLAMinutes > 360 {
		settings.MealSLAMinutes = defaults.MealSLAMinutes
	}
	return settings
}

func (s *Site) loadNutritionReminderSettings(userID string) nutritionReminderSettings {
	settings := defaultNutritionReminderSettings()
	if strings.TrimSpace(userID) == "" {
		return settings
	}

	err := s.DB.QueryRow(
		`select meal_reminder_lead_minutes,
		        meal_sla_minutes,
		        hydration_1030_enabled,
		        hydration_1500_enabled,
		        hydration_1800_enabled
		 from nutrition_reminder_settings
		 where user_id = $1`,
		userID,
	).Scan(
		&settings.MealReminderLeadMinutes,
		&settings.MealSLAMinutes,
		&settings.Hydration1030Enabled,
		&settings.Hydration1500Enabled,
		&settings.Hydration1800Enabled,
	)
	if err != nil {
		return defaultNutritionReminderSettings()
	}
	return clampNutritionReminderSettings(settings)
}

func (s *Site) saveNutritionReminderSettings(userID string, settings nutritionReminderSettings) error {
	if strings.TrimSpace(userID) == "" {
		return errors.New("nutrition: empty user id")
	}
	settings = clampNutritionReminderSettings(settings)
	_, err := s.DB.Exec(
		`insert into nutrition_reminder_settings (
			user_id,
			meal_reminder_lead_minutes,
			meal_sla_minutes,
			hydration_1030_enabled,
			hydration_1500_enabled,
			hydration_1800_enabled,
			updated_at
		 )
		 values ($1, $2, $3, $4, $5, $6, now())
		 on conflict (user_id)
		 do update set meal_reminder_lead_minutes = excluded.meal_reminder_lead_minutes,
		               meal_sla_minutes = excluded.meal_sla_minutes,
		               hydration_1030_enabled = excluded.hydration_1030_enabled,
		               hydration_1500_enabled = excluded.hydration_1500_enabled,
		               hydration_1800_enabled = excluded.hydration_1800_enabled,
		               updated_at = now()`,
		userID,
		settings.MealReminderLeadMinutes,
		settings.MealSLAMinutes,
		settings.Hydration1030Enabled,
		settings.Hydration1500Enabled,
		settings.Hydration1800Enabled,
	)
	return err
}

func nutritionHydrationReminderOptionsForSettings(settings nutritionReminderSettings) []nutritionHydrationReminderOption {
	all := nutritionHydrationReminderOptions()
	filtered := make([]nutritionHydrationReminderOption, 0, len(all))
	for _, option := range all {
		switch option.Key {
		case "1030":
			if settings.Hydration1030Enabled {
				filtered = append(filtered, option)
			}
		case "1500":
			if settings.Hydration1500Enabled {
				filtered = append(filtered, option)
			}
		case "1800":
			if settings.Hydration1800Enabled {
				filtered = append(filtered, option)
			}
		default:
			filtered = append(filtered, option)
		}
	}
	return filtered
}

func (s *Site) loadNutritionLeaderboard(limit int) []nutritionLeaderboardRow {
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.DB.Query(
		`select u.id,
		        u.name,
		        coalesce(u.department, ''),
		        coalesce(up.points_balance, 0),
		        coalesce(nd.days_completed, 0),
		        coalesce(nm.completed_slots, 0),
		        coalesce(nm.total_slots, 0),
		        coalesce(nh.hydration_days, 0),
		        coalesce(ne.last_event_at, nd.last_day)
		 from users u
		 left join user_points up on up.user_id = u.id
		 left join (
		   select user_id,
		          count(*) filter (where day_completed = true) as days_completed,
		          max(day_date)::timestamptz as last_day
		   from nutrition_day_progress
		   group by user_id
		 ) nd on nd.user_id = u.id
		 left join (
		   select user_id,
		          count(*) as total_slots,
		          count(*) filter (where status = 'completed') as completed_slots
		   from nutrition_plan_meals
		   group by user_id
		 ) nm on nm.user_id = u.id
		 left join (
		   select user_id, count(distinct day_date) as hydration_days
		   from nutrition_hydration_logs
		   where status = 'completed'
		   group by user_id
		 ) nh on nh.user_id = u.id
		 left join (
		   select user_id, max(created_at) as last_event_at
		   from nutrition_events
		   group by user_id
		 ) ne on ne.user_id = u.id
		 where u.role = 'employee'
		 order by coalesce(up.points_balance, 0) desc,
		          coalesce(nd.days_completed, 0) desc,
		          coalesce(nm.completed_slots, 0) desc,
		          u.name
		 limit $1`,
		limit,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	now := time.Now()
	list := []nutritionLeaderboardRow{}
	for rows.Next() {
		var userID string
		var row nutritionLeaderboardRow
		var completedSlots int
		var totalSlots int
		var hydrationDays int
		var lastCheckin sql.NullTime
		if err := rows.Scan(
			&userID,
			&row.Name,
			&row.Department,
			&row.Points,
			&row.Days,
			&completedSlots,
			&totalSlots,
			&hydrationDays,
			&lastCheckin,
		); err != nil {
			continue
		}

		if totalSlots > 0 {
			row.Compliance = int(float64(completedSlots) / float64(totalSlots) * 100)
		}
		hydrationBase := row.Days
		if hydrationBase <= 0 {
			hydrationBase = 1
		}
		row.Hydration = int(float64(hydrationDays) / float64(hydrationBase) * 100)
		if row.Hydration > 100 {
			row.Hydration = 100
		}
		if lastCheckin.Valid {
			row.LastCheckin = nutritionRelativeDateLabel(lastCheckin.Time, now)
		} else {
			row.LastCheckin = "Нет данных"
		}
		list = append(list, row)
	}
	return list
}

func nutritionRelativeDateLabel(value, now time.Time) string {
	valueDate := nutritionDateOnly(value)
	nowDate := nutritionDateOnly(now)
	diff := int(nowDate.Sub(valueDate).Hours() / 24)
	switch {
	case diff <= 0:
		return "Сегодня"
	case diff == 1:
		return "Вчера"
	case diff < 5:
		return fmt.Sprintf("%d дня назад", diff)
	case diff < 21:
		return fmt.Sprintf("%d дней назад", diff)
	default:
		return valueDate.Format("02.01.2006")
	}
}

func (s *Site) loadNutritionTrendForUser(userID string, now time.Time) []nutritionTrendPoint {
	labels := []string{"Вс", "Пн", "Вт", "Ср", "Чт", "Пт", "Сб"}
	start := nutritionDateOnly(now).AddDate(0, 0, -6)
	end := nutritionDateOnly(now).AddDate(0, 0, 1)

	dayProgress := map[string]struct {
		completed int
		total     int
	}{}
	rows, err := s.DB.Query(
		`select day_date,
		        coalesce(completed_slots, 0),
		        coalesce(total_slots, 0)
		 from nutrition_day_progress
		 where user_id = $1 and day_date >= $2 and day_date < $3`,
		userID,
		start,
		end,
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var dayDate time.Time
			var completed int
			var total int
			if err := rows.Scan(&dayDate, &completed, &total); err != nil {
				continue
			}
			key := nutritionDateOnly(dayDate).Format("2006-01-02")
			dayProgress[key] = struct {
				completed int
				total     int
			}{
				completed: completed,
				total:     total,
			}
		}
	}

	hydrationDone := map[string]int{}
	rows, err = s.DB.Query(
		`select day_date, count(*)
		 from nutrition_hydration_logs
		 where user_id = $1
		   and status = 'completed'
		   and day_date >= $2
		   and day_date < $3
		 group by day_date`,
		userID,
		start,
		end,
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var dayDate time.Time
			var count int
			if err := rows.Scan(&dayDate, &count); err != nil {
				continue
			}
			key := nutritionDateOnly(dayDate).Format("2006-01-02")
			hydrationDone[key] = count
		}
	}

	settings := s.loadNutritionReminderSettings(userID)
	hydrationTarget := len(nutritionHydrationReminderOptionsForSettings(settings))
	if hydrationTarget <= 0 {
		hydrationTarget = len(nutritionHydrationReminderOptions())
	}
	if hydrationTarget <= 0 {
		hydrationTarget = 3
	}

	trend := make([]nutritionTrendPoint, 0, 7)
	for offset := 0; offset < 7; offset++ {
		dayDate := start.AddDate(0, 0, offset)
		key := dayDate.Format("2006-01-02")
		point := nutritionTrendPoint{
			Label: labels[int(dayDate.Weekday())],
		}

		if stats, ok := dayProgress[key]; ok && stats.total > 0 {
			point.Compliance = int(float64(stats.completed) / float64(stats.total) * 100)
		}
		point.CompliancePercent = point.Compliance

		hydrationCount := hydrationDone[key]
		point.Hydration = int(float64(hydrationCount) / float64(hydrationTarget) * 100)
		if point.Hydration > 100 {
			point.Hydration = 100
		}
		point.HydrationPercent = point.Hydration
		trend = append(trend, point)
	}

	return trend
}

func (s *Site) loadNutritionAchievementsView(userID string) []nutritionAchievementView {
	views, err := s.refreshNutritionAchievements(userID)
	if err != nil {
		return nil
	}
	return views
}

func (s *Site) refreshNutritionAchievements(userID string) ([]nutritionAchievementView, error) {
	records, err := s.loadNutritionAchievementCatalog()
	if err != nil {
		return nil, err
	}
	views := make([]nutritionAchievementView, 0, len(records))
	for _, item := range records {
		metricValue := s.resolveNutritionAchievementMetric(userID, item.MetricKey, item.WindowDays)
		progress := metricValue
		if progress > item.TargetValue {
			progress = item.TargetValue
		}
		unlocked := metricValue >= item.TargetValue

		var unlockedAt sql.NullTime
		_ = s.DB.QueryRow(
			`select unlocked_at
			 from nutrition_user_achievements
			 where user_id = $1 and achievement_id = $2`,
			userID,
			item.ID,
		).Scan(&unlockedAt)

		rewardAlreadyGranted := s.nutritionAchievementRewardAlreadyGranted(userID, item.ID)
		if unlocked && item.PointsReward > 0 && !rewardAlreadyGranted {
			awardedPoints, pointsErr := s.applyNutritionPointsChangeWithDailyCap(
				userID,
				time.Now(),
				item.PointsReward,
				"achievement_unlock",
				"Открыто достижение: "+item.Title,
				"nutrition_achievement",
				item.ID,
				"",
			)
			if pointsErr == nil && awardedPoints > 0 {
				s.insertNutritionEvent(userID, "Достижение «"+item.Title+"» открыто: +"+fmt.Sprintf("%d", awardedPoints)+" баллов.")
			} else if pointsErr == nil {
				log.Printf(
					"nutrition: achievement points capped by daily limit user=%s achievement=%s (%s)",
					userID,
					item.ID,
					item.Code,
				)
			} else {
				log.Printf(
					"nutrition: award points for achievement failed user=%s achievement=%s (%s): %v",
					userID,
					item.ID,
					item.Code,
					pointsErr,
				)
			}
		}

		_, _ = s.DB.Exec(
			`insert into nutrition_user_achievements (
				user_id, achievement_id, progress, target, unlocked, unlocked_at, last_progress_at, updated_at
			 )
			 values (
			 	$1, $2, $3, $4, $5,
			 	case when $5 then coalesce($6, now()) else null end,
			 	now(),
			 	now()
			 )
			 on conflict (user_id, achievement_id)
			 do update set progress = excluded.progress,
			               target = excluded.target,
			               unlocked = excluded.unlocked,
			               unlocked_at = case
			                 when nutrition_user_achievements.unlocked then nutrition_user_achievements.unlocked_at
			                 when excluded.unlocked then coalesce(nutrition_user_achievements.unlocked_at, now())
			                 else null
			               end,
			               last_progress_at = excluded.last_progress_at,
			               updated_at = now()`,
			userID,
			item.ID,
			progress,
			item.TargetValue,
			unlocked,
			unlockedAt,
		)

		views = append(views, nutritionAchievementView{
			Title:        item.Title,
			Description:  item.Description,
			Icon:         item.Icon,
			Unlocked:     unlocked,
			Progress:     progress,
			Total:        item.TargetValue,
			PointsReward: item.PointsReward,
		})
	}
	return views, nil
}

func (s *Site) nutritionAchievementRewardAlreadyGranted(userID, achievementID string) bool {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(achievementID) == "" {
		return false
	}
	var exists bool
	_ = s.DB.QueryRow(
		`select exists(
		   select 1
		   from nutrition_points_ledger
		   where user_id = $1
		     and source_type = 'nutrition_achievement'
		     and source_id = $2
		     and reason_code = 'achievement_unlock'
		     and change_amount > 0
		 )`,
		userID,
		achievementID,
	).Scan(&exists)
	return exists
}

func (s *Site) loadNutritionAchievementCatalog() ([]nutritionAchievementRecord, error) {
	rows, err := s.DB.Query(
		`select c.id,
		        c.code,
		        c.title,
		        c.description,
		        c.icon,
		        c.points_reward,
		        r.metric_key,
		        r.window_days,
		        r.target_value
		 from nutrition_achievement_catalog c
		 join nutrition_achievement_rules r on r.id = c.rule_id
		 where c.active = true
		 order by c.sort_order, c.title`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := []nutritionAchievementRecord{}
	for rows.Next() {
		var row nutritionAchievementRecord
		if err := rows.Scan(
			&row.ID,
			&row.Code,
			&row.Title,
			&row.Description,
			&row.Icon,
			&row.PointsReward,
			&row.MetricKey,
			&row.WindowDays,
			&row.TargetValue,
		); err != nil {
			continue
		}
		records = append(records, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func (s *Site) resolveNutritionAchievementMetric(userID, metricKey string, windowDays int) int {
	windowDays = max(windowDays, 0)
	var fromDate any
	if windowDays > 0 {
		fromDate = nutritionDateOnly(time.Now()).AddDate(0, 0, -(windowDays - 1))
	}

	switch strings.ToLower(strings.TrimSpace(metricKey)) {
	case "best_streak":
		_, best := s.loadNutritionStreak(userID)
		return best
	case "current_streak":
		current, _ := s.loadNutritionStreak(userID)
		return current
	case "hydration_days_total":
		var count int
		if fromDate == nil {
			_ = s.DB.QueryRow(
				`select count(*)
				 from (
				   select day_date
				   from nutrition_hydration_logs
				   where user_id = $1 and status = 'completed'
				   group by day_date
				 ) t`,
				userID,
			).Scan(&count)
		} else {
			_ = s.DB.QueryRow(
				`select count(*)
				 from (
				   select day_date
				   from nutrition_hydration_logs
				   where user_id = $1 and status = 'completed' and day_date >= $2
				   group by day_date
				 ) t`,
				userID,
				fromDate,
			).Scan(&count)
		}
		return count
	case "completed_days_total":
		fallthrough
	default:
		var count int
		if fromDate == nil {
			_ = s.DB.QueryRow(
				`select count(*)
				 from nutrition_day_progress
				 where user_id = $1 and day_completed = true`,
				userID,
			).Scan(&count)
		} else {
			_ = s.DB.QueryRow(
				`select count(*)
				 from nutrition_day_progress
				 where user_id = $1 and day_completed = true and day_date >= $2`,
				userID,
				fromDate,
			).Scan(&count)
		}
		return count
	}
}

func (s *Site) applyNutritionPointsChangeWithDailyCap(
	userID string,
	dayDate time.Time,
	changeAmount int,
	reasonCode string,
	reason string,
	sourceType string,
	sourceID string,
	createdBy string,
) (int, error) {
	if changeAmount <= 0 {
		_, err := s.applyNutritionPointsChange(userID, changeAmount, reasonCode, reason, sourceType, sourceID, createdBy)
		if err != nil {
			return 0, err
		}
		return changeAmount, nil
	}

	if dayDate.IsZero() {
		dayDate = time.Now()
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	remaining, err := s.nutritionDailyPointsRemainingTx(tx, userID, dayDate)
	if err != nil {
		return 0, err
	}
	if remaining <= 0 {
		return 0, nil
	}

	award := min(changeAmount, remaining)
	if award <= 0 {
		return 0, nil
	}

	_, err = s.applyNutritionPointsChangeTx(tx, userID, award, reasonCode, reason, sourceType, sourceID, createdBy)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return award, nil
}

func (s *Site) nutritionDailyPointsRemainingTx(tx *sql.Tx, userID string, dayDate time.Time) (int, error) {
	if tx == nil {
		return 0, errors.New("nutrition: nil transaction for daily cap")
	}
	dayStart := nutritionDateOnly(dayDate)
	dayEnd := dayStart.AddDate(0, 0, 1)

	var earned int
	if err := tx.QueryRow(
		`select coalesce(sum(change_amount), 0)
		 from nutrition_points_ledger
		 where user_id = $1
		   and change_amount > 0
		   and created_at >= $2
		   and created_at < $3`,
		userID,
		dayStart,
		dayEnd,
	).Scan(&earned); err != nil {
		return 0, err
	}

	remaining := nutritionDailyPointsCap - earned
	if remaining < 0 {
		remaining = 0
	}
	return remaining, nil
}

func (s *Site) applyNutritionPointsChange(
	userID string,
	changeAmount int,
	reasonCode string,
	reason string,
	sourceType string,
	sourceID string,
	createdBy string,
) (int, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	balance, err := s.applyNutritionPointsChangeTx(tx, userID, changeAmount, reasonCode, reason, sourceType, sourceID, createdBy)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return balance, nil
}

func (s *Site) applyNutritionPointsChangeTx(
	tx *sql.Tx,
	userID string,
	changeAmount int,
	reasonCode string,
	reason string,
	sourceType string,
	sourceID string,
	createdBy string,
) (int, error) {
	if tx == nil {
		return 0, errors.New("nutrition: nil transaction")
	}
	if strings.TrimSpace(userID) == "" {
		return 0, errors.New("nutrition: empty user id")
	}
	if changeAmount == 0 {
		var balance int
		_ = tx.QueryRow(`select coalesce(points_balance, 0) from user_points where user_id = $1`, userID).Scan(&balance)
		return balance, nil
	}

	if strings.TrimSpace(reasonCode) == "" {
		reasonCode = "manual"
	}
	if strings.TrimSpace(sourceType) == "" {
		sourceType = "system"
	}

	_, _ = tx.Exec(
		`insert into user_points (user_id, points_balance, points_total, updated_at)
		 values ($1, 0, 0, now())
		 on conflict (user_id) do nothing`,
		userID,
	)

	var balance int
	err := tx.QueryRow(
		`update user_points
		 set points_balance = points_balance + $1,
		     points_total = points_total + case when $1 > 0 then $1 else 0 end,
		     updated_at = now()
		 where user_id = $2 and points_balance + $1 >= 0
		 returning points_balance`,
		changeAmount,
		userID,
	).Scan(&balance)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, errNutritionInsufficientPoints
		}
		return 0, err
	}

	_, err = tx.Exec(
		`insert into nutrition_points_ledger (
			user_id,
			change_amount,
			balance_after,
			reason_code,
			reason,
			source_type,
			source_id,
			created_by
		 )
		 values ($1, $2, $3, $4, $5, $6, $7, $8)`,
		userID,
		changeAmount,
		balance,
		strings.TrimSpace(reasonCode),
		strings.TrimSpace(reason),
		strings.TrimSpace(sourceType),
		nullIfEmpty(strings.TrimSpace(sourceID)),
		nullIfEmpty(strings.TrimSpace(createdBy)),
	)
	if err != nil {
		return 0, err
	}
	return balance, nil
}

func (s *Site) insertNutritionDayEvent(userID, dayKey, eventType, slotKey string, dayDate time.Time, metadata map[string]any) {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(eventType) == "" || dayDate.IsZero() {
		return
	}
	payload := map[string]any{}
	for key, value := range metadata {
		payload[key] = value
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		raw = []byte("{}")
	}
	_, _ = s.DB.Exec(
		`insert into nutrition_day_event_history (
			user_id, day_date, day_key, event_type, slot_key, metadata
		 )
		 values ($1, $2, $3, $4, $5, $6)`,
		userID,
		nutritionDateOnly(dayDate),
		normalizeNutritionDayKey(dayKey),
		strings.TrimSpace(eventType),
		nullIfEmpty(normalizeNutritionSlotKey(slotKey)),
		raw,
	)
}

func (s *Site) loadNutritionPointsLedger(limit int) []nutritionPointsLedgerView {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.DB.Query(
		`select l.user_id,
		        coalesce(u.employee_id, ''),
		        coalesce(u.name, ''),
		        l.change_amount,
		        coalesce(l.balance_after, 0),
		        coalesce(l.reason_code, ''),
		        coalesce(l.reason, ''),
		        coalesce(l.source_type, ''),
		        l.created_at,
		        coalesce(cu.name, ''),
		        coalesce(cu.id::text, '')
		 from nutrition_points_ledger l
		 join users u on u.id = l.user_id
		 left join users cu on cu.id = l.created_by
		 order by l.created_at desc
		 limit $1`,
		limit,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	result := []nutritionPointsLedgerView{}
	for rows.Next() {
		var row nutritionPointsLedgerView
		var createdAt time.Time
		if err := rows.Scan(
			&row.UserID,
			&row.EmployeeID,
			&row.UserName,
			&row.Change,
			&row.Balance,
			&row.ReasonCode,
			&row.Reason,
			&row.SourceType,
			&createdAt,
			&row.CreatedBy,
			&row.CreatedByID,
		); err != nil {
			continue
		}
		row.CreatedAt = createdAt.Format("02.01.2006 15:04")
		result = append(result, row)
	}
	return result
}

func (s *Site) ensureNutritionAchievementProgressForEmployees() {
	rows, err := s.DB.Query(`select id from users where role = 'employee'`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			continue
		}
		_, _ = s.refreshNutritionAchievements(userID)
	}
}

func (s *Site) loadNutritionAchievementProgressRows(limit int) []adminNutritionAchievementProgressRow {
	if limit <= 0 {
		limit = 250
	}
	rows, err := s.DB.Query(
		`select u.id,
		        u.name,
		        coalesce(u.employee_id, ''),
		        c.title,
		        coalesce(ua.progress, 0),
		        coalesce(ua.target, 0),
		        coalesce(ua.unlocked, false),
		        ua.unlocked_at,
		        ua.updated_at
		 from nutrition_user_achievements ua
		 join users u on u.id = ua.user_id
		 join nutrition_achievement_catalog c on c.id = ua.achievement_id
		 where u.role = 'employee'
		 order by ua.updated_at desc
		 limit $1`,
		limit,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	items := []adminNutritionAchievementProgressRow{}
	for rows.Next() {
		var row adminNutritionAchievementProgressRow
		var unlockedAt sql.NullTime
		var updatedAt time.Time
		if err := rows.Scan(
			&row.UserID,
			&row.UserName,
			&row.EmployeeID,
			&row.AchievementTitle,
			&row.Progress,
			&row.Target,
			&row.Unlocked,
			&unlockedAt,
			&updatedAt,
		); err != nil {
			continue
		}
		row.UpdatedAt = updatedAt.Format("02.01.2006 15:04")
		if unlockedAt.Valid {
			row.UnlockedAt = unlockedAt.Time.Format("02.01.2006 15:04")
		}
		items = append(items, row)
	}
	return items
}

func (s *Site) loadNutritionAdminAchievementCatalog() []adminNutritionAchievementCatalogRow {
	rows, err := s.DB.Query(
		`select c.id,
		        c.code,
		        c.title,
		        c.description,
		        c.icon,
		        c.points_reward,
		        c.active,
		        c.sort_order,
		        r.id,
		        r.rule_code,
		        r.metric_key,
		        r.window_days,
		        r.target_value,
		        r.description
		 from nutrition_achievement_catalog c
		 join nutrition_achievement_rules r on r.id = c.rule_id
		 order by c.sort_order, c.title`,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	items := []adminNutritionAchievementCatalogRow{}
	for rows.Next() {
		var row adminNutritionAchievementCatalogRow
		if err := rows.Scan(
			&row.ID,
			&row.Code,
			&row.Title,
			&row.Description,
			&row.Icon,
			&row.PointsReward,
			&row.Active,
			&row.SortOrder,
			&row.RuleID,
			&row.RuleCode,
			&row.MetricKey,
			&row.WindowDays,
			&row.TargetValue,
			&row.RuleDescription,
		); err != nil {
			continue
		}
		items = append(items, row)
	}
	return items
}

func (s *Site) upsertNutritionAchievement(rule adminNutritionAchievementCatalogRow) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var ruleID string
	err = tx.QueryRow(
		`insert into nutrition_achievement_rules (rule_code, metric_key, window_days, target_value, description, updated_at)
		 values ($1, $2, $3, $4, $5, now())
		 on conflict (rule_code)
		 do update set metric_key = excluded.metric_key,
		               window_days = excluded.window_days,
		               target_value = excluded.target_value,
		               description = excluded.description,
		               updated_at = now()
		 returning id`,
		rule.RuleCode,
		rule.MetricKey,
		max(rule.WindowDays, 0),
		max(rule.TargetValue, 1),
		rule.RuleDescription,
	).Scan(&ruleID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(
		`insert into nutrition_achievement_catalog (
			code, title, description, icon, points_reward, rule_id, active, sort_order, updated_at
		 )
		 values ($1, $2, $3, $4, $5, $6, $7, $8, now())
		 on conflict (code)
		 do update set title = excluded.title,
		               description = excluded.description,
		               icon = excluded.icon,
		               points_reward = excluded.points_reward,
		               rule_id = excluded.rule_id,
		               active = excluded.active,
		               sort_order = excluded.sort_order,
		               updated_at = now()`,
		rule.Code,
		rule.Title,
		rule.Description,
		rule.Icon,
		max(rule.PointsReward, 0),
		ruleID,
		rule.Active,
		max(rule.SortOrder, 1),
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}
