package site

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"rehab-app/internal/db"
	"rehab-app/internal/middleware"
)

const (
	nutritionSlotsPerDay        = 4
	nutritionDayCompletionPts   = 35
	nutritionReminderSLAMinutes = 60
)

type nutritionDashboardStats struct {
	DaysOnPlan      int
	HydrationDays   int
	Points          int
	ComplianceScore int
	CurrentStreak   int
	BestStreak      int
}

type nutritionMealSchedule struct {
	Name        string
	Description string
	Time        string
	Calories    int
	Protein     int
	Carbs       int
	Fats        int
}

type nutritionChecklistItem struct {
	Title     string
	Completed bool
}

type nutritionChallengeItem struct {
	Title     string
	Points    int
	Completed bool
}

type nutritionTrendPoint struct {
	Label             string
	Compliance        int
	CompliancePercent int
	Hydration         int
	HydrationPercent  int
}

type nutritionPlanDay struct {
	DayKey             string
	DayLabel           string
	DayDate            time.Time
	DateLabel          string
	Status             string
	Focus              string
	Hydration          string
	Slots              []nutritionMealSlotView
	HydrationReminders []nutritionHydrationReminderView
	CompletedSlots     int
}

type nutritionMealSlotView struct {
	DayKey          string
	SlotKey         string
	SlotLabel       string
	PlannedTime     string
	MealID          string
	MealName        string
	Calories        int
	Protein         int
	Carbs           int
	Fats            int
	Status          string
	CompletedAt     string
	CompletedOnTime bool
	ReminderStatus  string
	ReminderHint    string
	SuggestedMeal   *nutritionMealCard
	SuggestedReason string
}

type nutritionDayOption struct {
	Key   string
	Label string
}

type nutritionMealCard struct {
	ID          string
	Name        string
	Description string
	Category    string
	Calories    int
	Protein     int
	Carbs       int
	Fats        int
}

type nutritionLeaderboardRow struct {
	Name        string
	Department  string
	Points      int
	Days        int
	Compliance  int
	Hydration   int
	LastCheckin string
}

type nutritionReward struct {
	ID          string
	Title       string
	Description string
	PointsCost  int
	Category    string
}

type nutritionAchievementView struct {
	Title        string
	Description  string
	Icon         string
	Unlocked     bool
	Progress     int
	Total        int
	PointsReward int
}

type nutritionProfileView struct {
	EmployeeID      string
	Department      string
	Position        string
	NutritionTarget string
	DailyCalories   int
	WaterTarget     string
	MealPattern     string
	Restrictions    []string
}

type nutritionSupportContact struct {
	Title       string
	Description string
	ActionLabel string
	ActionValue string
}

type nutritionFAQItem struct {
	Question string
	Answer   string
}

type nutritionReminderItem struct {
	Title string
	Time  string
	State string
	Hint  string
}

type nutritionWeeklyReview struct {
	Strengths    []string
	Improvements []string
}

type nutritionEventView struct {
	Message   string
	CreatedAt string
}

type nutritionRewardHistoryView struct {
	ID         string
	Title      string
	PointsCost int
	Status     string
	RedeemedAt string
	UsedAt     string
	CanUse     bool
}

type nutritionAssignmentRecord struct {
	Meal                nutritionMealCard
	Status              string
	PlannedTime         string
	CompletedAt         time.Time
	SmartSwapFromMealID string
}

func (s *Site) moduleSelectorPage(w http.ResponseWriter, r *http.Request) {
	data := s.baseData(r, "Выбор модуля", "")
	data["HideNav"] = true
	s.render(w, "module_selector", data)
}

func (s *Site) nutritionDashboardPage(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	now := time.Now()
	planDays := s.buildNutritionPlan(user.ID, now)

	stats := nutritionDashboardStats{
		DaysOnPlan:      s.loadNutritionCompletedDays(user.ID),
		HydrationDays:   s.loadNutritionHydrationDaysEstimate(user.ID),
		Points:          s.loadUserPoints(user.ID),
		ComplianceScore: nutritionCompletionPercent(planDays),
	}
	stats.CurrentStreak, stats.BestStreak = s.loadNutritionStreak(user.ID)
	streakProgressPercent := 0
	if stats.BestStreak > 0 {
		streakProgressPercent = int(float64(stats.CurrentStreak) / float64(stats.BestStreak) * 100)
		if streakProgressPercent > 100 {
			streakProgressPercent = 100
		}
	}

	nextMeal := nutritionNextMeal(planDays, now)
	if nextMeal == nil {
		nextMeal = &nutritionMealSchedule{
			Name:        "План сформирован",
			Description: "Все приемы на текущий период закрыты. Добавьте блюда на следующую неделю.",
			Time:        "—",
		}
	}

	review := nutritionBuildWeeklyReview(planDays)
	reminders := nutritionBuildReminderItems(planDays, now)

	data := s.nutritionBaseData(r, "Питание", "nutrition-dashboard")
	data["Stats"] = stats
	data["NextMeal"] = nextMeal
	data["Checklist"] = []nutritionChecklistItem{
		{Title: "Завтрак до 09:00", Completed: nextMeal.Time != "08:30"},
		{Title: "Вода 1.5+ литра", Completed: stats.HydrationDays >= 4},
		{Title: "Овощи в 2 приемах пищи", Completed: stats.ComplianceScore >= 70},
		{Title: "Легкий ужин до 20:00", Completed: stats.ComplianceScore >= 85},
	}
	data["ChallengeItems"] = []nutritionChallengeItem{
		{Title: "3 дня подряд без пропуска приема", Points: 20, Completed: stats.CurrentStreak >= 3},
		{Title: "Закрыть 4 приема за день", Points: nutritionDayCompletionPts, Completed: stats.ComplianceScore >= 90},
		{Title: "5 дней с мягким SLA без просрочек", Points: 30, Completed: stats.ComplianceScore >= 80},
	}
	data["Trend"] = nutritionTrend()
	data["TrendBadge"] = "Последние 7 дней"
	data["StreakProgressPercent"] = streakProgressPercent
	data["Reminders"] = reminders
	data["WeeklyReview"] = review
	s.render(w, "nutrition_dashboard", data)
}

func (s *Site) nutritionPlanPage(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	now := time.Now()
	planDays := s.buildNutritionPlan(user.ID, now)

	data := s.nutritionBaseData(r, "План питания", "nutrition-plan")
	data["Error"] = r.URL.Query().Get("error")
	data["Success"] = r.URL.Query().Get("success")
	data["PlanDays"] = planDays
	data["DayCompletionPoints"] = nutritionDayCompletionPts
	data["Guidelines"] = []string{
		"Белок в каждом основном приеме пищи для поддержки восстановления мышц.",
		"Вода равномерно в течение дня, минимум 1.8 литра.",
		"Если прием пропущен, используйте умную замену по КБЖУ в 1 клик.",
	}
	s.render(w, "nutrition_plan", data)
}

func (s *Site) nutritionMealsPage(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	data := s.nutritionBaseData(r, "Блюда", "nutrition-meals")
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	category := strings.TrimSpace(r.URL.Query().Get("category"))
	selectedDay := normalizeNutritionDayKey(r.URL.Query().Get("day"))
	if selectedDay == "" {
		selectedDay = "monday"
	}
	selectedSlot := normalizeNutritionSlotKey(r.URL.Query().Get("slot"))
	if selectedSlot == "" && category != "" {
		selectedSlot = normalizeNutritionSlotKey(category)
	}
	if category == "" && selectedSlot != "" {
		category = nutritionSlotLabel(selectedSlot)
	}
	returnTo := nutritionSafeReturnPath(r.URL.Query().Get("return_to"))
	cards := nutritionMealLibrary()
	rules := s.nutritionDietRulesForUser(user.ID)

	filtered := make([]nutritionMealCard, 0, len(cards))
	q := strings.ToLower(query)
	for _, card := range cards {
		if !nutritionMealAllowed(card, rules) {
			continue
		}
		if category != "" && !strings.EqualFold(card.Category, category) {
			continue
		}
		if q != "" {
			blob := strings.ToLower(card.Name + " " + card.Description + " " + card.Category)
			if !strings.Contains(blob, q) {
				continue
			}
		}
		filtered = append(filtered, card)
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].Name < filtered[j].Name
	})

	data["Query"] = query
	data["Category"] = category
	data["SelectedDay"] = selectedDay
	data["DayOptions"] = nutritionDayOptions()
	data["SelectedSlot"] = selectedSlot
	data["SelectedSlotLabel"] = nutritionSlotLabel(selectedSlot)
	data["ReturnTo"] = returnTo
	data["Meals"] = filtered
	data["Error"] = r.URL.Query().Get("error")
	data["Success"] = r.URL.Query().Get("success")
	s.render(w, "nutrition_meals", data)
}

func (s *Site) nutritionMealAssign(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, nutritionMealsRedirectURL("", "", "monday", "", "Некорректные данные формы", "", ""), http.StatusSeeOther)
		return
	}

	mealID := strings.TrimSpace(chi.URLParam(r, "id"))
	meal, ok := nutritionMealByID(mealID)
	if !ok {
		http.Redirect(w, r, nutritionMealsRedirectURL(r.FormValue("return_q"), r.FormValue("return_category"), r.FormValue("day"), "", "Блюдо не найдено", r.FormValue("target_slot"), r.FormValue("return_to")), http.StatusSeeOther)
		return
	}
	if !nutritionMealAllowed(meal, s.nutritionDietRulesForUser(user.ID)) {
		http.Redirect(w, r, nutritionMealsRedirectURL(r.FormValue("return_q"), r.FormValue("return_category"), r.FormValue("day"), "", "Блюдо не подходит под ограничения анкеты питания", r.FormValue("target_slot"), r.FormValue("return_to")), http.StatusSeeOther)
		return
	}

	dayKey := normalizeNutritionDayKey(r.FormValue("day"))
	if dayKey == "" {
		http.Redirect(w, r, nutritionMealsRedirectURL(r.FormValue("return_q"), r.FormValue("return_category"), "monday", "", "Выберите день для добавления", r.FormValue("target_slot"), r.FormValue("return_to")), http.StatusSeeOther)
		return
	}

	targetSlot := normalizeNutritionSlotKey(r.FormValue("target_slot"))
	if targetSlot == "" {
		derivedSlot, slotOK := nutritionSlotForCategory(meal.Category)
		if !slotOK {
			http.Redirect(w, r, nutritionMealsRedirectURL(r.FormValue("return_q"), r.FormValue("return_category"), dayKey, "", "Категория блюда не поддерживается", r.FormValue("target_slot"), r.FormValue("return_to")), http.StatusSeeOther)
			return
		}
		targetSlot = derivedSlot
	}

	plannedTime := nutritionSlotPlannedTime(targetSlot)
	if err := s.saveNutritionMealAssignment(user.ID, dayKey, targetSlot, meal, plannedTime, ""); err != nil {
		http.Redirect(w, r, nutritionMealsRedirectURL(r.FormValue("return_q"), r.FormValue("return_category"), dayKey, "", "Не удалось сохранить выбор блюда", r.FormValue("target_slot"), r.FormValue("return_to")), http.StatusSeeOther)
		return
	}

	s.insertNutritionEvent(user.ID, "План обновлен: «"+meal.Name+"» назначено на "+nutritionDayLabel(dayKey)+" ("+nutritionSlotLabel(targetSlot)+").")
	success := "Блюдо «" + meal.Name + "» добавлено на " + nutritionDayLabel(dayKey) + " (" + nutritionSlotLabel(targetSlot) + ")"

	if returnTo := nutritionSafeReturnPath(r.FormValue("return_to")); returnTo != "" {
		http.Redirect(w, r, nutritionPathWithMessage(returnTo, "success", success), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, nutritionMealsRedirectURL(r.FormValue("return_q"), r.FormValue("return_category"), dayKey, success, "", targetSlot, ""), http.StatusSeeOther)
}

func (s *Site) nutritionPlanMealComplete(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	dayKey := normalizeNutritionDayKey(chi.URLParam(r, "day"))
	slotKey := normalizeNutritionSlotKey(chi.URLParam(r, "slot"))
	if dayKey == "" || slotKey == "" {
		http.Redirect(w, r, "/nutrition/plan?error=Некорректный%20прием%20пищи", http.StatusSeeOther)
		return
	}

	slot, ok := s.resolveNutritionPlanSlot(user.ID, dayKey, slotKey, time.Now())
	if !ok {
		http.Redirect(w, r, "/nutrition/plan?error=Прием%20пищи%20не%20найден", http.StatusSeeOther)
		return
	}

	if err := s.upsertNutritionMealStatus(user.ID, dayKey, slotKey, slot, "completed", ""); err != nil {
		http.Redirect(w, r, "/nutrition/plan?error=Не%20удалось%20сохранить%20статус", http.StatusSeeOther)
		return
	}

	s.insertNutritionEvent(user.ID, "Выполнен прием пищи: "+slot.SlotLabel+" ("+slot.MealName+").")
	dayDate, _ := nutritionDayDate(nutritionWeekStart(time.Now()), dayKey)
	awarded, _ := s.refreshNutritionDayProgress(user.ID, dayKey, dayDate)

	success := "Отмечено: «" + slot.SlotLabel + "» выполнен"
	if awarded {
		success += " (+" + fmt.Sprintf("%d", nutritionDayCompletionPts) + " баллов за полностью закрытый день)"
		s.insertNutritionEvent(user.ID, "День питания закрыт полностью: начислено +"+fmt.Sprintf("%d", nutritionDayCompletionPts)+" баллов.")
	}
	http.Redirect(w, r, "/nutrition/plan?success="+url.QueryEscape(success), http.StatusSeeOther)
}

func (s *Site) nutritionPlanMealSkip(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	dayKey := normalizeNutritionDayKey(chi.URLParam(r, "day"))
	slotKey := normalizeNutritionSlotKey(chi.URLParam(r, "slot"))
	if dayKey == "" || slotKey == "" {
		http.Redirect(w, r, "/nutrition/plan?error=Некорректный%20прием%20пищи", http.StatusSeeOther)
		return
	}

	slot, ok := s.resolveNutritionPlanSlot(user.ID, dayKey, slotKey, time.Now())
	if !ok {
		http.Redirect(w, r, "/nutrition/plan?error=Прием%20пищи%20не%20найден", http.StatusSeeOther)
		return
	}

	if err := s.upsertNutritionMealStatus(user.ID, dayKey, slotKey, slot, "skipped", ""); err != nil {
		http.Redirect(w, r, "/nutrition/plan?error=Не%20удалось%20сохранить%20статус", http.StatusSeeOther)
		return
	}

	s.insertNutritionEvent(user.ID, "Прием пищи пропущен: "+slot.SlotLabel+" ("+slot.MealName+").")
	dayDate, _ := nutritionDayDate(nutritionWeekStart(time.Now()), dayKey)
	_, _ = s.refreshNutritionDayProgress(user.ID, dayKey, dayDate)
	http.Redirect(w, r, "/nutrition/plan?success="+url.QueryEscape("Прием отмечен как пропущенный. Нажмите «Умная замена» для быстрого эквивалента по КБЖУ."), http.StatusSeeOther)
}

func (s *Site) nutritionPlanMealSmartReplace(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	dayKey := normalizeNutritionDayKey(chi.URLParam(r, "day"))
	slotKey := normalizeNutritionSlotKey(chi.URLParam(r, "slot"))
	if dayKey == "" || slotKey == "" {
		http.Redirect(w, r, "/nutrition/plan?error=Некорректный%20прием%20пищи", http.StatusSeeOther)
		return
	}

	slot, ok := s.resolveNutritionPlanSlot(user.ID, dayKey, slotKey, time.Now())
	if !ok {
		http.Redirect(w, r, "/nutrition/plan?error=Прием%20пищи%20не%20найден", http.StatusSeeOther)
		return
	}

	replacement, reason := s.nutritionSmartReplacementForUser(user.ID, slot.toMealCard(), slotKey)
	if replacement == nil {
		replacement = nutritionFirstAllowedMealForSlot(slotKey, s.nutritionDietRulesForUser(user.ID))
		reason = "Подобран ближайший допустимый вариант по ограничениям анкеты."
	}
	if replacement == nil {
		http.Redirect(w, r, "/nutrition/plan?error=Эквивалент%20для%20замены%20не%20найден", http.StatusSeeOther)
		return
	}

	if err := s.saveNutritionMealAssignment(user.ID, dayKey, slotKey, *replacement, nutritionSlotPlannedTime(slotKey), slot.MealID); err != nil {
		http.Redirect(w, r, "/nutrition/plan?error=Не%20удалось%20применить%20замену", http.StatusSeeOther)
		return
	}

	s.insertNutritionEvent(user.ID, "Умная замена: "+slot.SlotLabel+" заменен на «"+replacement.Name+"».")
	success := "Умная замена применена: «" + replacement.Name + "». " + reason
	http.Redirect(w, r, "/nutrition/plan?success="+url.QueryEscape(success), http.StatusSeeOther)
}

func (s *Site) nutritionLeaderboardPage(w http.ResponseWriter, r *http.Request) {
	data := s.nutritionBaseData(r, "Рейтинг питания", "nutrition-leaderboard")
	data["Leaderboard"] = []nutritionLeaderboardRow{
		{Name: "Алексей Иванов", Department: "Реакторный цех", Points: 420, Days: 26, Compliance: 92, Hydration: 90, LastCheckin: "Сегодня"},
		{Name: "Елена Петрова", Department: "Безопасность", Points: 390, Days: 24, Compliance: 89, Hydration: 86, LastCheckin: "Вчера"},
		{Name: "Максим Власов", Department: "Инженерный отдел", Points: 360, Days: 23, Compliance: 84, Hydration: 88, LastCheckin: "Сегодня"},
		{Name: "Ирина Смирнова", Department: "Логистика", Points: 340, Days: 21, Compliance: 81, Hydration: 83, LastCheckin: "2 дня назад"},
	}
	s.render(w, "nutrition_leaderboard", data)
}

func (s *Site) nutritionRewardsPage(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	rewards := append([]nutritionReward(nil), nutritionRewardsCatalog()...)
	sort.SliceStable(rewards, func(i, j int) bool {
		if rewards[i].PointsCost == rewards[j].PointsCost {
			return rewards[i].Title < rewards[j].Title
		}
		return rewards[i].PointsCost < rewards[j].PointsCost
	})

	data := s.nutritionBaseData(r, "Поощрения питания", "nutrition-rewards")
	data["Rewards"] = rewards
	data["Points"] = s.loadUserPoints(user.ID)
	data["RewardOwnedCounts"] = s.loadNutritionRewardOwnedCounts(user.ID)
	data["Error"] = r.URL.Query().Get("error")
	data["Success"] = r.URL.Query().Get("success")
	s.render(w, "nutrition_rewards", data)
}

func (s *Site) nutritionRewardRedeem(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	rewardID := strings.TrimSpace(chi.URLParam(r, "id"))
	if rewardID == "" {
		http.Redirect(w, r, "/nutrition/rewards?error=Не%20выбрано%20поощрение", http.StatusSeeOther)
		return
	}

	reward, ok := nutritionRewardByID(rewardID)
	if !ok {
		http.Redirect(w, r, "/nutrition/rewards?error=Поощрение%20не%20найдено", http.StatusSeeOther)
		return
	}

	_ = db.EnsureUserDefaults(s.DB, user.ID)
	tx, err := s.DB.Begin()
	if err != nil {
		http.Redirect(w, r, "/nutrition/rewards?error=Не%20удалось%20создать%20заявку", http.StatusSeeOther)
		return
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`update user_points
		 set points_balance = points_balance - $1,
		     updated_at = now()
		 where user_id = $2 and points_balance >= $1`,
		reward.PointsCost,
		user.ID,
	)
	if err != nil {
		http.Redirect(w, r, "/nutrition/rewards?error=Не%20удалось%20списать%20баллы", http.StatusSeeOther)
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		http.Redirect(w, r, "/nutrition/rewards?error=Недостаточно%20баллов", http.StatusSeeOther)
		return
	}

	_, err = tx.Exec(
		`insert into nutrition_reward_redemptions (user_id, reward_id, reward_title, points_cost, status)
		 values ($1, $2, $3, $4, 'issued')`,
		user.ID,
		reward.ID,
		reward.Title,
		reward.PointsCost,
	)
	if err != nil {
		http.Redirect(w, r, "/nutrition/rewards?error=Не%20удалось%20сохранить%20поощрение", http.StatusSeeOther)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Redirect(w, r, "/nutrition/rewards?error=Не%20удалось%20завершить%20операцию", http.StatusSeeOther)
		return
	}

	s.insertNutritionEvent(user.ID, "Получено поощрение: «"+reward.Title+"».")
	http.Redirect(w, r, "/nutrition/rewards?success="+url.QueryEscape("Поощрение «"+reward.Title+"» добавлено в профиль"), http.StatusSeeOther)
}

func (s *Site) nutritionAchievementsPage(w http.ResponseWriter, r *http.Request) {
	data := s.nutritionBaseData(r, "Достижения питания", "nutrition-achievements")
	data["Achievements"] = []nutritionAchievementView{
		{Title: "7 дней режима", Description: "7 дней подряд без пропуска основного рациона.", Icon: "🥗", Unlocked: true, Progress: 7, Total: 7, PointsReward: 40},
		{Title: "Водный баланс", Description: "Выполняйте норму воды 14 дней подряд.", Icon: "💧", Unlocked: false, Progress: 9, Total: 14, PointsReward: 50},
		{Title: "Белковый фокус", Description: "Достигайте цели по белку 10 дней подряд.", Icon: "🍗", Unlocked: false, Progress: 6, Total: 10, PointsReward: 45},
		{Title: "Стабильный ужин", Description: "Легкий ужин до 20:00 в течение 12 дней.", Icon: "🌙", Unlocked: false, Progress: 8, Total: 12, PointsReward: 35},
		{Title: "Месяц восстановления", Description: "30 дней по плану питания без больших отклонений.", Icon: "🏅", Unlocked: false, Progress: 18, Total: 30, PointsReward: 120},
	}
	s.render(w, "nutrition_achievements", data)
}

func (s *Site) nutritionProfilePage(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	questionnaire, updatedAt, _ := s.loadNutritionQuestionnaire(user.ID)
	summary := nutritionRestrictionSummary(questionnaire, updatedAt)

	data := s.nutritionBaseData(r, "Профиль питания", "nutrition-profile")
	data["Success"] = r.URL.Query().Get("success")
	data["Error"] = r.URL.Query().Get("error")
	data["Points"] = s.loadUserPoints(user.ID)
	currentStreak, bestStreak := s.loadNutritionStreak(user.ID)
	data["CurrentStreak"] = currentStreak
	data["BestStreak"] = bestStreak
	data["RewardHistory"] = s.loadNutritionRewardHistory(user.ID)
	data["RestrictionSummary"] = summary

	data["Profile"] = nutritionProfileView{
		EmployeeID:      user.EmployeeID,
		Department:      user.Department,
		Position:        user.Position,
		NutritionTarget: nutritionOrDefault(questionnaire.NutritionGoal, "Поддержка восстановления и стабильная энергия"),
		DailyCalories:   nutritionIntOrDefault(questionnaire.CaloriesTarget, 2100),
		WaterTarget:     nutritionOrDefault(questionnaire.WaterTargetLiters, "1.8") + " л/день",
		MealPattern:     nutritionOrDefault(questionnaire.MealPattern, "3 основных + 1 перекус"),
		Restrictions:    summary.SoftLimit,
	}
	s.render(w, "nutrition_profile", data)
}

func (s *Site) nutritionRewardUse(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	redemptionID := strings.TrimSpace(chi.URLParam(r, "id"))
	if redemptionID == "" {
		http.Redirect(w, r, "/nutrition/profile?error=Поощрение%20не%20выбрано", http.StatusSeeOther)
		return
	}

	res, err := s.DB.Exec(
		`update nutrition_reward_redemptions
		 set status = 'used', used_at = now()
		 where id = $1 and user_id = $2 and status = 'issued'`,
		redemptionID,
		user.ID,
	)
	if err != nil {
		http.Redirect(w, r, "/nutrition/profile?error=Не%20удалось%20обновить%20статус", http.StatusSeeOther)
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		http.Redirect(w, r, "/nutrition/profile?error=Поощрение%20уже%20использовано%20или%20недоступно", http.StatusSeeOther)
		return
	}

	s.insertNutritionEvent(user.ID, "Поощрение переведено в статус «Использовано».")
	http.Redirect(w, r, "/nutrition/profile?success=Поощрение%20отмечено%20как%20использованное", http.StatusSeeOther)
}

func (s *Site) nutritionSupportPage(w http.ResponseWriter, r *http.Request) {
	data := s.nutritionBaseData(r, "Поддержка питания", "nutrition-support")
	data["Contacts"] = []nutritionSupportContact{
		{
			Title:       "Нутрициолог проекта",
			Description: "Персональные вопросы по рациону, восстановлению и корректировке плана.",
			ActionLabel: "Почта",
			ActionValue: "nutrition-support@company.local",
		},
		{
			Title:       "Координатор реабилитации",
			Description: "Организационные вопросы по модулю питания и начислению поощрений.",
			ActionLabel: "Внутренний номер",
			ActionValue: "#4721",
		},
	}
	data["FAQ"] = []nutritionFAQItem{
		{Question: "Как часто обновляется план питания?", Answer: "План на неделю можно корректировать ежедневно в разделе «План питания»."},
		{Question: "Как работает умная замена?", Answer: "Система подбирает блюдо той же категории с ближайшим КБЖУ и применяет его в 1 клик."},
		{Question: "Когда начисляются баллы?", Answer: "Баллы начисляются автоматически при закрытии всех приемов пищи за день."},
	}
	s.render(w, "nutrition_support", data)
}

func (s *Site) nutritionBaseData(r *http.Request, title, active string) map[string]any {
	data := s.baseData(r, title, active)
	data["Module"] = "nutrition"
	return data
}

func (s *Site) buildNutritionPlan(userID string, now time.Time) []nutritionPlanDay {
	planDays := nutritionPlanWeek(now)
	assignments := s.loadNutritionMealAssignments(userID)
	rules := s.nutritionDietRulesForUser(userID)
	applyNutritionAssignments(planDays, assignments, now, rules)
	hydrationLogs := s.loadNutritionHydrationLogs(userID, nutritionWeekStart(now))
	applyNutritionHydrationReminders(planDays, hydrationLogs, now)
	return planDays
}

func (s *Site) resolveNutritionPlanSlot(userID, dayKey, slotKey string, now time.Time) (nutritionMealSlotView, bool) {
	planDays := s.buildNutritionPlan(userID, now)
	for _, day := range planDays {
		if day.DayKey != dayKey {
			continue
		}
		for _, slot := range day.Slots {
			if slot.SlotKey == slotKey {
				return slot, true
			}
		}
	}
	return nutritionMealSlotView{}, false
}

func (s *Site) saveNutritionMealAssignment(userID, dayKey, slot string, meal nutritionMealCard, plannedTime, smartSwapFrom string) error {
	_, err := s.DB.Exec(
		`insert into nutrition_plan_meals (
			user_id, day_key, meal_slot, meal_id, meal_name,
			calories, protein, carbs, fats,
			status, planned_time, smart_swap_from_meal_id,
			completed_at, skipped_at, updated_at
		 )
		 values ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'planned', $10, $11, null, null, now())
		 on conflict (user_id, day_key, meal_slot)
		 do update set meal_id = excluded.meal_id,
		               meal_name = excluded.meal_name,
		               calories = excluded.calories,
		               protein = excluded.protein,
		               carbs = excluded.carbs,
		               fats = excluded.fats,
		               status = 'planned',
		               planned_time = excluded.planned_time,
		               smart_swap_from_meal_id = excluded.smart_swap_from_meal_id,
		               completed_at = null,
		               skipped_at = null,
		               updated_at = now()`,
		userID,
		dayKey,
		slot,
		meal.ID,
		meal.Name,
		meal.Calories,
		meal.Protein,
		meal.Carbs,
		meal.Fats,
		plannedTime,
		nullIfEmpty(strings.TrimSpace(smartSwapFrom)),
	)
	return err
}

func (s *Site) upsertNutritionMealStatus(userID, dayKey, slot string, slotView nutritionMealSlotView, status, smartSwapFrom string) error {
	_, err := s.DB.Exec(
		`insert into nutrition_plan_meals (
			user_id, day_key, meal_slot, meal_id, meal_name,
			calories, protein, carbs, fats,
			status, planned_time, smart_swap_from_meal_id,
			completed_at, skipped_at, updated_at
		 )
		 values (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9,
			$10, $11, $12,
			case when $10 = 'completed' then now() else null end,
			case when $10 = 'skipped' then now() else null end,
			now()
		 )
		 on conflict (user_id, day_key, meal_slot)
		 do update set meal_id = excluded.meal_id,
		               meal_name = excluded.meal_name,
		               calories = excluded.calories,
		               protein = excluded.protein,
		               carbs = excluded.carbs,
		               fats = excluded.fats,
		               status = excluded.status,
		               planned_time = excluded.planned_time,
		               smart_swap_from_meal_id = excluded.smart_swap_from_meal_id,
		               completed_at = case when excluded.status = 'completed' then now() else null end,
		               skipped_at = case when excluded.status = 'skipped' then now() else null end,
		               updated_at = now()`,
		userID,
		dayKey,
		slot,
		slotView.MealID,
		slotView.MealName,
		slotView.Calories,
		slotView.Protein,
		slotView.Carbs,
		slotView.Fats,
		status,
		nutritionSlotPlannedTime(slot),
		nullIfEmpty(strings.TrimSpace(smartSwapFrom)),
	)
	return err
}

func (s *Site) refreshNutritionDayProgress(userID, dayKey string, dayDate time.Time) (bool, error) {
	var completedCount int
	if err := s.DB.QueryRow(
		`select count(*)
		 from nutrition_plan_meals
		 where user_id = $1 and day_key = $2 and status = 'completed'`,
		userID,
		dayKey,
	).Scan(&completedCount); err != nil {
		return false, err
	}

	dayCompleted := completedCount >= nutritionSlotsPerDay
	_, err := s.DB.Exec(
		`insert into nutrition_day_progress (
			user_id, day_date, day_key, completed_slots, total_slots, day_completed, updated_at
		 ) values ($1, $2, $3, $4, $5, $6, now())
		 on conflict (user_id, day_date)
		 do update set day_key = excluded.day_key,
		               completed_slots = excluded.completed_slots,
		               total_slots = excluded.total_slots,
		               day_completed = excluded.day_completed,
		               updated_at = now()`,
		userID,
		dayDate,
		dayKey,
		completedCount,
		nutritionSlotsPerDay,
		dayCompleted,
	)
	if err != nil {
		return false, err
	}

	if dayCompleted {
		_, _ = s.DB.Exec(
			`update nutrition_day_progress
			 set completed_at = coalesce(completed_at, now())
			 where user_id = $1 and day_date = $2`,
			userID,
			dayDate,
		)
	} else {
		_, _ = s.DB.Exec(
			`update nutrition_day_progress
			 set completed_at = null
			 where user_id = $1 and day_date = $2`,
			userID,
			dayDate,
		)
	}

	awarded := false
	if dayCompleted {
		var pointsAwarded bool
		_ = s.DB.QueryRow(
			`select coalesce(points_awarded, false)
			 from nutrition_day_progress
			 where user_id = $1 and day_date = $2`,
			userID,
			dayDate,
		).Scan(&pointsAwarded)
		if !pointsAwarded {
			_ = db.EnsureUserDefaults(s.DB, userID)
			_, err = s.DB.Exec(
				`update user_points
				 set points_balance = points_balance + $1,
				     points_total = points_total + $1,
				     updated_at = now()
				 where user_id = $2`,
				nutritionDayCompletionPts,
				userID,
			)
			if err == nil {
				_, _ = s.DB.Exec(
					`update nutrition_day_progress
					 set points_awarded = true
					 where user_id = $1 and day_date = $2`,
					userID,
					dayDate,
				)
				awarded = true
			}
		}
	}

	s.updateNutritionUserStats(userID)
	return awarded, nil
}

func (s *Site) loadNutritionMealAssignments(userID string) map[string]map[string]nutritionAssignmentRecord {
	assignments := map[string]map[string]nutritionAssignmentRecord{}
	rows, err := s.DB.Query(
		`select day_key, meal_slot, meal_id, meal_name,
		        coalesce(calories, 0), coalesce(protein, 0), coalesce(carbs, 0), coalesce(fats, 0),
		        coalesce(status, 'planned'), coalesce(planned_time, ''), completed_at, coalesce(smart_swap_from_meal_id, '')
		 from nutrition_plan_meals
		 where user_id = $1`,
		userID,
	)
	if err != nil {
		return s.loadNutritionMealAssignmentsLegacy(userID)
	}
	defer rows.Close()

	for rows.Next() {
		var dayKey string
		var slot string
		var mealID string
		var mealName string
		var calories int
		var protein int
		var carbs int
		var fats int
		var status string
		var plannedTime string
		var completedAt sql.NullTime
		var smartSwapFrom string
		if err := rows.Scan(&dayKey, &slot, &mealID, &mealName, &calories, &protein, &carbs, &fats, &status, &plannedTime, &completedAt, &smartSwapFrom); err != nil {
			continue
		}
		if normalizeNutritionDayKey(dayKey) == "" || normalizeNutritionSlotKey(slot) == "" {
			continue
		}
		card, ok := nutritionMealByID(mealID)
		if !ok {
			card = nutritionMealCard{
				ID:          mealID,
				Name:        mealName,
				Category:    nutritionCategoryBySlot(slot),
				Calories:    calories,
				Protein:     protein,
				Carbs:       carbs,
				Fats:        fats,
				Description: "Выбрано из библиотеки",
			}
		}
		if _, exists := assignments[dayKey]; !exists {
			assignments[dayKey] = map[string]nutritionAssignmentRecord{}
		}
		rec := nutritionAssignmentRecord{
			Meal:                card,
			Status:              normalizeNutritionMealStatus(status),
			PlannedTime:         strings.TrimSpace(plannedTime),
			SmartSwapFromMealID: strings.TrimSpace(smartSwapFrom),
		}
		if completedAt.Valid {
			rec.CompletedAt = completedAt.Time
		}
		assignments[dayKey][normalizeNutritionSlotKey(slot)] = rec
	}

	return assignments
}

func (s *Site) loadNutritionMealAssignmentsLegacy(userID string) map[string]map[string]nutritionAssignmentRecord {
	assignments := map[string]map[string]nutritionAssignmentRecord{}
	rows, err := s.DB.Query(
		`select day_key, meal_slot, meal_id, meal_name,
		        coalesce(calories, 0), coalesce(protein, 0), coalesce(carbs, 0), coalesce(fats, 0)
		 from nutrition_plan_meals
		 where user_id = $1`,
		userID,
	)
	if err != nil {
		return assignments
	}
	defer rows.Close()

	for rows.Next() {
		var dayKey, slot, mealID, mealName string
		var calories, protein, carbs, fats int
		if err := rows.Scan(&dayKey, &slot, &mealID, &mealName, &calories, &protein, &carbs, &fats); err != nil {
			continue
		}
		if normalizeNutritionDayKey(dayKey) == "" || normalizeNutritionSlotKey(slot) == "" {
			continue
		}
		card, ok := nutritionMealByID(mealID)
		if !ok {
			card = nutritionMealCard{ID: mealID, Name: mealName, Category: nutritionCategoryBySlot(slot), Calories: calories, Protein: protein, Carbs: carbs, Fats: fats}
		}
		if _, exists := assignments[dayKey]; !exists {
			assignments[dayKey] = map[string]nutritionAssignmentRecord{}
		}
		assignments[dayKey][normalizeNutritionSlotKey(slot)] = nutritionAssignmentRecord{Meal: card, Status: "planned"}
	}

	return assignments
}

func applyNutritionAssignments(planDays []nutritionPlanDay, assignments map[string]map[string]nutritionAssignmentRecord, now time.Time, rules nutritionDietRules) {
	for i := range planDays {
		completedSlots := 0
		for j := range planDays[i].Slots {
			slot := &planDays[i].Slots[j]
			if dayAssignments, exists := assignments[planDays[i].DayKey]; exists {
				if rec, ok := dayAssignments[slot.SlotKey]; ok {
					slot.MealID = rec.Meal.ID
					slot.MealName = rec.Meal.Name
					slot.Calories = rec.Meal.Calories
					slot.Protein = rec.Meal.Protein
					slot.Carbs = rec.Meal.Carbs
					slot.Fats = rec.Meal.Fats
					slot.Status = normalizeNutritionMealStatus(rec.Status)
					if strings.TrimSpace(rec.PlannedTime) != "" {
						slot.PlannedTime = strings.TrimSpace(rec.PlannedTime)
					}
					if !rec.CompletedAt.IsZero() {
						slot.CompletedAt = rec.CompletedAt.Format("15:04")
						slot.CompletedOnTime = nutritionIsCompletedOnTime(planDays[i].DayDate, slot.PlannedTime, rec.CompletedAt)
					}
				}
			}

			if slot.Status != "completed" && !nutritionMealAllowed(slot.toMealCard(), rules) {
				replacement, _ := nutritionSmartReplacementWithRules(slot.toMealCard(), slot.SlotKey, rules)
				if replacement == nil {
					replacement = nutritionFirstAllowedMealForSlot(slot.SlotKey, rules)
				}
				if replacement != nil {
					slot.MealID = replacement.ID
					slot.MealName = replacement.Name
					slot.Calories = replacement.Calories
					slot.Protein = replacement.Protein
					slot.Carbs = replacement.Carbs
					slot.Fats = replacement.Fats
				}
			}

			if slot.Status == "skipped" {
				if replacement, reason := nutritionSmartReplacementWithRules(slot.toMealCard(), slot.SlotKey, rules); replacement != nil {
					slot.SuggestedMeal = replacement
					slot.SuggestedReason = reason
				}
			}

			slot.ReminderStatus, slot.ReminderHint = nutritionMealReminder(planDays[i].DayDate, slot.PlannedTime, slot.Status, now)
			if slot.Status == "completed" {
				completedSlots++
			}
		}

		planDays[i].CompletedSlots = completedSlots
		switch {
		case completedSlots >= nutritionSlotsPerDay:
			planDays[i].Status = "completed"
		case completedSlots > 0:
			planDays[i].Status = "in_progress"
		case nutritionDateOnly(planDays[i].DayDate).Before(nutritionDateOnly(now)):
			planDays[i].Status = "skipped"
		default:
			planDays[i].Status = "pending"
		}
	}
}

func (slot nutritionMealSlotView) toMealCard() nutritionMealCard {
	return nutritionMealCard{
		ID:       slot.MealID,
		Name:     slot.MealName,
		Category: slot.SlotLabel,
		Calories: slot.Calories,
		Protein:  slot.Protein,
		Carbs:    slot.Carbs,
		Fats:     slot.Fats,
	}
}

func (s *Site) loadUserPoints(userID string) int {
	_ = db.EnsureUserDefaults(s.DB, userID)
	var points int
	_ = s.DB.QueryRow(`select coalesce(points_balance, 0) from user_points where user_id = $1`, userID).Scan(&points)
	return points
}

func (s *Site) loadNutritionCompletedDays(userID string) int {
	var days int
	_ = s.DB.QueryRow(
		`select count(*)
		 from nutrition_day_progress
		 where user_id = $1 and day_completed = true`,
		userID,
	).Scan(&days)
	return days
}

func (s *Site) loadNutritionHydrationDaysEstimate(userID string) int {
	var days int
	_ = s.DB.QueryRow(
		`select count(*)
		 from nutrition_day_progress
		 where user_id = $1 and day_completed = true and day_date >= current_date - interval '6 days'`,
		userID,
	).Scan(&days)
	if days > 7 {
		days = 7
	}
	return days
}

func (s *Site) updateNutritionUserStats(userID string) {
	rows, err := s.DB.Query(
		`select day_date
		 from nutrition_day_progress
		 where user_id = $1 and day_completed = true
		 order by day_date`,
		userID,
	)
	if err != nil {
		return
	}
	defer rows.Close()

	completedDays := []time.Time{}
	for rows.Next() {
		var day time.Time
		if err := rows.Scan(&day); err != nil {
			continue
		}
		completedDays = append(completedDays, nutritionDateOnly(day))
	}

	currentStreak, bestStreak := nutritionComputeStreaks(completedDays)
	var lastDay any
	if len(completedDays) > 0 {
		lastDay = completedDays[len(completedDays)-1]
	}
	_, _ = s.DB.Exec(
		`insert into nutrition_user_stats (
			user_id, current_streak, best_streak, total_completed_days, last_completed_day, updated_at
		 ) values ($1, $2, $3, $4, $5, now())
		 on conflict (user_id)
		 do update set current_streak = excluded.current_streak,
		               best_streak = excluded.best_streak,
		               total_completed_days = excluded.total_completed_days,
		               last_completed_day = excluded.last_completed_day,
		               updated_at = now()`,
		userID,
		currentStreak,
		bestStreak,
		len(completedDays),
		lastDay,
	)
}

func (s *Site) loadNutritionStreak(userID string) (int, int) {
	var current int
	var best int
	err := s.DB.QueryRow(
		`select coalesce(current_streak, 0), coalesce(best_streak, 0)
		 from nutrition_user_stats
		 where user_id = $1`,
		userID,
	).Scan(&current, &best)
	if err == nil {
		return current, best
	}
	s.updateNutritionUserStats(userID)
	_ = s.DB.QueryRow(
		`select coalesce(current_streak, 0), coalesce(best_streak, 0)
		 from nutrition_user_stats
		 where user_id = $1`,
		userID,
	).Scan(&current, &best)
	return current, best
}

func nutritionComputeStreaks(completedDays []time.Time) (int, int) {
	if len(completedDays) == 0 {
		return 0, 0
	}
	best := 1
	cur := 1
	for i := 1; i < len(completedDays); i++ {
		if nutritionIsConsecutivePlanDay(completedDays[i-1], completedDays[i]) {
			cur++
		} else {
			cur = 1
		}
		if cur > best {
			best = cur
		}
	}

	current := 1
	for i := len(completedDays) - 1; i > 0; i-- {
		if nutritionIsConsecutivePlanDay(completedDays[i-1], completedDays[i]) {
			current++
		} else {
			break
		}
	}
	return current, best
}

func nutritionIsConsecutivePlanDay(prev, next time.Time) bool {
	prev = nutritionDateOnly(prev)
	next = nutritionDateOnly(next)
	diff := int(next.Sub(prev).Hours() / 24)
	if diff == 1 {
		return true
	}
	return prev.Weekday() == time.Friday && next.Weekday() == time.Monday && diff == 3
}

func (s *Site) loadNutritionEvents(userID string, limit int) []nutritionEventView {
	if limit <= 0 {
		limit = 5
	}
	rows, err := s.DB.Query(
		`select message, created_at
		 from nutrition_events
		 where user_id = $1
		 order by created_at desc
		 limit $2`,
		userID,
		limit,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	events := []nutritionEventView{}
	for rows.Next() {
		var message string
		var created time.Time
		if err := rows.Scan(&message, &created); err != nil {
			continue
		}
		events = append(events, nutritionEventView{
			Message:   message,
			CreatedAt: created.Format("02.01 15:04"),
		})
	}
	return events
}

func (s *Site) insertNutritionEvent(userID, message string) {
	if strings.TrimSpace(message) == "" {
		return
	}
	_, _ = s.DB.Exec(
		`insert into nutrition_events (user_id, message)
		 values ($1, $2)`,
		userID,
		strings.TrimSpace(message),
	)
}

func nutritionBuildReminderItems(planDays []nutritionPlanDay, now time.Time) []nutritionReminderItem {
	items := []nutritionReminderItem{}
	todayKey := nutritionDayKeyFromWeekday(now.Weekday())
	var target *nutritionPlanDay

	for i := range planDays {
		if planDays[i].DayKey == todayKey {
			target = &planDays[i]
			break
		}
	}
	if target == nil {
		for i := range planDays {
			if !nutritionDateOnly(planDays[i].DayDate).Before(nutritionDateOnly(now)) {
				target = &planDays[i]
				break
			}
		}
	}
	if target == nil && len(planDays) > 0 {
		target = &planDays[0]
	}

	if target != nil {
		for _, slot := range target.Slots {
			items = append(items, nutritionReminderItem{
				Title: slot.SlotLabel + ": " + slot.MealName,
				Time:  slot.PlannedTime,
				State: slot.ReminderStatus,
				Hint:  slot.ReminderHint,
			})
		}
		for _, reminder := range target.HydrationReminders {
			items = append(items, nutritionReminderItem{
				Title: "Вода",
				Time:  reminder.Time,
				State: reminder.Status,
				Hint:  reminder.Hint,
			})
		}
	}
	return items
}

func nutritionBuildWeeklyReview(planDays []nutritionPlanDay) nutritionWeeklyReview {
	total := 0
	completed := 0
	skipped := 0
	onTime := 0
	breakfastDone := 0

	for _, day := range planDays {
		for _, slot := range day.Slots {
			total++
			switch slot.Status {
			case "completed":
				completed++
				if slot.CompletedOnTime {
					onTime++
				}
				if slot.SlotKey == "breakfast" {
					breakfastDone++
				}
			case "skipped":
				skipped++
			}
		}
	}

	completionRate := 0
	onTimeRate := 0
	if total > 0 {
		completionRate = int(float64(completed) / float64(total) * 100)
		onTimeRate = int(float64(onTime) / float64(total) * 100)
	}

	strengthCandidates := []string{}
	if completionRate >= 75 {
		strengthCandidates = append(strengthCandidates, fmt.Sprintf("Соблюдение недельного плана: %d%%.", completionRate))
	}
	if onTimeRate >= 60 {
		strengthCandidates = append(strengthCandidates, fmt.Sprintf("Приемы в рамках мягкого SLA: %d%%.", onTimeRate))
	}
	if breakfastDone >= 4 {
		strengthCandidates = append(strengthCandidates, "Стабильный старт дня: завтраки закрываются регулярно.")
	}
	if skipped <= 2 {
		strengthCandidates = append(strengthCandidates, "Низкий уровень пропусков приемов пищи.")
	}
	for len(strengthCandidates) < 3 {
		strengthCandidates = append(strengthCandidates, "План питания поддерживается в стабильном рабочем режиме.")
	}

	improvements := []string{}
	if skipped > 0 {
		improvements = append(improvements, fmt.Sprintf("Сократить пропуски приемов пищи (сейчас: %d).", skipped))
	}
	if onTimeRate < 60 {
		improvements = append(improvements, "Смещать отметки о приеме ближе к плановому времени для SLA.")
	}
	if breakfastDone < 4 {
		improvements = append(improvements, "Укрепить дисциплину по завтракам (целевой минимум: 4 из 5).")
	}
	for len(improvements) < 2 {
		improvements = append(improvements, "Добавить одну дополнительную водную точку в середине смены.")
	}

	return nutritionWeeklyReview{
		Strengths:    strengthCandidates[:3],
		Improvements: improvements[:2],
	}
}

func nutritionNextMeal(planDays []nutritionPlanDay, now time.Time) *nutritionMealSchedule {
	type candidate struct {
		dayDate time.Time
		timeVal time.Time
		slot    nutritionMealSlotView
	}
	candidates := []candidate{}
	for _, day := range planDays {
		for _, slot := range day.Slots {
			if slot.Status == "completed" {
				continue
			}
			timeVal, ok := nutritionParseSlotDateTime(day.DayDate, slot.PlannedTime)
			if !ok {
				continue
			}
			if timeVal.Before(now.Add(-2 * time.Hour)) {
				continue
			}
			candidates = append(candidates, candidate{dayDate: day.DayDate, timeVal: timeVal, slot: slot})
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].timeVal.Before(candidates[j].timeVal)
	})
	ch := candidates[0].slot
	return &nutritionMealSchedule{
		Name:        ch.MealName,
		Description: ch.SlotLabel + " · " + nutritionDayLabel(ch.DayKey),
		Time:        ch.PlannedTime,
		Calories:    ch.Calories,
		Protein:     ch.Protein,
		Carbs:       ch.Carbs,
		Fats:        ch.Fats,
	}
}

func nutritionCompletionPercent(planDays []nutritionPlanDay) int {
	total := 0
	completed := 0
	for _, day := range planDays {
		for _, slot := range day.Slots {
			total++
			if slot.Status == "completed" {
				completed++
			}
		}
	}
	if total == 0 {
		return 0
	}
	return int(float64(completed) / float64(total) * 100)
}

func nutritionPlanWeek(now time.Time) []nutritionPlanDay {
	weekStart := nutritionWeekStart(now)
	dayOptions := nutritionDayOptions()

	focuses := []string{
		"Старт недели",
		"Стабильный белок",
		"Контроль соли",
		"Равномерная энергия",
		"Поддержка перед выходными",
	}
	hydration := []string{"1.9 л", "2.0 л", "1.8 л", "1.9 л", "2.0 л"}

	breakfastIDs := []string{"meal-breakfast-1", "meal-breakfast-2", "meal-breakfast-3", "meal-breakfast-4", "meal-breakfast-1"}
	lunchIDs := []string{"meal-lunch-1", "meal-lunch-2", "meal-lunch-3", "meal-lunch-4", "meal-lunch-1"}
	dinnerIDs := []string{"meal-dinner-1", "meal-dinner-2", "meal-dinner-3", "meal-dinner-4", "meal-dinner-1"}
	snackIDs := []string{"meal-snack-1", "meal-snack-2", "meal-snack-3", "meal-snack-4", "meal-snack-1"}

	planDays := make([]nutritionPlanDay, 0, len(dayOptions))
	for i, dayOption := range dayOptions {
		dayDate := weekStart.AddDate(0, 0, i)
		slots := []nutritionMealSlotView{
			nutritionSlotFromMeal(dayOption.Key, "breakfast", breakfastIDs[i]),
			nutritionSlotFromMeal(dayOption.Key, "lunch", lunchIDs[i]),
			nutritionSlotFromMeal(dayOption.Key, "dinner", dinnerIDs[i]),
			nutritionSlotFromMeal(dayOption.Key, "snack", snackIDs[i]),
		}
		planDays = append(planDays, nutritionPlanDay{
			DayKey:    dayOption.Key,
			DayLabel:  dayOption.Label,
			DayDate:   dayDate,
			DateLabel: dayDate.Format("02.01"),
			Status:    "pending",
			Focus:     focuses[i],
			Hydration: hydration[i],
			Slots:     slots,
		})
	}
	return planDays
}

func nutritionSlotFromMeal(dayKey, slotKey, mealID string) nutritionMealSlotView {
	meal, ok := nutritionMealByID(mealID)
	if !ok {
		meal = nutritionFallbackMealForSlot(slotKey)
	}
	return nutritionMealSlotView{
		DayKey:      dayKey,
		SlotKey:     slotKey,
		SlotLabel:   nutritionSlotLabel(slotKey),
		PlannedTime: nutritionSlotPlannedTime(slotKey),
		MealID:      meal.ID,
		MealName:    meal.Name,
		Calories:    meal.Calories,
		Protein:     meal.Protein,
		Carbs:       meal.Carbs,
		Fats:        meal.Fats,
		Status:      "planned",
	}
}

func nutritionFallbackMealForSlot(slotKey string) nutritionMealCard {
	category := nutritionSlotLabel(slotKey)
	for _, meal := range nutritionMealLibrary() {
		if strings.EqualFold(meal.Category, category) {
			return meal
		}
	}
	return nutritionMealCard{ID: "fallback", Name: "Блюдо по умолчанию", Category: category}
}

func nutritionSmartReplacement(current nutritionMealCard, slotKey string) (*nutritionMealCard, string) {
	candidates := nutritionMealsBySlot(slotKey)
	bestIdx := -1
	bestScore := 1<<31 - 1
	for idx, candidate := range candidates {
		if candidate.ID == current.ID {
			continue
		}
		score := nutritionMealDistance(current, candidate)
		if score < bestScore {
			bestScore = score
			bestIdx = idx
		}
	}
	if bestIdx < 0 {
		return nil, ""
	}
	best := candidates[bestIdx]
	reason := fmt.Sprintf("Эквивалент по КБЖУ: %d ккал, Б %d г, У %d г, Ж %d г.", best.Calories, best.Protein, best.Carbs, best.Fats)
	return &best, reason
}

func nutritionMealDistance(a, b nutritionMealCard) int {
	return nutritionAbs(a.Calories-b.Calories) +
		nutritionAbs(a.Protein-b.Protein)*2 +
		nutritionAbs(a.Carbs-b.Carbs) +
		nutritionAbs(a.Fats-b.Fats)
}

func nutritionAbs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func nutritionMealsBySlot(slotKey string) []nutritionMealCard {
	category := nutritionSlotLabel(slotKey)
	meals := []nutritionMealCard{}
	for _, meal := range nutritionMealLibrary() {
		if strings.EqualFold(meal.Category, category) {
			meals = append(meals, meal)
		}
	}
	return meals
}

func nutritionWeekStart(now time.Time) time.Time {
	now = nutritionDateOnly(now)
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return now.AddDate(0, 0, -(weekday - 1))
}

func nutritionDayDate(weekStart time.Time, dayKey string) (time.Time, bool) {
	offset := map[string]int{
		"monday":    0,
		"tuesday":   1,
		"wednesday": 2,
		"thursday":  3,
		"friday":    4,
	}
	key := normalizeNutritionDayKey(dayKey)
	idx, ok := offset[key]
	if !ok {
		return time.Time{}, false
	}
	return weekStart.AddDate(0, 0, idx), true
}

func nutritionDateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func nutritionDayOptions() []nutritionDayOption {
	return []nutritionDayOption{
		{Key: "monday", Label: "Понедельник"},
		{Key: "tuesday", Label: "Вторник"},
		{Key: "wednesday", Label: "Среда"},
		{Key: "thursday", Label: "Четверг"},
		{Key: "friday", Label: "Пятница"},
	}
}

func normalizeNutritionDayKey(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "monday", "понедельник":
		return "monday"
	case "tuesday", "вторник":
		return "tuesday"
	case "wednesday", "среда":
		return "wednesday"
	case "thursday", "четверг":
		return "thursday"
	case "friday", "пятница":
		return "friday"
	default:
		return ""
	}
}

func nutritionDayLabel(dayKey string) string {
	for _, option := range nutritionDayOptions() {
		if option.Key == normalizeNutritionDayKey(dayKey) {
			return option.Label
		}
	}
	return ""
}

func nutritionDayKeyFromWeekday(weekday time.Weekday) string {
	switch weekday {
	case time.Monday:
		return "monday"
	case time.Tuesday:
		return "tuesday"
	case time.Wednesday:
		return "wednesday"
	case time.Thursday:
		return "thursday"
	case time.Friday:
		return "friday"
	default:
		return ""
	}
}

func normalizeNutritionSlotKey(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "breakfast", "завтрак":
		return "breakfast"
	case "lunch", "обед":
		return "lunch"
	case "dinner", "ужин":
		return "dinner"
	case "snack", "перекус":
		return "snack"
	default:
		return ""
	}
}

func nutritionSlotForCategory(category string) (string, bool) {
	slot := normalizeNutritionSlotKey(category)
	return slot, slot != ""
}

func nutritionSlotLabel(slot string) string {
	switch normalizeNutritionSlotKey(slot) {
	case "breakfast":
		return "Завтрак"
	case "lunch":
		return "Обед"
	case "dinner":
		return "Ужин"
	case "snack":
		return "Перекус"
	default:
		return ""
	}
}

func nutritionCategoryBySlot(slot string) string {
	return nutritionSlotLabel(slot)
}

func nutritionSlotPlannedTime(slot string) string {
	switch normalizeNutritionSlotKey(slot) {
	case "breakfast":
		return "08:30"
	case "lunch":
		return "13:00"
	case "dinner":
		return "19:00"
	case "snack":
		return "16:30"
	default:
		return "12:00"
	}
}

func nutritionParseSlotDateTime(dayDate time.Time, hhmm string) (time.Time, bool) {
	hhmm = strings.TrimSpace(hhmm)
	if hhmm == "" {
		return time.Time{}, false
	}
	parts := strings.Split(hhmm, ":")
	if len(parts) != 2 {
		return time.Time{}, false
	}
	hour := 0
	min := 0
	_, errH := fmt.Sscanf(parts[0], "%d", &hour)
	_, errM := fmt.Sscanf(parts[1], "%d", &min)
	if errH != nil || errM != nil || hour < 0 || hour > 23 || min < 0 || min > 59 {
		return time.Time{}, false
	}
	return time.Date(dayDate.Year(), dayDate.Month(), dayDate.Day(), hour, min, 0, 0, dayDate.Location()), true
}

func nutritionIsCompletedOnTime(dayDate time.Time, plannedTime string, completedAt time.Time) bool {
	due, ok := nutritionParseSlotDateTime(dayDate, plannedTime)
	if !ok {
		return true
	}
	return !completedAt.After(due.Add(time.Duration(nutritionReminderSLAMinutes) * time.Minute))
}

func nutritionMealReminder(dayDate time.Time, plannedTime, status string, now time.Time) (string, string) {
	status = normalizeNutritionMealStatus(status)
	if status == "completed" {
		return "Выполнено", "Прием закрыт"
	}
	today := nutritionDateOnly(now)
	if nutritionDateOnly(dayDate).Before(today) {
		if status == "skipped" {
			return "Пропущено", "Используйте умную замену для корректировки"
		}
		return "Просрочено", "Плановый прием не закрыт"
	}
	if nutritionDateOnly(dayDate).After(today) {
		return "Запланировано", "Прием по графику"
	}
	due, ok := nutritionParseSlotDateTime(dayDate, plannedTime)
	if !ok {
		return "По графику", "Время приема не задано"
	}
	if now.Before(due) {
		return "По графику", "До приема по плану"
	}
	slaEdge := due.Add(time.Duration(nutritionReminderSLAMinutes) * time.Minute)
	if now.Before(slaEdge) {
		return "Мягкий SLA", "Рекомендуется закрыть прием в течение часа"
	}
	return "Просрочено", "Требуется закрытие или замена"
}

func nutritionGenericReminderState(now time.Time, hhmm string) (string, string) {
	today := nutritionDateOnly(now)
	planned, ok := nutritionParseSlotDateTime(today, hhmm)
	if !ok {
		return "Напоминание", "Контрольная точка воды"
	}
	if now.Before(planned) {
		return "Напоминание", "Плановая точка воды"
	}
	if now.Before(planned.Add(time.Duration(nutritionReminderSLAMinutes) * time.Minute)) {
		return "Мягкий SLA", "Выполните водный чекпоинт в течение часа"
	}
	return "Просрочено", "Контрольный питьевой слот пропущен"
}

func normalizeNutritionMealStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed":
		return "completed"
	case "skipped":
		return "skipped"
	default:
		return "planned"
	}
}

func nutritionPathWithMessage(path, key, message string) string {
	if !strings.HasPrefix(path, "/nutrition") {
		return "/nutrition/plan"
	}
	values := url.Values{}
	if strings.TrimSpace(message) != "" {
		values.Set(key, strings.TrimSpace(message))
	}
	if encoded := values.Encode(); encoded != "" {
		if strings.Contains(path, "?") {
			return path + "&" + encoded
		}
		return path + "?" + encoded
	}
	return path
}

func nutritionSafeReturnPath(path string) string {
	path = strings.TrimSpace(path)
	if strings.HasPrefix(path, "/nutrition") {
		return path
	}
	return ""
}

func nutritionMealsRedirectURL(query, category, dayKey, success, errMsg, slot, returnTo string) string {
	values := url.Values{}
	if strings.TrimSpace(query) != "" {
		values.Set("q", strings.TrimSpace(query))
	}
	if strings.TrimSpace(category) != "" {
		values.Set("category", strings.TrimSpace(category))
	}
	if normalizeNutritionDayKey(dayKey) != "" {
		values.Set("day", normalizeNutritionDayKey(dayKey))
	}
	if normalizeNutritionSlotKey(slot) != "" {
		values.Set("slot", normalizeNutritionSlotKey(slot))
	}
	if nutritionSafeReturnPath(returnTo) != "" {
		values.Set("return_to", nutritionSafeReturnPath(returnTo))
	}
	if strings.TrimSpace(success) != "" {
		values.Set("success", strings.TrimSpace(success))
	}
	if strings.TrimSpace(errMsg) != "" {
		values.Set("error", strings.TrimSpace(errMsg))
	}
	result := "/nutrition/meals"
	if encoded := values.Encode(); encoded != "" {
		result += "?" + encoded
	}
	return result
}

func (s *Site) loadNutritionRewardOwnedCounts(userID string) map[string]int {
	counts := map[string]int{}
	rows, err := s.DB.Query(
		`select reward_id, count(*)
		 from nutrition_reward_redemptions
		 where user_id = $1
		 group by reward_id`,
		userID,
	)
	if err != nil {
		return counts
	}
	defer rows.Close()
	for rows.Next() {
		var rewardID string
		var count int
		if err := rows.Scan(&rewardID, &count); err != nil {
			continue
		}
		counts[rewardID] = count
	}
	return counts
}

func (s *Site) loadNutritionRewardHistory(userID string) []nutritionRewardHistoryView {
	rows, err := s.DB.Query(
		`select id, reward_title, points_cost, status, redeemed_at, used_at
		 from nutrition_reward_redemptions
		 where user_id = $1
		 order by redeemed_at desc`,
		userID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	history := []nutritionRewardHistoryView{}
	for rows.Next() {
		var item nutritionRewardHistoryView
		var redeemedAt time.Time
		var usedAt sql.NullTime
		if err := rows.Scan(&item.ID, &item.Title, &item.PointsCost, &item.Status, &redeemedAt, &usedAt); err != nil {
			continue
		}
		item.RedeemedAt = redeemedAt.Format("02.01.2006 15:04")
		if usedAt.Valid {
			item.UsedAt = usedAt.Time.Format("02.01.2006 15:04")
		}
		item.CanUse = strings.EqualFold(item.Status, "issued")
		history = append(history, item)
	}
	return history
}

func nutritionTrend() []nutritionTrendPoint {
	labels := []string{"Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"}
	values := []struct {
		compliance int
		hydration  int
	}{
		{84, 80},
		{88, 90},
		{83, 86},
		{91, 94},
		{86, 88},
		{78, 82},
		{90, 92},
	}
	trend := make([]nutritionTrendPoint, 0, len(values))
	for i, item := range values {
		trend = append(trend, nutritionTrendPoint{
			Label:             labels[i],
			Compliance:        item.compliance,
			CompliancePercent: item.compliance,
			Hydration:         item.hydration,
			HydrationPercent:  item.hydration,
		})
	}
	return trend
}

func nutritionMealLibrary() []nutritionMealCard {
	return []nutritionMealCard{
		{ID: "meal-breakfast-1", Name: "Каша овсяная молочная", Description: "Классический корпоративный завтрак с медленными углеводами.", Category: "Завтрак", Calories: 320, Protein: 12, Carbs: 46, Fats: 10},
		{ID: "meal-breakfast-2", Name: "Омлет паровой с зеленью", Description: "Легкий белковый завтрак для рабочего дня.", Category: "Завтрак", Calories: 290, Protein: 21, Carbs: 6, Fats: 19},
		{ID: "meal-breakfast-3", Name: "Сырники из творога 5%", Description: "Традиционный завтрак столовой с повышенным белком.", Category: "Завтрак", Calories: 360, Protein: 19, Carbs: 34, Fats: 16},
		{ID: "meal-breakfast-4", Name: "Гречневая каша с яйцом", Description: "Сытный завтрак для стабильной энергии.", Category: "Завтрак", Calories: 340, Protein: 17, Carbs: 37, Fats: 14},
		{ID: "meal-breakfast-5", Name: "Пшенная каша с тыквой", Description: "Стандартный теплый завтрак для первой половины смены.", Category: "Завтрак", Calories: 330, Protein: 11, Carbs: 49, Fats: 9},
		{ID: "meal-breakfast-6", Name: "Творог с ягодами и орехами", Description: "Белковый завтрак для контроля аппетита в течение дня.", Category: "Завтрак", Calories: 315, Protein: 24, Carbs: 19, Fats: 14},

		{ID: "meal-lunch-1", Name: "Суп куриный + котлета паровая с гречкой", Description: "Стандартный обед гос.корпорации с балансом белка и гарнира.", Category: "Обед", Calories: 560, Protein: 35, Carbs: 54, Fats: 21},
		{ID: "meal-lunch-2", Name: "Борщ + индейка тушеная с рисом", Description: "Горячее первое и второе для полноценного обеда.", Category: "Обед", Calories: 590, Protein: 39, Carbs: 58, Fats: 20},
		{ID: "meal-lunch-3", Name: "Щи + рыба на пару с картофельным пюре", Description: "Легкий обед с упором на восстановление.", Category: "Обед", Calories: 540, Protein: 33, Carbs: 57, Fats: 18},
		{ID: "meal-lunch-4", Name: "Суп чечевичный + говядина тушеная с рисом", Description: "Обед с высоким содержанием белка и железа.", Category: "Обед", Calories: 620, Protein: 42, Carbs: 61, Fats: 22},
		{ID: "meal-lunch-5", Name: "Рассольник + куриная грудка с перловкой", Description: "Классический столовый обед с умеренной калорийностью.", Category: "Обед", Calories: 575, Protein: 37, Carbs: 55, Fats: 19},
		{ID: "meal-lunch-6", Name: "Овощной суп + тефтели из индейки с булгуром", Description: "Обед для стабильной энергии без тяжести после приема пищи.", Category: "Обед", Calories: 545, Protein: 38, Carbs: 52, Fats: 17},

		{ID: "meal-dinner-1", Name: "Минтай запеченный + овощное рагу", Description: "Стандартный легкий ужин после рабочего дня.", Category: "Ужин", Calories: 430, Protein: 34, Carbs: 28, Fats: 18},
		{ID: "meal-dinner-2", Name: "Индейка на пару + салат овощной", Description: "Белковый ужин с низкой нагрузкой на ЖКТ.", Category: "Ужин", Calories: 410, Protein: 36, Carbs: 17, Fats: 20},
		{ID: "meal-dinner-3", Name: "Творожная запеканка + кефир", Description: "Мягкий ужин для восстановления и сна.", Category: "Ужин", Calories: 390, Protein: 29, Carbs: 24, Fats: 17},
		{ID: "meal-dinner-4", Name: "Куриная грудка + брокколи на пару", Description: "Ужин с контролируемой калорийностью.", Category: "Ужин", Calories: 420, Protein: 38, Carbs: 20, Fats: 18},
		{ID: "meal-dinner-5", Name: "Запеканка овощная с курицей", Description: "Легкий ужин в корпоративном стиле с повышенным белком.", Category: "Ужин", Calories: 405, Protein: 32, Carbs: 22, Fats: 17},
		{ID: "meal-dinner-6", Name: "Рыбные тефтели + салат из капусты", Description: "Ужин с акцентом на восстановление и контроль жиров.", Category: "Ужин", Calories: 400, Protein: 33, Carbs: 18, Fats: 16},

		{ID: "meal-snack-1", Name: "Кефир + цельнозерновые хлебцы", Description: "Базовый перекус между сменами.", Category: "Перекус", Calories: 210, Protein: 10, Carbs: 24, Fats: 8},
		{ID: "meal-snack-2", Name: "Яблоко + творог", Description: "Простой белковый перекус.", Category: "Перекус", Calories: 230, Protein: 16, Carbs: 25, Fats: 6},
		{ID: "meal-snack-3", Name: "Йогурт натуральный + орехи", Description: "Перекус для поддержки энергии и концентрации.", Category: "Перекус", Calories: 260, Protein: 11, Carbs: 15, Fats: 17},
		{ID: "meal-snack-4", Name: "Банан + протеиновый напиток", Description: "Перекус перед тренировкой или активной сменой.", Category: "Перекус", Calories: 280, Protein: 22, Carbs: 31, Fats: 6},
		{ID: "meal-snack-5", Name: "Груша + йогурт питьевой", Description: "Легкий перекус для удержания темпа между приемами пищи.", Category: "Перекус", Calories: 220, Protein: 9, Carbs: 30, Fats: 6},
		{ID: "meal-snack-6", Name: "Хумус + овощные палочки", Description: "Перекус с клетчаткой для стабильной концентрации в смене.", Category: "Перекус", Calories: 240, Protein: 10, Carbs: 19, Fats: 13},
	}
}

func nutritionMealByID(id string) (nutritionMealCard, bool) {
	for _, meal := range nutritionMealLibrary() {
		if meal.ID == strings.TrimSpace(id) {
			return meal, true
		}
	}
	return nutritionMealCard{}, false
}

func nutritionRewardsCatalog() []nutritionReward {
	return []nutritionReward{
		{
			ID:          "nutri-1",
			Title:       "Персональная консультация с нутрициологом 30 мин",
			Description: "Индивидуальный разбор рациона и корректировка питания под вашу динамику восстановления.",
			PointsCost:  190,
			Category:    "Консультация",
		},
		{
			ID:          "nutri-2",
			Title:       "Сертификат фитнес-обеда",
			Description: "Сертификат на полезный обед из партнерского меню корпоративного питания.",
			PointsCost:  140,
			Category:    "Питание",
		},
		{
			ID:          "nutri-3",
			Title:       "Сертификат на спорт-питание",
			Description: "Сертификат на базовый набор спортивного питания у партнера программы.",
			PointsCost:  230,
			Category:    "Бонус",
		},
		{
			ID:          "nutri-4",
			Title:       "Сертификат на недельный полезный ланч-набор",
			Description: "Набор сбалансированных обедов на 5 рабочих дней в корпоративной столовой.",
			PointsCost:  270,
			Category:    "Питание",
		},
		{
			ID:          "nutri-5",
			Title:       "Персональный разбор состава тела + план корректировки рациона",
			Description: "Диагностика состава тела и индивидуальный план питания на 4 недели.",
			PointsCost:  360,
			Category:    "Консультация",
		},
		{
			ID:          "nutri-6",
			Title:       "Доступ к закрытому мастер-классу по питанию в сменном графике",
			Description: "Практический мастер-класс по рациону при сменной работе с Q&A от экспертов.",
			PointsCost:  170,
			Category:    "Обучение",
		},
		{
			ID:          "nutri-7",
			Title:       "Корпоративный бокс полезных перекусов на месяц",
			Description: "Месячный набор полезных перекусов для поддержания энергии в течение смен.",
			PointsCost:  300,
			Category:    "Питание",
		},
		{
			ID:          "nutri-8",
			Title:       "Дополнительный здоровый выходной",
			Description: "Дополнительный день восстановления по внутреннему регламенту компании.",
			PointsCost:  450,
			Category:    "Бонус",
		},
	}
}

func nutritionRewardByID(id string) (nutritionReward, bool) {
	for _, reward := range nutritionRewardsCatalog() {
		if reward.ID == id {
			return reward, true
		}
	}
	return nutritionReward{}, false
}
