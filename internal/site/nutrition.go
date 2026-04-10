package site

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"rehab-app/internal/db"
	"rehab-app/internal/middleware"
)

const (
	nutritionSlotsPerDay        = 4
	nutritionDayCompletionPts   = 35
	nutritionDayComboPts        = 20
	nutritionWeekBasePts        = 25
	nutritionWeekMultiplier     = 2
	nutritionWeekTargetDays     = 5
	nutritionWeekNoSkipPts      = 30
	nutritionDailyPointsCap     = 300
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
	Timeline           []nutritionPlanTimelineItem
	CompletedSlots     int
}

type nutritionPlanTimelineItem struct {
	Kind      string
	Time      string
	Order     int
	Meal      nutritionMealSlotView
	Hydration nutritionHydrationReminderView
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
	MaxPerUser  int
	HasLimit    bool
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
	CorporateEmail  string
	Department      string
	Position        string
	Age             int
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
	ID             string
	Title          string
	PointsCost     int
	Status         string
	RequestedAt    string
	ReviewedAt     string
	UsedAt         string
	ReviewedBy     string
	ManagerComment string
	CanUse         bool
}

type nutritionRewardProgressView struct {
	Received  int
	Pending   int
	Rejected  int
	Limit     int
	HasLimit  bool
	Exhausted bool
}

type nutritionProfileLeaderboardRow struct {
	Rank      int
	Name      string
	Points    int
	IsCurrent bool
}

type nutritionProfileLeaderboardView struct {
	HasData bool
	Rank    int
	Points  int
	Rows    []nutritionProfileLeaderboardRow
}

type nutritionAssignmentRecord struct {
	Meal                nutritionMealCard
	Status              string
	PlannedTime         string
	CompletedAt         time.Time
	SmartSwapFromMealID string
}

func (s *Site) nutritionDashboardPage(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	now := time.Now()
	s.ensureNutritionMissedDayCompletionRewards(user.ID, now, 60)
	weekLength := len(nutritionDayOptions())
	s.ensureNutritionWeekRewards(user.ID, nutritionWeekStart(now), now)
	if weekLength > 0 {
		s.ensureNutritionWeekRewards(user.ID, nutritionWeekStart(now).AddDate(0, 0, -weekLength), now)
	}
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
		{Title: "5 дней с мягким допуском без просрочек", Points: 30, Completed: stats.ComplianceScore >= 80},
	}
	data["Trend"] = s.loadNutritionTrendForUser(user.ID, now)
	data["TrendBadge"] = "Последние 7 дней"
	data["StreakProgressPercent"] = streakProgressPercent
	data["Reminders"] = reminders
	data["WeeklyReview"] = review
	s.render(w, "nutrition_dashboard", data)
}

func (s *Site) nutritionPlanPage(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	now := time.Now()
	s.ensureNutritionMissedDayCompletionRewards(user.ID, now, 60)
	weekLength := len(nutritionDayOptions())
	s.ensureNutritionWeekRewards(user.ID, nutritionWeekStart(now), now)
	if weekLength > 0 {
		s.ensureNutritionWeekRewards(user.ID, nutritionWeekStart(now).AddDate(0, 0, -weekLength), now)
	}
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
	dayDate, ok := nutritionDayDate(nutritionWeekStart(time.Now()), dayKey)
	if !ok {
		http.Redirect(w, r, nutritionMealsRedirectURL(r.FormValue("return_q"), r.FormValue("return_category"), dayKey, "", "Не удалось определить дату выбранного дня", r.FormValue("target_slot"), r.FormValue("return_to")), http.StatusSeeOther)
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
	if err := s.saveNutritionMealAssignment(user.ID, dayDate, dayKey, targetSlot, meal, plannedTime, ""); err != nil {
		http.Redirect(w, r, nutritionMealsRedirectURL(r.FormValue("return_q"), r.FormValue("return_category"), dayKey, "", "Не удалось сохранить выбор блюда", r.FormValue("target_slot"), r.FormValue("return_to")), http.StatusSeeOther)
		return
	}

	s.insertNutritionEvent(user.ID, "План обновлен: «"+meal.Name+"» назначено на "+nutritionDayLabel(dayKey)+" ("+nutritionSlotLabel(targetSlot)+").")
	s.insertNutritionDayEvent(user.ID, dayKey, "meal_assigned", targetSlot, dayDate, map[string]any{
		"meal_id":   meal.ID,
		"meal_name": meal.Name,
	})
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
	dayDate, ok := nutritionDayDate(nutritionWeekStart(time.Now()), dayKey)
	if !ok {
		http.Redirect(w, r, "/nutrition/plan?error=Некорректный%20день", http.StatusSeeOther)
		return
	}

	slot, ok := s.resolveNutritionPlanSlot(user.ID, dayKey, slotKey, time.Now())
	if !ok {
		http.Redirect(w, r, "/nutrition/plan?error=Прием%20пищи%20не%20найден", http.StatusSeeOther)
		return
	}

	if err := s.upsertNutritionMealStatus(user.ID, dayDate, dayKey, slotKey, slot, "completed", ""); err != nil {
		http.Redirect(w, r, "/nutrition/plan?error=Не%20удалось%20сохранить%20статус", http.StatusSeeOther)
		return
	}

	s.insertNutritionEvent(user.ID, "Выполнен прием пищи: "+slot.SlotLabel+" ("+slot.MealName+").")
	s.insertNutritionDayEvent(user.ID, dayKey, "meal_completed", slotKey, dayDate, map[string]any{
		"meal_id":   slot.MealID,
		"meal_name": slot.MealName,
	})
	awardedDayPoints, progressErr := s.refreshNutritionDayProgress(user.ID, dayKey, dayDate)
	if progressErr != nil {
		log.Printf("nutrition: refresh day progress failed user=%s day=%s: %v", user.ID, dayDate.Format("2006-01-02"), progressErr)
		http.Redirect(w, r, "/nutrition/plan?error="+url.QueryEscape("Статус приема сохранен, но обновление баллов не удалось. Попробуйте еще раз."), http.StatusSeeOther)
		return
	}

	success := "Отмечено: «" + slot.SlotLabel + "» выполнен"
	if awardedDayPoints > 0 {
		success += " (+" + fmt.Sprintf("%d", awardedDayPoints) + " баллов за полностью закрытый день)"
		s.insertNutritionEvent(user.ID, "День питания закрыт полностью: начислено +"+fmt.Sprintf("%d", awardedDayPoints)+" баллов.")
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
	dayDate, ok := nutritionDayDate(nutritionWeekStart(time.Now()), dayKey)
	if !ok {
		http.Redirect(w, r, "/nutrition/plan?error=Некорректный%20день", http.StatusSeeOther)
		return
	}

	slot, ok := s.resolveNutritionPlanSlot(user.ID, dayKey, slotKey, time.Now())
	if !ok {
		http.Redirect(w, r, "/nutrition/plan?error=Прием%20пищи%20не%20найден", http.StatusSeeOther)
		return
	}

	if err := s.upsertNutritionMealStatus(user.ID, dayDate, dayKey, slotKey, slot, "skipped", ""); err != nil {
		http.Redirect(w, r, "/nutrition/plan?error=Не%20удалось%20сохранить%20статус", http.StatusSeeOther)
		return
	}

	s.insertNutritionEvent(user.ID, "Прием пищи пропущен: "+slot.SlotLabel+" ("+slot.MealName+").")
	s.insertNutritionDayEvent(user.ID, dayKey, "meal_skipped", slotKey, dayDate, map[string]any{
		"meal_id":   slot.MealID,
		"meal_name": slot.MealName,
	})
	if _, progressErr := s.refreshNutritionDayProgress(user.ID, dayKey, dayDate); progressErr != nil {
		log.Printf("nutrition: refresh day progress after skip failed user=%s day=%s: %v", user.ID, dayDate.Format("2006-01-02"), progressErr)
	}
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
	dayDate, ok := nutritionDayDate(nutritionWeekStart(time.Now()), dayKey)
	if !ok {
		http.Redirect(w, r, "/nutrition/plan?error=Некорректный%20день", http.StatusSeeOther)
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

	if err := s.saveNutritionMealAssignment(user.ID, dayDate, dayKey, slotKey, *replacement, nutritionSlotPlannedTime(slotKey), slot.MealID); err != nil {
		http.Redirect(w, r, "/nutrition/plan?error=Не%20удалось%20применить%20замену", http.StatusSeeOther)
		return
	}

	s.insertNutritionEvent(user.ID, "Умная замена: "+slot.SlotLabel+" заменен на «"+replacement.Name+"».")
	s.insertNutritionDayEvent(user.ID, dayKey, "meal_smart_swap", slotKey, dayDate, map[string]any{
		"from_meal_id":   slot.MealID,
		"from_meal_name": slot.MealName,
		"to_meal_id":     replacement.ID,
		"to_meal_name":   replacement.Name,
	})
	success := "Умная замена применена: «" + replacement.Name + "». " + reason
	http.Redirect(w, r, "/nutrition/plan?success="+url.QueryEscape(success), http.StatusSeeOther)
}

func (s *Site) nutritionLeaderboardPage(w http.ResponseWriter, r *http.Request) {
	data := s.nutritionBaseData(r, "Рейтинг питания", "nutrition-leaderboard")
	data["Leaderboard"] = s.loadNutritionLeaderboard(50)
	s.render(w, "nutrition_leaderboard", data)
}

func (s *Site) nutritionRewardsPage(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	rewards := s.loadNutritionRewardsCatalogWithLimits()
	sort.SliceStable(rewards, func(i, j int) bool {
		if rewards[i].PointsCost == rewards[j].PointsCost {
			return rewards[i].Title < rewards[j].Title
		}
		return rewards[i].PointsCost < rewards[j].PointsCost
	})

	data := s.nutritionBaseData(r, "Поощрения питания", "nutrition-rewards")
	data["Rewards"] = rewards
	data["Points"] = s.loadUserPoints(user.ID)
	data["RewardProgress"] = s.loadNutritionRewardProgress(user.ID, rewards)
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
	if s.loadUserPoints(user.ID) < reward.PointsCost {
		http.Redirect(w, r, "/nutrition/rewards?error=Недостаточно%20баллов%20для%20отправки%20заявки", http.StatusSeeOther)
		return
	}

	limit, hasLimit := s.loadNutritionRewardLimit(reward.ID)
	received, _ := s.loadNutritionRewardCountsForLimit(user.ID, reward.ID)
	if hasLimit && received >= limit {
		http.Redirect(w, r, "/nutrition/rewards?error="+url.QueryEscape("Лимит заявок на поощрение «"+reward.Title+"» исчерпан"), http.StatusSeeOther)
		return
	}
	var requestID string
	err := s.DB.QueryRow(
		`insert into nutrition_reward_redemptions (
		    user_id,
		    reward_id,
		    reward_title,
		    points_cost,
		    status,
		    requested_at,
		    manager_comment
		  )
		  values ($1, $2, $3, $4, 'pending', now(), '')
		  returning id::text`,
		user.ID,
		reward.ID,
		reward.Title,
		reward.PointsCost,
	).Scan(&requestID)
	if err != nil {
		http.Redirect(w, r, "/nutrition/rewards?error=Не%20удалось%20отправить%20заявку", http.StatusSeeOther)
		return
	}

	s.insertNutritionEvent(user.ID, "Отправлена заявка на поощрение: «"+reward.Title+"».")
	s.logNutritionAudit(
		user,
		"reward_request_created",
		"reward_request",
		requestID,
		user.ID,
		strings.TrimSpace(user.Department),
		map[string]any{
			"reward_id":    reward.ID,
			"reward_title": reward.Title,
			"points_cost":  reward.PointsCost,
		},
	)
	http.Redirect(w, r, "/nutrition/rewards?success="+url.QueryEscape("Заявка на поощрение «"+reward.Title+"» отправлена руководителю"), http.StatusSeeOther)
}

func (s *Site) nutritionAchievementsPage(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	data := s.nutritionBaseData(r, "Достижения питания", "nutrition-achievements")
	data["Achievements"] = s.loadNutritionAchievementsView(user.ID)
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
	data["ReminderSettings"] = s.loadNutritionReminderSettings(user.ID)
	data["RewardHistory"] = s.loadNutritionRewardHistory(user.ID)
	data["RestrictionSummary"] = summary
	data["LeaderboardCompact"] = s.loadNutritionProfileLeaderboard(user.ID)
	data["UserRole"] = strings.ToLower(strings.TrimSpace(user.Role))

	data["Profile"] = nutritionProfileView{
		EmployeeID:      user.EmployeeID,
		Department:      user.Department,
		Position:        user.Position,
		CorporateEmail:  user.CorporateEmail,
		Age:             questionnaire.Age,
		NutritionTarget: nutritionOrDefault(questionnaire.NutritionGoal, "Поддержка восстановления и стабильная энергия"),
		DailyCalories:   nutritionIntOrDefault(questionnaire.CaloriesTarget, 2100),
		WaterTarget:     nutritionOrDefault(questionnaire.WaterTargetLiters, "1.8") + " л/день",
		MealPattern:     nutritionOrDefault(questionnaire.MealPattern, "3 основных + 1 перекус"),
		Restrictions:    summary.SoftLimit,
	}
	s.render(w, "nutrition_profile", data)
}

func (s *Site) nutritionProfileReminderSettingsUpdate(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/nutrition/profile?error=Некорректные%20данные%20настроек", http.StatusSeeOther)
		return
	}

	leadMinutes, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("meal_reminder_lead_minutes")))
	slaMinutes, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("meal_sla_minutes")))
	settings := nutritionReminderSettings{
		MealReminderLeadMinutes: leadMinutes,
		MealSLAMinutes:          slaMinutes,
		Hydration1030Enabled:    strings.TrimSpace(r.FormValue("hydration_1030_enabled")) == "on",
		Hydration1500Enabled:    strings.TrimSpace(r.FormValue("hydration_1500_enabled")) == "on",
		Hydration1800Enabled:    strings.TrimSpace(r.FormValue("hydration_1800_enabled")) == "on",
	}
	if err := s.saveNutritionReminderSettings(user.ID, settings); err != nil {
		http.Redirect(w, r, "/nutrition/profile?error=Не%20удалось%20сохранить%20настройки%20напоминаний", http.StatusSeeOther)
		return
	}

	s.insertNutritionEvent(user.ID, "Настройки напоминаний по питанию обновлены.")
	http.Redirect(w, r, "/nutrition/profile?success=Настройки%20напоминаний%20обновлены", http.StatusSeeOther)
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
		 where id = $1 and user_id = $2 and status in ('approved', 'issued')`,
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
	s.nutritionSupportTicketsPage(w, r)
}

func (s *Site) nutritionBaseData(r *http.Request, title, active string) map[string]any {
	data := s.baseData(r, title, active)
	data["Module"] = "nutrition"
	return data
}

func (s *Site) buildNutritionPlan(userID string, now time.Time) []nutritionPlanDay {
	weekStart := nutritionWeekStart(now)
	planDays := nutritionPlanWeek(now)
	assignments := s.loadNutritionMealAssignments(userID, weekStart)
	rules := s.nutritionDietRulesForUser(userID)
	reminderSettings := s.loadNutritionReminderSettings(userID)
	applyNutritionAssignments(planDays, assignments, now, rules, reminderSettings)
	hydrationLogs := s.loadNutritionHydrationLogs(userID, weekStart)
	applyNutritionHydrationReminders(planDays, hydrationLogs, now, reminderSettings)
	for i := range planDays {
		planDays[i].Timeline = nutritionBuildPlanTimeline(planDays[i])
	}
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

func (s *Site) saveNutritionMealAssignment(userID string, dayDate time.Time, dayKey, slot string, meal nutritionMealCard, plannedTime, smartSwapFrom string) error {
	dayDate = nutritionDateOnly(dayDate)
	_, err := s.DB.Exec(
		`insert into nutrition_plan_meals (
			user_id, day_date, day_key, meal_slot, meal_id, meal_name,
			calories, protein, carbs, fats,
			status, planned_time, smart_swap_from_meal_id,
			completed_at, skipped_at, updated_at
		 )
		 values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'planned', $11, $12, null, null, now())
		 on conflict (user_id, day_date, meal_slot)
		 do update set day_key = excluded.day_key,
		               meal_id = excluded.meal_id,
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
		dayDate,
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

func (s *Site) upsertNutritionMealStatus(userID string, dayDate time.Time, dayKey, slot string, slotView nutritionMealSlotView, status, smartSwapFrom string) error {
	dayDate = nutritionDateOnly(dayDate)
	_, err := s.DB.Exec(
		`insert into nutrition_plan_meals (
			user_id, day_date, day_key, meal_slot, meal_id, meal_name,
			calories, protein, carbs, fats,
			status, planned_time, smart_swap_from_meal_id,
			completed_at, skipped_at, updated_at
		 )
		 values (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10,
			$11, $12, $13,
			case when $11 = 'completed' then now() else null end,
			case when $11 = 'skipped' then now() else null end,
			now()
		 )
		 on conflict (user_id, day_date, meal_slot)
		 do update set day_key = excluded.day_key,
		               meal_id = excluded.meal_id,
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
		dayDate,
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

func (s *Site) refreshNutritionDayProgress(userID, dayKey string, dayDate time.Time) (int, error) {
	dayDate = nutritionDateOnly(dayDate)
	var previousDayCompleted bool
	var previousPointsAwarded bool
	_ = s.DB.QueryRow(
		`select coalesce(day_completed, false), coalesce(points_awarded, false)
		 from nutrition_day_progress
		 where user_id = $1 and day_date = $2`,
		userID,
		dayDate,
	).Scan(&previousDayCompleted, &previousPointsAwarded)

	var completedCount int
	if err := s.DB.QueryRow(
		`select count(*)
		 from nutrition_plan_meals
		 where user_id = $1 and day_date = $2 and status = 'completed'`,
		userID,
		dayDate,
	).Scan(&completedCount); err != nil {
		return 0, err
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
		return 0, err
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

	awardedDayPoints := 0
	if dayCompleted && !previousPointsAwarded {
		if awarded, pointsErr := s.applyNutritionPointsChangeWithDailyCap(
			userID,
			dayDate,
			nutritionDayCompletionPts,
			"day_completion",
			"Начисление за полностью закрытый день питания",
			"nutrition_day_progress",
			dayDate.Format("2006-01-02"),
			"",
		); pointsErr == nil && awarded > 0 {
			_, _ = s.DB.Exec(
				`update nutrition_day_progress
				 set points_awarded = true
				 where user_id = $1 and day_date = $2`,
				userID,
				dayDate,
			)
			awardedDayPoints = awarded
		} else if pointsErr == nil {
			log.Printf(
				"nutrition: day-completion points capped by daily limit user=%s day=%s",
				userID,
				dayDate.Format("2006-01-02"),
			)
		} else {
			log.Printf(
				"nutrition: award points for day completion failed user=%s day=%s: %v",
				userID,
				dayDate.Format("2006-01-02"),
				pointsErr,
			)
		}
	}

	if dayCompleted {
		comboSourceID := dayDate.Format("2006-01-02") + ":combo"
		if s.nutritionDayHydrationComboCompleted(userID, dayKey, dayDate) &&
			!s.nutritionPositiveAwardExists(userID, "day_combo", "nutrition_day_progress", comboSourceID) {
			awardedCombo, comboErr := s.applyNutritionPointsChangeWithDailyCap(
				userID,
				dayDate,
				nutritionDayComboPts,
				"day_combo",
				"Комбо-день: еда и приемы воды закрыты по плану",
				"nutrition_day_progress",
				comboSourceID,
				"",
			)
			if comboErr != nil {
				log.Printf("nutrition: combo-day award failed user=%s day=%s: %v", userID, dayDate.Format("2006-01-02"), comboErr)
			} else if awardedCombo > 0 {
				s.insertNutritionEvent(userID, "Комбо-день: начислено +"+fmt.Sprintf("%d", awardedCombo)+" баллов.")
			}
		}
		s.ensureNutritionWeekRewards(userID, nutritionWeekStart(dayDate), dayDate)
	}

	s.updateNutritionUserStats(userID)
	if dayCompleted && !previousDayCompleted {
		s.insertNutritionDayEvent(userID, dayKey, "day_closed", "", dayDate, map[string]any{
			"completed_slots": completedCount,
		})
	}
	if !dayCompleted && previousDayCompleted {
		s.insertNutritionDayEvent(userID, dayKey, "day_reopened", "", dayDate, map[string]any{
			"completed_slots": completedCount,
		})
	}
	_, _ = s.refreshNutritionAchievements(userID)
	return awardedDayPoints, nil
}

func (s *Site) nutritionDayHydrationComboCompleted(userID, dayKey string, dayDate time.Time) bool {
	dayDate = nutritionDateOnly(dayDate)
	settings := s.loadNutritionReminderSettings(userID)
	required := len(nutritionHydrationReminderOptionsForSettings(settings))
	if required <= 0 {
		required = len(nutritionHydrationReminderOptions())
	}
	if required <= 0 {
		return true
	}

	var completed int
	_ = s.DB.QueryRow(
		`select count(*)
		 from nutrition_hydration_logs
		 where user_id = $1
		   and day_date = $2
		   and day_key = $3
		   and status = 'completed'`,
		userID,
		dayDate,
		normalizeNutritionDayKey(dayKey),
	).Scan(&completed)
	return completed >= required
}

func (s *Site) nutritionCompletedDaysInWeek(userID string, weekStart time.Time) int {
	weekStart = nutritionDateOnly(weekStart)
	weekEnd := weekStart.AddDate(0, 0, len(nutritionDayOptions()))
	var completed int
	_ = s.DB.QueryRow(
		`select count(*)
		 from nutrition_day_progress
		 where user_id = $1
		   and day_completed = true
		   and day_date >= $2
		   and day_date < $3`,
		userID,
		weekStart,
		weekEnd,
	).Scan(&completed)
	return completed
}

func (s *Site) nutritionWeekHasNoSkippedMeals(userID string, weekStart time.Time) bool {
	weekStart = nutritionDateOnly(weekStart)
	weekEnd := weekStart.AddDate(0, 0, len(nutritionDayOptions()))
	var skipped int
	_ = s.DB.QueryRow(
		`select count(*)
		 from nutrition_day_event_history
		 where user_id = $1
		   and day_date >= $2
		   and day_date < $3
		   and event_type = 'meal_skipped'`,
		userID,
		weekStart,
		weekEnd,
	).Scan(&skipped)
	return skipped == 0
}

func (s *Site) ensureNutritionWeekRewards(userID string, weekStart, awardDay time.Time) {
	if strings.TrimSpace(userID) == "" {
		return
	}
	weekStart = nutritionDateOnly(weekStart)
	if awardDay.IsZero() {
		awardDay = time.Now()
	}
	awardDay = nutritionDateOnly(awardDay)

	daysPerWeek := len(nutritionDayOptions())
	weekTargetDays := min(nutritionWeekTargetDays, daysPerWeek)
	if weekTargetDays <= 0 {
		return
	}

	weekCompletedDays := s.nutritionCompletedDaysInWeek(userID, weekStart)
	if weekCompletedDays < weekTargetDays {
		return
	}

	weekSourceID := weekStart.Format("2006-01-02") + ":week_full"
	weekAward := nutritionWeekBasePts * nutritionWeekMultiplier
	if !s.nutritionPositiveAwardExists(userID, "week_completion", "nutrition_week_progress", weekSourceID) {
		awardedWeek, weekErr := s.applyNutritionPointsChangeWithDailyCap(
			userID,
			awardDay,
			weekAward,
			"week_completion",
			"Бонус за "+fmt.Sprintf("%d", weekTargetDays)+" закрытых дней недели (мультипликатор x2)",
			"nutrition_week_progress",
			weekSourceID,
			"",
		)
		if weekErr != nil {
			log.Printf("nutrition: weekly completion award failed user=%s week=%s: %v", userID, weekStart.Format("2006-01-02"), weekErr)
		} else if awardedWeek > 0 {
			s.insertNutritionEvent(
				userID,
				"Неделя закрыта: "+fmt.Sprintf("%d", weekCompletedDays)+" дней, начислено +"+fmt.Sprintf("%d", awardedWeek)+" баллов (x2).",
			)
		}
	}

	noSkipSourceID := weekStart.Format("2006-01-02") + ":week_no_skip"
	if s.nutritionWeekHasNoSkippedMeals(userID, weekStart) &&
		!s.nutritionPositiveAwardExists(userID, "week_no_skip", "nutrition_week_progress", noSkipSourceID) {
		awardedNoSkip, noSkipErr := s.applyNutritionPointsChangeWithDailyCap(
			userID,
			awardDay,
			nutritionWeekNoSkipPts,
			"week_no_skip",
			"Анти-скип бонус: неделя без пропусков приемов пищи",
			"nutrition_week_progress",
			noSkipSourceID,
			"",
		)
		if noSkipErr != nil {
			log.Printf("nutrition: weekly no-skip award failed user=%s week=%s: %v", userID, weekStart.Format("2006-01-02"), noSkipErr)
		} else if awardedNoSkip > 0 {
			s.insertNutritionEvent(userID, "Анти-скип неделя: начислено +"+fmt.Sprintf("%d", awardedNoSkip)+" баллов.")
		}
	}
}

func (s *Site) ensureNutritionMissedDayCompletionRewards(userID string, awardDay time.Time, limit int) {
	if strings.TrimSpace(userID) == "" {
		return
	}
	if awardDay.IsZero() {
		awardDay = time.Now()
	}
	awardDay = nutritionDateOnly(awardDay)
	if limit <= 0 {
		limit = 30
	}

	rows, err := s.DB.Query(
		`select day_date
		 from nutrition_day_progress
		 where user_id = $1
		   and day_completed = true
		   and coalesce(points_awarded, false) = false
		 order by day_date asc
		 limit $2`,
		userID,
		limit,
	)
	if err != nil {
		log.Printf("nutrition: load missed day completion rewards failed user=%s: %v", userID, err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var dayDate time.Time
		if scanErr := rows.Scan(&dayDate); scanErr != nil {
			continue
		}
		dayDate = nutritionDateOnly(dayDate)
		sourceID := dayDate.Format("2006-01-02")
		if s.nutritionPositiveAwardExists(userID, "day_completion", "nutrition_day_progress", sourceID) {
			_, _ = s.DB.Exec(
				`update nutrition_day_progress
				 set points_awarded = true
				 where user_id = $1 and day_date = $2`,
				userID,
				dayDate,
			)
			continue
		}

		awarded, pointsErr := s.applyNutritionPointsChangeWithDailyCap(
			userID,
			awardDay,
			nutritionDayCompletionPts,
			"day_completion",
			"Начисление за полностью закрытый день питания",
			"nutrition_day_progress",
			sourceID,
			"",
		)
		if pointsErr != nil {
			log.Printf("nutrition: recover day-completion points failed user=%s day=%s: %v", userID, sourceID, pointsErr)
			continue
		}
		if awarded > 0 {
			_, _ = s.DB.Exec(
				`update nutrition_day_progress
				 set points_awarded = true
				 where user_id = $1 and day_date = $2`,
				userID,
				dayDate,
			)
		}
		s.ensureNutritionWeekRewards(userID, nutritionWeekStart(dayDate), awardDay)
	}
}

func (s *Site) nutritionPositiveAwardExists(userID, reasonCode, sourceType, sourceID string) bool {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(reasonCode) == "" || strings.TrimSpace(sourceType) == "" {
		return false
	}
	var exists bool
	cleanSourceID := strings.TrimSpace(sourceID)
	_ = s.DB.QueryRow(
		`select exists(
		   select 1
		   from nutrition_points_ledger
		   where user_id = $1
		     and reason_code = $2
		     and source_type = $3
		     and coalesce(source_id, '') = $4
		     and change_amount > 0
		 )`,
		userID,
		strings.TrimSpace(reasonCode),
		strings.TrimSpace(sourceType),
		cleanSourceID,
	).Scan(&exists)
	return exists
}

func (s *Site) loadNutritionMealAssignments(userID string, weekStart time.Time) map[string]map[string]nutritionAssignmentRecord {
	weekStart = nutritionDateOnly(weekStart)
	weekEnd := weekStart.AddDate(0, 0, len(nutritionDayOptions()))
	assignments := map[string]map[string]nutritionAssignmentRecord{}
	rows, err := s.DB.Query(
		`select day_date, day_key, meal_slot, meal_id, meal_name,
		        coalesce(calories, 0), coalesce(protein, 0), coalesce(carbs, 0), coalesce(fats, 0),
		        coalesce(status, 'planned'), coalesce(planned_time, ''), completed_at, coalesce(smart_swap_from_meal_id, '')
		 from nutrition_plan_meals
		 where user_id = $1 and day_date >= $2 and day_date < $3`,
		userID,
		weekStart,
		weekEnd,
	)
	if err != nil {
		return s.loadNutritionMealAssignmentsLegacy(userID)
	}
	defer rows.Close()

	for rows.Next() {
		var dayDate time.Time
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
		if err := rows.Scan(&dayDate, &dayKey, &slot, &mealID, &mealName, &calories, &protein, &carbs, &fats, &status, &plannedTime, &completedAt, &smartSwapFrom); err != nil {
			continue
		}
		normalizedDayKey := nutritionDayKeyFromWeekday(nutritionDateOnly(dayDate).Weekday())
		if normalizedDayKey == "" {
			normalizedDayKey = normalizeNutritionDayKey(dayKey)
		}
		normalizedSlot := normalizeNutritionSlotKey(slot)
		if normalizedDayKey == "" || normalizedSlot == "" {
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
		if _, exists := assignments[normalizedDayKey]; !exists {
			assignments[normalizedDayKey] = map[string]nutritionAssignmentRecord{}
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
		assignments[normalizedDayKey][normalizedSlot] = rec
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

func applyNutritionAssignments(
	planDays []nutritionPlanDay,
	assignments map[string]map[string]nutritionAssignmentRecord,
	now time.Time,
	rules nutritionDietRules,
	reminderSettings nutritionReminderSettings,
) {
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

			slot.ReminderStatus, slot.ReminderHint = nutritionMealReminder(
				planDays[i].DayDate,
				slot.PlannedTime,
				slot.Status,
				now,
				reminderSettings.MealSLAMinutes,
			)
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
	return diff == 1
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
		if len(target.Timeline) > 0 {
			for _, checkpoint := range target.Timeline {
				if checkpoint.Kind == "meal" {
					items = append(items, nutritionReminderItem{
						Title: checkpoint.Meal.SlotLabel + ": " + checkpoint.Meal.MealName,
						Time:  checkpoint.Meal.PlannedTime,
						State: checkpoint.Meal.ReminderStatus,
						Hint:  checkpoint.Meal.ReminderHint,
					})
					continue
				}
				items = append(items, nutritionReminderItem{
					Title: "Прием воды",
					Time:  checkpoint.Hydration.Time,
					State: checkpoint.Hydration.Status,
					Hint:  checkpoint.Hydration.Hint,
				})
			}
			return items
		}

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
				Title: "Прием воды",
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
		strengthCandidates = append(strengthCandidates, fmt.Sprintf("Приемы в рамках мягкого допуска по времени: %d%%.", onTimeRate))
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
		improvements = append(improvements, "Смещать отметки о приеме ближе к плановому времени для мягкого допуска.")
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
	next := candidates[0]
	ch := next.slot
	dayWithDate := nutritionDayLabel(ch.DayKey) + " (" + next.dayDate.Format("02.01") + ")"
	return &nutritionMealSchedule{
		Name:        ch.MealName,
		Description: ch.SlotLabel + " · " + dayWithDate,
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
		"Домашний режим: баланс воды",
		"Подготовка к новой неделе",
	}
	hydration := []string{"1.9 л", "2.0 л", "1.8 л", "1.9 л", "2.0 л", "1.9 л", "1.8 л"}

	breakfastIDs := []string{"meal-breakfast-1", "meal-breakfast-2", "meal-breakfast-3", "meal-breakfast-4", "meal-breakfast-1", "meal-breakfast-2", "meal-breakfast-3"}
	lunchIDs := []string{"meal-lunch-1", "meal-lunch-2", "meal-lunch-3", "meal-lunch-4", "meal-lunch-1", "meal-lunch-2", "meal-lunch-3"}
	dinnerIDs := []string{"meal-dinner-1", "meal-dinner-2", "meal-dinner-3", "meal-dinner-4", "meal-dinner-1", "meal-dinner-2", "meal-dinner-3"}
	snackIDs := []string{"meal-snack-1", "meal-snack-2", "meal-snack-3", "meal-snack-4", "meal-snack-1", "meal-snack-2", "meal-snack-3"}

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
		"saturday":  5,
		"sunday":    6,
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
		{Key: "saturday", Label: "Суббота"},
		{Key: "sunday", Label: "Воскресенье"},
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
	case "saturday", "суббота":
		return "saturday"
	case "sunday", "воскресенье":
		return "sunday"
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
	case time.Saturday:
		return "saturday"
	case time.Sunday:
		return "sunday"
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

func nutritionPlanCheckpointOrder(dayDate time.Time, hhmm string) int {
	due, ok := nutritionParseSlotDateTime(dayDate, hhmm)
	if !ok {
		return 24*60 + 1
	}
	return due.Hour()*60 + due.Minute()
}

func nutritionBuildPlanTimeline(day nutritionPlanDay) []nutritionPlanTimelineItem {
	timeline := make([]nutritionPlanTimelineItem, 0, len(day.Slots)+len(day.HydrationReminders))
	for _, slot := range day.Slots {
		timeLabel := strings.TrimSpace(slot.PlannedTime)
		timeline = append(timeline, nutritionPlanTimelineItem{
			Kind:  "meal",
			Time:  timeLabel,
			Order: nutritionPlanCheckpointOrder(day.DayDate, timeLabel),
			Meal:  slot,
		})
	}
	for _, reminder := range day.HydrationReminders {
		timeLabel := strings.TrimSpace(reminder.Time)
		timeline = append(timeline, nutritionPlanTimelineItem{
			Kind:      "hydration",
			Time:      timeLabel,
			Order:     nutritionPlanCheckpointOrder(day.DayDate, timeLabel),
			Hydration: reminder,
		})
	}

	sort.SliceStable(timeline, func(i, j int) bool {
		if timeline[i].Order != timeline[j].Order {
			return timeline[i].Order < timeline[j].Order
		}
		if timeline[i].Kind != timeline[j].Kind {
			return timeline[i].Kind == "meal"
		}
		return timeline[i].Time < timeline[j].Time
	})

	return timeline
}

func nutritionIsCompletedOnTime(dayDate time.Time, plannedTime string, completedAt time.Time) bool {
	due, ok := nutritionParseSlotDateTime(dayDate, plannedTime)
	if !ok {
		return true
	}
	return !completedAt.After(due.Add(time.Duration(nutritionReminderSLAMinutes) * time.Minute))
}

func nutritionMealReminder(dayDate time.Time, plannedTime, status string, now time.Time, slaMinutes int) (string, string) {
	status = normalizeNutritionMealStatus(status)
	if slaMinutes < 15 {
		slaMinutes = nutritionReminderSLAMinutes
	}
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
	slaEdge := due.Add(time.Duration(slaMinutes) * time.Minute)
	if now.Before(slaEdge) {
		return "Мягкий допуск", "Рекомендуется закрыть прием в течение часа"
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
		return "Мягкий допуск", "Выполните прием воды в течение часа"
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

func (s *Site) loadNutritionRewardsCatalogWithLimits() []nutritionReward {
	rewards := append([]nutritionReward(nil), nutritionRewardsCatalog()...)
	rows, err := s.DB.Query(
		`select reward_id, max_per_user
		 from nutrition_reward_limits
		 where max_per_user is not null and max_per_user > 0`,
	)
	if err != nil {
		return rewards
	}
	defer rows.Close()

	limits := map[string]int{}
	for rows.Next() {
		var rewardID string
		var limit int
		if scanErr := rows.Scan(&rewardID, &limit); scanErr != nil {
			continue
		}
		limits[strings.TrimSpace(rewardID)] = limit
	}

	for i := range rewards {
		limit, ok := limits[rewards[i].ID]
		if !ok || limit <= 0 {
			continue
		}
		rewards[i].HasLimit = true
		rewards[i].MaxPerUser = limit
	}
	return rewards
}

func (s *Site) loadNutritionRewardLimit(rewardID string) (int, bool) {
	var maxPerUser sql.NullInt64
	err := s.DB.QueryRow(
		`select max_per_user
		 from nutrition_reward_limits
		 where reward_id = $1`,
		strings.TrimSpace(rewardID),
	).Scan(&maxPerUser)
	if err != nil || !maxPerUser.Valid || maxPerUser.Int64 <= 0 {
		return 0, false
	}
	return int(maxPerUser.Int64), true
}

func (s *Site) loadNutritionRewardCountsForLimit(userID, rewardID string) (int, int) {
	var received int
	var pending int
	_ = s.DB.QueryRow(
		`select
		    count(*) filter (where lower(btrim(coalesce(status, ''))) in ('approved', 'issued', 'used', 'completed')) as received,
		    count(*) filter (where lower(btrim(coalesce(status, ''))) = 'pending') as pending
		 from nutrition_reward_redemptions
		 where user_id = $1 and reward_id = $2`,
		userID,
		strings.TrimSpace(rewardID),
	).Scan(&received, &pending)
	return received, pending
}

func (s *Site) loadNutritionRewardProgress(userID string, rewards []nutritionReward) map[string]nutritionRewardProgressView {
	progress := map[string]nutritionRewardProgressView{}
	for _, reward := range rewards {
		progress[reward.ID] = nutritionRewardProgressView{
			Limit:    reward.MaxPerUser,
			HasLimit: reward.HasLimit,
		}
	}

	rows, err := s.DB.Query(
		`select reward_id,
		        count(*) filter (where lower(btrim(coalesce(status, ''))) in ('approved', 'issued', 'used', 'completed')) as received,
		        count(*) filter (where lower(btrim(coalesce(status, ''))) = 'pending') as pending,
		        count(*) filter (where lower(btrim(coalesce(status, ''))) = 'rejected') as rejected
		 from nutrition_reward_redemptions
		 where user_id = $1
		 group by reward_id`,
		userID,
	)
	if err != nil {
		return progress
	}
	defer rows.Close()

	for rows.Next() {
		var rewardID string
		var received int
		var pending int
		var rejected int
		if scanErr := rows.Scan(&rewardID, &received, &pending, &rejected); scanErr != nil {
			continue
		}
		item := progress[rewardID]
		item.Received = received
		item.Pending = pending
		item.Rejected = rejected
		if item.HasLimit {
			item.Exhausted = item.Received >= item.Limit
		}
		progress[rewardID] = item
	}
	return progress
}

func (s *Site) loadNutritionRewardHistory(userID string) []nutritionRewardHistoryView {
	rows, err := s.DB.Query(
		`select rr.id,
		        rr.reward_title,
		        rr.points_cost,
		        coalesce(rr.status, 'pending'),
		        rr.requested_at,
		        rr.reviewed_at,
		        rr.used_at,
		        coalesce(m.name, ''),
		        coalesce(rr.manager_comment, '')
		 from nutrition_reward_redemptions rr
		 left join users m on m.id = rr.reviewed_by
		 where rr.user_id = $1
		 order by rr.requested_at desc`,
		userID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	history := []nutritionRewardHistoryView{}
	for rows.Next() {
		var item nutritionRewardHistoryView
		var requestedAt time.Time
		var reviewedAt sql.NullTime
		var usedAt sql.NullTime
		if scanErr := rows.Scan(
			&item.ID,
			&item.Title,
			&item.PointsCost,
			&item.Status,
			&requestedAt,
			&reviewedAt,
			&usedAt,
			&item.ReviewedBy,
			&item.ManagerComment,
		); scanErr != nil {
			continue
		}
		item.RequestedAt = requestedAt.Format("02.01.2006 15:04")
		if reviewedAt.Valid {
			item.ReviewedAt = reviewedAt.Time.Format("02.01.2006 15:04")
		}
		if usedAt.Valid {
			item.UsedAt = usedAt.Time.Format("02.01.2006 15:04")
		}
		status := strings.ToLower(strings.TrimSpace(item.Status))
		item.CanUse = status == "approved" || status == "issued"
		history = append(history, item)
	}
	return history
}

func (s *Site) loadNutritionProfileLeaderboard(userID string) nutritionProfileLeaderboardView {
	view := nutritionProfileLeaderboardView{}
	var rank int
	var points int
	err := s.DB.QueryRow(
		`with ranked as (
		   select u.id,
		          coalesce(up.points_balance, 0) as points,
		          row_number() over (
		            order by coalesce(up.points_balance, 0) desc,
		                     coalesce(nd.days_completed, 0) desc,
		                     coalesce(nm.completed_slots, 0) desc,
		                     u.name
		          ) as rank
		   from users u
		   left join user_points up on up.user_id = u.id
		   left join (
		     select user_id, count(*) filter (where day_completed = true) as days_completed
		     from nutrition_day_progress
		     group by user_id
		   ) nd on nd.user_id = u.id
		   left join (
		     select user_id, count(*) filter (where status = 'completed') as completed_slots
		     from nutrition_plan_meals
		     group by user_id
		   ) nm on nm.user_id = u.id
		   where u.role = 'employee'
		 )
		 select rank, points
		 from ranked
		 where id = $1`,
		userID,
	).Scan(&rank, &points)
	if err != nil {
		return view
	}
	view.HasData = true
	view.Rank = rank
	view.Points = points

	rows, err := s.DB.Query(
		`with ranked as (
		   select u.id,
		          u.name,
		          coalesce(up.points_balance, 0) as points,
		          row_number() over (
		            order by coalesce(up.points_balance, 0) desc,
		                     coalesce(nd.days_completed, 0) desc,
		                     coalesce(nm.completed_slots, 0) desc,
		                     u.name
		          ) as rank
		   from users u
		   left join user_points up on up.user_id = u.id
		   left join (
		     select user_id, count(*) filter (where day_completed = true) as days_completed
		     from nutrition_day_progress
		     group by user_id
		   ) nd on nd.user_id = u.id
		   left join (
		     select user_id, count(*) filter (where status = 'completed') as completed_slots
		     from nutrition_plan_meals
		     group by user_id
		   ) nm on nm.user_id = u.id
		   where u.role = 'employee'
		 )
		 select id, name, points, rank
		 from ranked
		 where rank between $1 and $2
		 order by rank`,
		max(1, rank-2),
		rank+2,
	)
	if err != nil {
		return view
	}
	defer rows.Close()

	for rows.Next() {
		var rowID string
		var row nutritionProfileLeaderboardRow
		if scanErr := rows.Scan(&rowID, &row.Name, &row.Points, &row.Rank); scanErr != nil {
			continue
		}
		row.IsCurrent = rowID == userID
		view.Rows = append(view.Rows, row)
	}
	return view
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
		{ID: "meal-snack-4", Name: "Банан + протеиновый напиток", Description: "Перекус перед активной сменой для поддержания энергии.", Category: "Перекус", Calories: 280, Protein: 22, Carbs: 31, Fats: 6},
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
