package site

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"rehab-app/internal/middleware"
)

type nutritionQuestionnaireData struct {
	Age                int      `json:"age"`
	LactoseLevel       string   `json:"lactose_level"`
	GlutenIntolerance  bool     `json:"gluten_intolerance"`
	Allergies          []string `json:"allergies"`
	GITRestrictions    []string `json:"git_restrictions"`
	DoctorDietAssigned bool     `json:"doctor_diet_assigned"`
	DiscomfortFoods    string   `json:"discomfort_foods"`
	SymptomFrequency   string   `json:"symptom_frequency"`
	WorseTime          string   `json:"worse_time"`
	WorkSchedule       string   `json:"work_schedule"`
	MealWindows        []string `json:"meal_windows"`
	CanteenAccess      string   `json:"canteen_access"`
	RecoveryPriority   string   `json:"recovery_priority"`
	EnergyLevel        string   `json:"energy_level"`
	StressSleepNote    string   `json:"stress_sleep_note"`
	AvoidFoods         string   `json:"avoid_foods"`
	PreferredFormats   []string `json:"preferred_formats"`
	NutritionGoal      string   `json:"nutrition_goal"`
	CaloriesTarget     int      `json:"calories_target"`
	WaterTargetLiters  string   `json:"water_target_liters"`
	MealPattern        string   `json:"meal_pattern"`
}

type nutritionRestrictionSummaryView struct {
	HardBan     []string
	SoftLimit   []string
	Recommended string
	LastUpdated string
	Status      string
}

type nutritionDietRules struct {
	HardBanKeywords      []string
	HardBanLabels        []string
	SoftLimitLabels      []string
	RecommendedFormats   []string
	RequiresConsultation bool
}

type nutritionHydrationReminderOption struct {
	Key  string
	Time string
}

type nutritionHydrationReminderView struct {
	ReminderKey string
	Time        string
	Status      string
	StatusCode  string
	Hint        string
	CompletedAt string
}

type nutritionHydrationLogRecord struct {
	Status      string
	CompletedAt time.Time
	UpdatedAt   time.Time
}

func defaultNutritionQuestionnaireData() nutritionQuestionnaireData {
	return nutritionQuestionnaireData{
		LactoseLevel:      "нет",
		SymptomFrequency:  "редко",
		WorseTime:         "вечером",
		WorkSchedule:      "дневной",
		CanteenAccess:     "есть",
		RecoveryPriority:  "энергия",
		EnergyLevel:       "средняя",
		NutritionGoal:     "Поддержка энергии",
		CaloriesTarget:    2100,
		WaterTargetLiters: "1.8",
		MealPattern:       "3 основных + 1 перекус",
	}
}

func (s *Site) nutritionQuestionnairePage(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	questionnaire, updatedAt, err := s.loadNutritionQuestionnaire(user.ID)
	if err != nil {
		questionnaire = defaultNutritionQuestionnaireData()
	}

	data := s.nutritionBaseData(r, "Опросник питания", "nutrition-questionnaire")
	data["Error"] = r.URL.Query().Get("error")
	data["Success"] = r.URL.Query().Get("success")
	if err != nil {
		data["Error"] = "Не удалось загрузить анкету питания"
	}
	nutritionSetQuestionnaireTemplateData(data, questionnaire, map[string]string{}, updatedAt)
	s.render(w, "nutrition_questionnaire", data)
}

func (s *Site) nutritionQuestionnaireSubmit(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/nutrition/questionnaire?error=Некорректные%20данные%20формы", http.StatusSeeOther)
		return
	}

	questionnaire, errors := nutritionQuestionnaireFromForm(r)
	if len(errors) > 0 {
		data := s.nutritionBaseData(r, "Опросник питания", "nutrition-questionnaire")
		nutritionSetQuestionnaireTemplateData(data, questionnaire, errors, time.Now())
		s.render(w, "nutrition_questionnaire", data)
		return
	}

	if err := s.saveNutritionQuestionnaire(user.ID, questionnaire); err != nil {
		http.Redirect(w, r, "/nutrition/questionnaire?error=Не%20удалось%20сохранить%20анкету", http.StatusSeeOther)
		return
	}

	s.insertNutritionEvent(user.ID, "Опросник питания обновлен: ограничения и цели скорректированы.")
	http.Redirect(w, r, "/nutrition/profile?success="+url.QueryEscape("Анкета питания сохранена"), http.StatusSeeOther)
}

func nutritionQuestionnaireFromForm(r *http.Request) (nutritionQuestionnaireData, map[string]string) {
	calories, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("calories_target")))
	age, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("age")))
	questionnaire := nutritionQuestionnaireData{
		Age:                age,
		LactoseLevel:       normalizeNutritionSelectValue(r.FormValue("lactose_level"), nutritionLactoseOptions(), "нет"),
		GlutenIntolerance:  strings.TrimSpace(r.FormValue("gluten_intolerance")) == "on",
		Allergies:          normalizeNutritionMultiSelection(r.Form["allergies"], nutritionAllergyOptions()),
		GITRestrictions:    normalizeNutritionMultiSelection(r.Form["git_restrictions"], nutritionGastroOptions()),
		DoctorDietAssigned: strings.TrimSpace(r.FormValue("doctor_diet_assigned")) == "on",
		DiscomfortFoods:    strings.TrimSpace(r.FormValue("discomfort_foods")),
		SymptomFrequency:   normalizeNutritionSelectValue(r.FormValue("symptom_frequency"), nutritionSymptomFrequencyOptions(), "редко"),
		WorseTime:          normalizeNutritionSelectValue(r.FormValue("worse_time"), nutritionWorseTimeOptions(), "вечером"),
		WorkSchedule:       normalizeNutritionSelectValue(r.FormValue("work_schedule"), nutritionWorkScheduleOptions(), "дневной"),
		MealWindows:        normalizeNutritionMultiSelection(r.Form["meal_windows"], nutritionMealWindowOptions()),
		CanteenAccess:      normalizeNutritionSelectValue(r.FormValue("canteen_access"), nutritionCanteenAccessOptions(), "есть"),
		RecoveryPriority:   normalizeNutritionSelectValue(r.FormValue("recovery_priority"), nutritionRecoveryPriorityOptions(), "энергия"),
		EnergyLevel:        normalizeNutritionSelectValue(r.FormValue("energy_level"), nutritionEnergyLevelOptions(), "средняя"),
		StressSleepNote:    strings.TrimSpace(r.FormValue("stress_sleep_note")),
		AvoidFoods:         strings.TrimSpace(r.FormValue("avoid_foods")),
		PreferredFormats:   normalizeNutritionMultiSelection(r.Form["preferred_formats"], nutritionFormatOptions()),
		NutritionGoal:      normalizeNutritionSelectValue(r.FormValue("nutrition_goal"), nutritionGoalOptions(), "Поддержка энергии"),
		CaloriesTarget:     calories,
		WaterTargetLiters:  normalizeNutritionSelectValue(r.FormValue("water_target_liters"), nutritionWaterTargetOptions(), "1.8"),
		MealPattern:        normalizeNutritionSelectValue(r.FormValue("meal_pattern"), nutritionMealPatternOptions(), "3 основных + 1 перекус"),
	}
	if questionnaire.CaloriesTarget == 0 {
		questionnaire.CaloriesTarget = 2100
	}
	return questionnaire, validateNutritionQuestionnaire(questionnaire)
}

func nutritionSetQuestionnaireTemplateData(data map[string]any, questionnaire nutritionQuestionnaireData, errors map[string]string, updatedAt time.Time) {
	data["Questionnaire"] = questionnaire
	data["Errors"] = errors
	data["LactoseOptions"] = nutritionLactoseOptions()
	data["AllergyOptions"] = nutritionAllergyOptions()
	data["GastroOptions"] = nutritionGastroOptions()
	data["SymptomFrequencyOptions"] = nutritionSymptomFrequencyOptions()
	data["WorseTimeOptions"] = nutritionWorseTimeOptions()
	data["WorkScheduleOptions"] = nutritionWorkScheduleOptions()
	data["MealWindowOptions"] = nutritionMealWindowOptions()
	data["CanteenAccessOptions"] = nutritionCanteenAccessOptions()
	data["PriorityOptions"] = nutritionRecoveryPriorityOptions()
	data["EnergyLevelOptions"] = nutritionEnergyLevelOptions()
	data["GoalOptions"] = nutritionGoalOptions()
	data["WaterTargetOptions"] = nutritionWaterTargetOptions()
	data["MealPatternOptions"] = nutritionMealPatternOptions()
	data["FormatOptions"] = nutritionFormatOptions()
	if !updatedAt.IsZero() {
		data["UpdatedAt"] = updatedAt.Format("02.01.2006 15:04")
	} else {
		data["UpdatedAt"] = "не заполнен"
	}
}

func validateNutritionQuestionnaire(q nutritionQuestionnaireData) map[string]string {
	errors := map[string]string{}
	if q.Age < 0 || q.Age > 120 {
		errors["age"] = "Укажите корректный возраст"
	}
	if q.Age != 0 && (q.Age < 14 || q.Age > 100) {
		errors["age"] = "Возраст должен быть в диапазоне 14-100"
	}
	if strings.TrimSpace(q.NutritionGoal) == "" {
		errors["nutrition_goal"] = "Выберите цель питания"
	}
	if q.CaloriesTarget < 1200 || q.CaloriesTarget > 4500 {
		errors["calories_target"] = "Диапазон калорий 1200-4500"
	}
	if strings.TrimSpace(q.MealPattern) == "" {
		errors["meal_pattern"] = "Выберите режим приема пищи"
	}
	return errors
}

func (s *Site) loadNutritionQuestionnaire(userID string) (nutritionQuestionnaireData, time.Time, error) {
	defaultData := defaultNutritionQuestionnaireData()
	var raw []byte
	var updatedAt time.Time
	err := s.DB.QueryRow(
		`select answers, updated_at
		 from nutrition_questionnaire_responses
		 where user_id = $1`,
		userID,
	).Scan(&raw, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return defaultData, time.Time{}, nil
		}
		return defaultData, time.Time{}, err
	}
	if len(raw) == 0 {
		return defaultData, updatedAt, nil
	}
	questionnaire := defaultData
	if err := json.Unmarshal(raw, &questionnaire); err != nil {
		return defaultData, updatedAt, nil
	}
	if questionnaire.CaloriesTarget == 0 {
		questionnaire.CaloriesTarget = defaultData.CaloriesTarget
	}
	if strings.TrimSpace(questionnaire.NutritionGoal) == "" {
		questionnaire.NutritionGoal = defaultData.NutritionGoal
	}
	if strings.TrimSpace(questionnaire.WaterTargetLiters) == "" {
		questionnaire.WaterTargetLiters = defaultData.WaterTargetLiters
	}
	if strings.TrimSpace(questionnaire.MealPattern) == "" {
		questionnaire.MealPattern = defaultData.MealPattern
	}
	if questionnaire.Age < 0 || questionnaire.Age > 120 {
		questionnaire.Age = 0
	}
	return questionnaire, updatedAt, nil
}

func (s *Site) saveNutritionQuestionnaire(userID string, questionnaire nutritionQuestionnaireData) error {
	payload, err := json.Marshal(questionnaire)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(
		`insert into nutrition_questionnaire_responses (user_id, answers)
		 values ($1, $2)
		 on conflict (user_id)
		 do update set answers = excluded.answers, updated_at = now()`,
		userID,
		payload,
	)
	return err
}

func nutritionRestrictionSummary(questionnaire nutritionQuestionnaireData, updatedAt time.Time) nutritionRestrictionSummaryView {
	rules := nutritionDietRulesFromQuestionnaire(questionnaire)
	summary := nutritionRestrictionSummaryView{
		HardBan:     append([]string(nil), rules.HardBanLabels...),
		SoftLimit:   append([]string(nil), rules.SoftLimitLabels...),
		Recommended: "Поддерживать равномерный режим и щадящие способы приготовления.",
		LastUpdated: "не заполнен",
		Status:      "Консультация нутрициолога не требуется",
	}
	if len(rules.RecommendedFormats) > 0 {
		summary.Recommended = strings.Join(rules.RecommendedFormats, ", ")
	}
	if updatedAt.IsZero() {
		summary.LastUpdated = "не заполнен"
	} else {
		summary.LastUpdated = updatedAt.Format("02.01.2006 15:04")
	}
	if rules.RequiresConsultation {
		summary.Status = "Требуется консультация нутрициолога"
	}
	if len(summary.HardBan) == 0 {
		summary.HardBan = []string{"нет жестких ограничений"}
	}
	if len(summary.SoftLimit) == 0 {
		summary.SoftLimit = []string{"ограничения не указаны"}
	}
	return summary
}

func nutritionOrDefault(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func nutritionIntOrDefault(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func (s *Site) nutritionDietRulesForUser(userID string) nutritionDietRules {
	questionnaire, _, err := s.loadNutritionQuestionnaire(userID)
	if err != nil {
		return nutritionDietRules{}
	}
	return nutritionDietRulesFromQuestionnaire(questionnaire)
}

func nutritionDietRulesFromQuestionnaire(questionnaire nutritionQuestionnaireData) nutritionDietRules {
	rules := nutritionDietRules{}
	lowerLactose := strings.ToLower(strings.TrimSpace(questionnaire.LactoseLevel))
	if lowerLactose == "легкая" || lowerLactose == "выраженная" {
		rules.HardBanLabels = append(rules.HardBanLabels, "Лактоза ("+questionnaire.LactoseLevel+")")
		rules.HardBanKeywords = append(rules.HardBanKeywords, "молоч", "творог", "кефир", "йогурт", "сырник", "омлет")
	}
	if questionnaire.GlutenIntolerance {
		rules.HardBanLabels = append(rules.HardBanLabels, "Непереносимость глютена")
		rules.HardBanKeywords = append(rules.HardBanKeywords, "пшен", "булгур", "перлов", "хлеб")
	}

	for _, allergy := range questionnaire.Allergies {
		switch strings.ToLower(strings.TrimSpace(allergy)) {
		case "молоко", "лактоза":
			rules.HardBanKeywords = append(rules.HardBanKeywords, "молоч", "творог", "кефир", "йогурт")
		case "орехи":
			rules.HardBanKeywords = append(rules.HardBanKeywords, "орех")
		case "рыба":
			rules.HardBanKeywords = append(rules.HardBanKeywords, "рыб", "минтай")
		case "яйца":
			rules.HardBanKeywords = append(rules.HardBanKeywords, "яйц", "омлет")
		}
		rules.HardBanLabels = append(rules.HardBanLabels, "Аллергия: "+allergy)
	}

	for _, restriction := range questionnaire.GITRestrictions {
		rules.SoftLimitLabels = append(rules.SoftLimitLabels, "ЖКТ: "+restriction)
		switch strings.ToLower(strings.TrimSpace(restriction)) {
		case "жареное":
			rules.HardBanKeywords = append(rules.HardBanKeywords, "жарен")
		case "жирное":
			rules.HardBanKeywords = append(rules.HardBanKeywords, "жир", "орех")
		case "острое":
			rules.HardBanKeywords = append(rules.HardBanKeywords, "остр")
		}
	}

	avoidFoods := nutritionSplitKeywords(questionnaire.AvoidFoods)
	if len(avoidFoods) > 0 {
		rules.HardBanLabels = append(rules.HardBanLabels, "Индивидуальный стоп-лист")
		rules.HardBanKeywords = append(rules.HardBanKeywords, avoidFoods...)
	}
	if strings.TrimSpace(questionnaire.DiscomfortFoods) != "" {
		rules.SoftLimitLabels = append(rules.SoftLimitLabels, "Контроль триггеров дискомфорта")
	}
	if len(questionnaire.PreferredFormats) > 0 {
		rules.RecommendedFormats = append(rules.RecommendedFormats, questionnaire.PreferredFormats...)
	}
	if len(questionnaire.MealWindows) > 0 {
		rules.RecommendedFormats = append(rules.RecommendedFormats, "Пищевые окна: "+strings.Join(questionnaire.MealWindows, ", "))
	}

	if questionnaire.DoctorDietAssigned || strings.EqualFold(strings.TrimSpace(questionnaire.SymptomFrequency), "почти всегда") {
		rules.RequiresConsultation = true
	}

	rules.HardBanLabels = nutritionUniqueStrings(rules.HardBanLabels)
	rules.SoftLimitLabels = nutritionUniqueStrings(rules.SoftLimitLabels)
	rules.RecommendedFormats = nutritionUniqueStrings(rules.RecommendedFormats)
	rules.HardBanKeywords = nutritionUniqueStringsLower(rules.HardBanKeywords)
	return rules
}

func nutritionMealAllowed(meal nutritionMealCard, rules nutritionDietRules) bool {
	if len(rules.HardBanKeywords) == 0 {
		return true
	}
	blob := strings.ToLower(strings.TrimSpace(meal.Name + " " + meal.Description + " " + meal.Category))
	for _, keyword := range rules.HardBanKeywords {
		if keyword == "" {
			continue
		}
		if strings.Contains(blob, keyword) {
			return false
		}
	}
	return true
}

func nutritionSmartReplacementWithRules(current nutritionMealCard, slotKey string, rules nutritionDietRules) (*nutritionMealCard, string) {
	candidates := nutritionMealsBySlot(slotKey)
	bestIdx := -1
	bestScore := 1<<31 - 1
	for idx, candidate := range candidates {
		if candidate.ID == current.ID {
			continue
		}
		if !nutritionMealAllowed(candidate, rules) {
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
	reason := "Эквивалент по КБЖУ с учетом ограничений анкеты питания."
	return &best, reason
}

func nutritionFirstAllowedMealForSlot(slotKey string, rules nutritionDietRules) *nutritionMealCard {
	candidates := nutritionMealsBySlot(slotKey)
	for i := range candidates {
		if nutritionMealAllowed(candidates[i], rules) {
			item := candidates[i]
			return &item
		}
	}
	return nil
}

func (s *Site) nutritionSmartReplacementForUser(userID string, current nutritionMealCard, slotKey string) (*nutritionMealCard, string) {
	rules := s.nutritionDietRulesForUser(userID)
	return nutritionSmartReplacementWithRules(current, slotKey, rules)
}

func nutritionUniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, trimmed)
	}
	return result
}

func nutritionUniqueStringsLower(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.ToLower(strings.TrimSpace(value))
		if trimmed == "" {
			continue
		}
		if seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		result = append(result, trimmed)
	}
	return result
}

func nutritionSplitKeywords(value string) []string {
	replacer := strings.NewReplacer(";", ",", "\n", ",")
	normalized := replacer.Replace(value)
	parts := strings.Split(normalized, ",")
	keywords := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			keywords = append(keywords, trimmed)
		}
	}
	return nutritionUniqueStringsLower(keywords)
}

func normalizeNutritionSelectValue(value string, allowed []string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	for _, option := range allowed {
		if strings.EqualFold(option, value) {
			return option
		}
	}
	return fallback
}

func normalizeNutritionMultiSelection(values, allowed []string) []string {
	selected := map[string]bool{}
	for _, value := range values {
		selected[strings.ToLower(strings.TrimSpace(value))] = true
	}
	result := []string{}
	for _, option := range allowed {
		if selected[strings.ToLower(option)] {
			result = append(result, option)
		}
	}
	return result
}

func nutritionLactoseOptions() []string {
	return []string{"нет", "легкая", "выраженная"}
}

func nutritionAllergyOptions() []string {
	return []string{"молоко", "орехи", "рыба", "яйца", "морепродукты", "цитрусовые"}
}

func nutritionGastroOptions() []string {
	return []string{"острое", "жареное", "жирное", "кислое", "грубая клетчатка"}
}

func nutritionSymptomFrequencyOptions() []string {
	return []string{"редко", "часто", "почти всегда"}
}

func nutritionWorseTimeOptions() []string {
	return []string{"утром", "днем", "вечером", "ночью"}
}

func nutritionWorkScheduleOptions() []string {
	return []string{"дневной", "сменный", "ночной"}
}

func nutritionMealWindowOptions() []string {
	return []string{"07:00-09:00", "12:00-14:00", "16:00-17:00", "18:00-20:00", "после 20:00"}
}

func nutritionCanteenAccessOptions() []string {
	return []string{"есть", "нет", "частично"}
}

func nutritionRecoveryPriorityOptions() []string {
	return []string{"энергия", "ЖКТ-комфорт", "контроль веса", "набор белка"}
}

func nutritionEnergyLevelOptions() []string {
	return []string{"низкая", "средняя", "высокая"}
}

func nutritionGoalOptions() []string {
	return []string{"Поддержка энергии", "Восстановление ЖКТ", "Контроль веса", "Набор белка"}
}

func nutritionWaterTargetOptions() []string {
	return []string{"1.5", "1.8", "2.0", "2.2"}
}

func nutritionMealPatternOptions() []string {
	return []string{"3 основных + 1 перекус", "3 основных + 2 перекуса", "4 небольших приема"}
}

func nutritionFormatOptions() []string {
	return []string{"супы", "каши", "мягкая пища", "холодные блюда", "горячие блюда"}
}

func nutritionHydrationReminderOptions() []nutritionHydrationReminderOption {
	return []nutritionHydrationReminderOption{
		{Key: "1030", Time: "10:30"},
		{Key: "1500", Time: "15:00"},
		{Key: "1800", Time: "18:00"},
	}
}

func normalizeNutritionHydrationReminderKey(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, ":", ""))
	switch value {
	case "1030", "1500", "1800":
		return value
	default:
		return ""
	}
}

func nutritionHydrationReminderTime(reminderKey string) string {
	for _, option := range nutritionHydrationReminderOptions() {
		if option.Key == normalizeNutritionHydrationReminderKey(reminderKey) {
			return option.Time
		}
	}
	return ""
}

func normalizeNutritionHydrationStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "completed":
		return "completed"
	case "cleared":
		return "cleared"
	default:
		return "planned"
	}
}

func nutritionHydrationReminderState(dayDate time.Time, plannedTime, status string, now time.Time, slaMinutes int) (string, string) {
	status = normalizeNutritionHydrationStatus(status)
	if slaMinutes < 15 {
		slaMinutes = nutritionReminderSLAMinutes
	}
	if status == "completed" {
		return "Выполнено", "Прием воды закрыт"
	}
	if status == "cleared" {
		return "Очищено", "Напоминание очищено вручную"
	}
	today := nutritionDateOnly(now)
	if nutritionDateOnly(dayDate).Before(today) {
		return "Просрочено", "Прием воды пропущен"
	}
	if nutritionDateOnly(dayDate).After(today) {
		return "Запланировано", "Напоминание по расписанию"
	}
	due, ok := nutritionParseSlotDateTime(dayDate, plannedTime)
	if !ok {
		return "Напоминание", "Контрольная точка воды"
	}
	if now.Before(due) {
		return "Напоминание", "Плановая точка воды"
	}
	if now.Before(due.Add(time.Duration(slaMinutes) * time.Minute)) {
		return "Мягкий допуск", "Рекомендуется закрыть прием воды в течение часа"
	}
	return "Просрочено", "Требуется отметка выполнения или очистка"
}

func (s *Site) loadNutritionHydrationLogs(userID string, weekStart time.Time) map[string]map[string]nutritionHydrationLogRecord {
	logs := map[string]map[string]nutritionHydrationLogRecord{}
	weekEnd := weekStart.AddDate(0, 0, len(nutritionDayOptions()))
	rows, err := s.DB.Query(
		`select day_key, reminder_key, coalesce(status, 'planned'), completed_at, updated_at
		 from nutrition_hydration_logs
		 where user_id = $1 and day_date >= $2 and day_date < $3`,
		userID,
		weekStart,
		weekEnd,
	)
	if err != nil {
		return logs
	}
	defer rows.Close()

	for rows.Next() {
		var dayKey string
		var reminderKey string
		var status string
		var completedAt sql.NullTime
		var updatedAt time.Time
		if err := rows.Scan(&dayKey, &reminderKey, &status, &completedAt, &updatedAt); err != nil {
			continue
		}
		dayKey = normalizeNutritionDayKey(dayKey)
		reminderKey = normalizeNutritionHydrationReminderKey(reminderKey)
		if dayKey == "" || reminderKey == "" {
			continue
		}
		if _, exists := logs[dayKey]; !exists {
			logs[dayKey] = map[string]nutritionHydrationLogRecord{}
		}
		record := nutritionHydrationLogRecord{Status: normalizeNutritionHydrationStatus(status), UpdatedAt: updatedAt}
		if completedAt.Valid {
			record.CompletedAt = completedAt.Time
		}
		logs[dayKey][reminderKey] = record
	}
	return logs
}

func applyNutritionHydrationReminders(
	planDays []nutritionPlanDay,
	hydrationLogs map[string]map[string]nutritionHydrationLogRecord,
	now time.Time,
	reminderSettings nutritionReminderSettings,
) {
	options := nutritionHydrationReminderOptionsForSettings(reminderSettings)
	for i := range planDays {
		planDays[i].HydrationReminders = make([]nutritionHydrationReminderView, 0, len(options))
		for _, option := range options {
			record := nutritionHydrationLogRecord{Status: "planned"}
			if dayLogs, exists := hydrationLogs[planDays[i].DayKey]; exists {
				if item, ok := dayLogs[option.Key]; ok {
					record = item
				}
			}
			state, hint := nutritionHydrationReminderState(
				planDays[i].DayDate,
				option.Time,
				record.Status,
				now,
				reminderSettings.MealSLAMinutes,
			)
			view := nutritionHydrationReminderView{
				ReminderKey: option.Key,
				Time:        option.Time,
				Status:      state,
				StatusCode:  record.Status,
				Hint:        hint,
			}
			if !record.CompletedAt.IsZero() {
				view.CompletedAt = record.CompletedAt.Format("15:04")
			}
			planDays[i].HydrationReminders = append(planDays[i].HydrationReminders, view)
		}
	}
}

func (s *Site) nutritionHydrationComplete(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	dayKey := normalizeNutritionDayKey(chi.URLParam(r, "day"))
	reminderKey := normalizeNutritionHydrationReminderKey(chi.URLParam(r, "key"))
	if dayKey == "" || reminderKey == "" {
		http.Redirect(w, r, "/nutrition/plan?error=Некорректный%20прием%20воды", http.StatusSeeOther)
		return
	}
	dayDate, ok := nutritionDayDate(nutritionWeekStart(time.Now()), dayKey)
	if !ok {
		http.Redirect(w, r, "/nutrition/plan?error=Некорректный%20день", http.StatusSeeOther)
		return
	}

	_, err := s.DB.Exec(
		`insert into nutrition_hydration_logs (user_id, day_date, day_key, reminder_key, status, completed_at, updated_at)
		 values ($1, $2, $3, $4, 'completed', now(), now())
		 on conflict (user_id, day_date, reminder_key)
		 do update set day_key = excluded.day_key,
		               status = 'completed',
		               completed_at = now(),
		               updated_at = now()`,
		user.ID,
		dayDate,
		dayKey,
		reminderKey,
	)
	if err != nil {
		http.Redirect(w, r, "/nutrition/plan?error=Не%20удалось%20обновить%20прием%20воды", http.StatusSeeOther)
		return
	}

	timeLabel := nutritionHydrationReminderTime(reminderKey)
	s.insertNutritionEvent(user.ID, "Прием воды "+timeLabel+" выполнен.")
	s.insertNutritionDayEvent(user.ID, dayKey, "hydration_completed", reminderKey, dayDate, map[string]any{
		"reminder_time": timeLabel,
	})
	if _, progressErr := s.refreshNutritionDayProgress(user.ID, dayKey, dayDate); progressErr != nil {
		log.Printf("nutrition: refresh day progress after hydration complete failed user=%s day=%s: %v", user.ID, dayDate.Format("2006-01-02"), progressErr)
	}
	_, _ = s.refreshNutritionAchievements(user.ID)
	http.Redirect(w, r, "/nutrition/plan?success="+url.QueryEscape("Прием воды "+timeLabel+" отмечен как выполненный"), http.StatusSeeOther)
}

func (s *Site) nutritionHydrationClear(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	dayKey := normalizeNutritionDayKey(chi.URLParam(r, "day"))
	reminderKey := normalizeNutritionHydrationReminderKey(chi.URLParam(r, "key"))
	if dayKey == "" || reminderKey == "" {
		http.Redirect(w, r, "/nutrition/plan?error=Некорректный%20прием%20воды", http.StatusSeeOther)
		return
	}
	dayDate, ok := nutritionDayDate(nutritionWeekStart(time.Now()), dayKey)
	if !ok {
		http.Redirect(w, r, "/nutrition/plan?error=Некорректный%20день", http.StatusSeeOther)
		return
	}

	_, err := s.DB.Exec(
		`insert into nutrition_hydration_logs (user_id, day_date, day_key, reminder_key, status, completed_at, updated_at)
		 values ($1, $2, $3, $4, 'cleared', null, now())
		 on conflict (user_id, day_date, reminder_key)
		 do update set day_key = excluded.day_key,
		               status = 'cleared',
		               completed_at = null,
		               updated_at = now()`,
		user.ID,
		dayDate,
		dayKey,
		reminderKey,
	)
	if err != nil {
		http.Redirect(w, r, "/nutrition/plan?error=Не%20удалось%20очистить%20прием%20воды", http.StatusSeeOther)
		return
	}

	timeLabel := nutritionHydrationReminderTime(reminderKey)
	s.insertNutritionEvent(user.ID, "Прием воды "+timeLabel+" очищен.")
	s.insertNutritionDayEvent(user.ID, dayKey, "hydration_cleared", reminderKey, dayDate, map[string]any{
		"reminder_time": timeLabel,
	})
	if _, progressErr := s.refreshNutritionDayProgress(user.ID, dayKey, dayDate); progressErr != nil {
		log.Printf("nutrition: refresh day progress after hydration clear failed user=%s day=%s: %v", user.ID, dayDate.Format("2006-01-02"), progressErr)
	}
	_, _ = s.refreshNutritionAchievements(user.ID)
	http.Redirect(w, r, "/nutrition/plan?success="+url.QueryEscape("Прием воды "+timeLabel+" очищен"), http.StatusSeeOther)
}

func (s *Site) loadNutritionNotificationEntries(userID string, clearedAt, now time.Time) []notificationHistoryEntry {
	entries := []notificationHistoryEntry{}
	reminderSettings := s.loadNutritionReminderSettings(userID)
	mealLead := time.Duration(reminderSettings.MealReminderLeadMinutes) * time.Minute
	mealSLA := time.Duration(reminderSettings.MealSLAMinutes) * time.Minute
	if mealLead < 0 {
		mealLead = 20 * time.Minute
	}
	if mealSLA < 15*time.Minute {
		mealSLA = time.Duration(nutritionReminderSLAMinutes) * time.Minute
	}

	rows, err := s.DB.Query(
		`select message, created_at
		 from nutrition_events
		 where user_id = $1 and created_at > $2
		 order by created_at desc`,
		userID,
		clearedAt,
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var message string
			var createdAt time.Time
			if err := rows.Scan(&message, &createdAt); err != nil {
				continue
			}
			entries = append(entries, notificationHistoryEntry{When: createdAt, Reason: message})
		}
	}

	today := nutritionDateOnly(now)
	rows, err = s.DB.Query(
		`select meal_name, meal_slot, coalesce(status, 'planned'), coalesce(planned_time, '')
		 from nutrition_plan_meals
		 where user_id = $1 and day_date = $2`,
		userID,
		today,
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var mealName string
			var mealSlot string
			var status string
			var plannedTime string
			if err := rows.Scan(&mealName, &mealSlot, &status, &plannedTime); err != nil {
				continue
			}
			if normalizeNutritionMealStatus(status) != "planned" {
				continue
			}
			if strings.TrimSpace(plannedTime) == "" {
				plannedTime = nutritionSlotPlannedTime(mealSlot)
			}
			due, ok := nutritionParseSlotDateTime(today, plannedTime)
			if !ok || !due.After(clearedAt) {
				continue
			}
			slotLabel := nutritionSlotLabel(mealSlot)
			if now.After(due.Add(mealSLA)) {
				reason := "Просрочен прием пищи: " + slotLabel
				if strings.TrimSpace(mealName) != "" {
					reason += " («" + mealName + "»)"
				}
				entries = append(entries, notificationHistoryEntry{When: due, Reason: reason})
				continue
			}
			if now.After(due.Add(-mealLead)) {
				reason := "Напоминание: прием пищи " + slotLabel + " в " + plannedTime
				if strings.TrimSpace(mealName) != "" {
					reason += " («" + mealName + "»)"
				}
				entries = append(entries, notificationHistoryEntry{When: due, Reason: reason})
			}
		}
	}

	logs := map[string]nutritionHydrationLogRecord{}
	rows, err = s.DB.Query(
		`select reminder_key, coalesce(status, 'planned'), completed_at, updated_at
		 from nutrition_hydration_logs
		 where user_id = $1 and day_date = $2`,
		userID,
		today,
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var reminderKey string
			var status string
			var completedAt sql.NullTime
			var updatedAt time.Time
			if err := rows.Scan(&reminderKey, &status, &completedAt, &updatedAt); err != nil {
				continue
			}
			key := normalizeNutritionHydrationReminderKey(reminderKey)
			if key == "" {
				continue
			}
			record := nutritionHydrationLogRecord{Status: normalizeNutritionHydrationStatus(status), UpdatedAt: updatedAt}
			if completedAt.Valid {
				record.CompletedAt = completedAt.Time
			}
			logs[key] = record
		}
	}

	for _, option := range nutritionHydrationReminderOptionsForSettings(reminderSettings) {
		record, exists := logs[option.Key]
		if exists && (record.Status == "completed" || record.Status == "cleared") {
			continue
		}
		due, ok := nutritionParseSlotDateTime(today, option.Time)
		if !ok || !due.After(clearedAt) {
			continue
		}
		if now.After(due.Add(mealSLA)) {
			entries = append(entries, notificationHistoryEntry{When: due, Reason: "Просрочен прием воды: " + option.Time})
			continue
		}
		if now.After(due.Add(-mealLead)) {
			entries = append(entries, notificationHistoryEntry{When: due, Reason: "Напоминание по воде: прием воды " + option.Time})
		}
	}

	return entries
}

func (s *Site) loadNutritionAdminQuestionnaireNotifications(clearedAt time.Time) []notificationHistoryEntry {
	entries := []notificationHistoryEntry{}
	rows, err := s.DB.Query(
		`select u.name, coalesce(u.employee_id, ''), ne.created_at
		 from nutrition_events ne
		 join users u on u.id = ne.user_id
		 where u.role = 'employee'
		   and ne.message like 'Опросник питания обновлен:%'
		   and ne.created_at > $1
		 order by ne.created_at desc`,
		clearedAt,
	)
	if err != nil {
		return entries
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		var employeeID string
		var createdAt time.Time
		if err := rows.Scan(&name, &employeeID, &createdAt); err != nil {
			continue
		}
		reason := "Сотрудник " + name + " прошел опрос питания"
		if strings.TrimSpace(employeeID) != "" {
			reason += " (ID " + employeeID + ")"
		}
		entries = append(entries, notificationHistoryEntry{When: createdAt, Reason: reason})
	}
	return entries
}
