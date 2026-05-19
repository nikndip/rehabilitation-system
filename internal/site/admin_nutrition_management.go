package site

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

type adminNutritionAchievementCatalogRow struct {
	ID              string
	Code            string
	Title           string
	Description     string
	Icon            string
	PointsReward    int
	Active          bool
	SortOrder       int
	RuleID          string
	RuleCode        string
	MetricKey       string
	WindowDays      int
	TargetValue     int
	RuleDescription string
}

type adminNutritionAchievementProgressRow struct {
	UserID           string
	UserName         string
	EmployeeID       string
	AchievementTitle string
	Progress         int
	Target           int
	Unlocked         bool
	UnlockedAt       string
	UpdatedAt        string
	ProgressPercent  int
}

type adminNutritionPointsEmployeeRow struct {
	ID             string
	Name           string
	EmployeeID     string
	CorporateEmail string
	Department     string
	Position       string
	Points         int
}

type adminNutritionMetricOption struct {
	Key   string
	Label string
}

func (s *Site) adminNutritionEmployeeEmailUpdate(w http.ResponseWriter, r *http.Request) {
	employeeID := normalizeResourceID(chi.URLParam(r, "id"))
	if employeeID == "" {
		http.Redirect(w, r, "/admin/nutrition?error=Сотрудник%20не%20найден", http.StatusSeeOther)
		return
	}
	if _, ok := s.loadAdminNutritionEmployee(employeeID); !ok {
		http.Redirect(w, r, "/admin/nutrition?error=Сотрудник%20не%20найден", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/nutrition/employees/"+employeeID+"?error=Некорректные%20данные%20формы", http.StatusSeeOther)
		return
	}

	email, err := normalizeCorporateEmail(r.FormValue("corporate_email"))
	if err != nil {
		http.Redirect(w, r, "/admin/nutrition/employees/"+employeeID+"?error="+url.QueryEscape("Проверьте формат корпоративной почты"), http.StatusSeeOther)
		return
	}

	_, err = s.DB.Exec(
		`update users
		 set corporate_email = nullif($1, ''),
		     updated_at = now()
		 where id = $2 and role = 'employee'`,
		email,
		employeeID,
	)
	if err != nil {
		http.Redirect(w, r, "/admin/nutrition/employees/"+employeeID+"?error="+url.QueryEscape("Не удалось сохранить корпоративную почту"), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/admin/nutrition/employees/"+employeeID+"?success="+url.QueryEscape("Корпоративная почта обновлена"), http.StatusSeeOther)
}

func (s *Site) adminNutritionEmployeeReminderUpdate(w http.ResponseWriter, r *http.Request) {
	employeeID := normalizeResourceID(chi.URLParam(r, "id"))
	if employeeID == "" {
		http.Redirect(w, r, "/admin/nutrition?error=Сотрудник%20не%20найден", http.StatusSeeOther)
		return
	}
	if _, ok := s.loadAdminNutritionEmployee(employeeID); !ok {
		http.Redirect(w, r, "/admin/nutrition?error=Сотрудник%20не%20найден", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/nutrition/employees/"+employeeID+"?error=Некорректные%20данные%20формы", http.StatusSeeOther)
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
	if err := s.saveNutritionReminderSettings(employeeID, settings); err != nil {
		http.Redirect(w, r, "/admin/nutrition/employees/"+employeeID+"?error=Не%20удалось%20обновить%20напоминания", http.StatusSeeOther)
		return
	}

	s.insertNutritionEvent(employeeID, "Настройки напоминаний обновлены администратором.")
	http.Redirect(w, r, "/admin/nutrition/employees/"+employeeID+"?success="+url.QueryEscape("Настройки напоминаний обновлены"), http.StatusSeeOther)
}

func (s *Site) adminNutritionAchievementsPage(w http.ResponseWriter, r *http.Request) {
	s.ensureNutritionAchievementProgressForEmployees()

	data := s.nutritionBaseData(r, "Админка питания: достижения", "nutrition-admin")
	data["Success"] = r.URL.Query().Get("success")
	data["Error"] = r.URL.Query().Get("error")
	data["Catalog"] = s.loadNutritionAdminAchievementCatalog()
	progress := s.loadNutritionAchievementProgressRows(300)
	for i := range progress {
		progress[i].ProgressPercent = 0
		if progress[i].Target > 0 {
			progress[i].ProgressPercent = int(float64(progress[i].Progress) / float64(progress[i].Target) * 100)
			if progress[i].ProgressPercent > 100 {
				progress[i].ProgressPercent = 100
			}
		}
	}
	data["ProgressRows"] = progress
	data["MetricOptions"] = adminNutritionMetricOptions()
	s.render(w, "admin_nutrition_achievements", data)
}

func (s *Site) adminNutritionAchievementCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/nutrition/achievements?error=Некорректные%20данные%20формы", http.StatusSeeOther)
		return
	}

	item, err := adminNutritionAchievementFromForm(r)
	if err != nil {
		http.Redirect(w, r, "/admin/nutrition/achievements?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	if item.RuleCode == "" {
		item.RuleCode = item.Code + "-rule"
	}

	if err := s.upsertNutritionAchievement(item); err != nil {
		http.Redirect(w, r, "/admin/nutrition/achievements?error=Не%20удалось%20сохранить%20достижение", http.StatusSeeOther)
		return
	}
	s.ensureNutritionAchievementProgressForEmployees()
	http.Redirect(w, r, "/admin/nutrition/achievements?success="+url.QueryEscape("Достижение сохранено"), http.StatusSeeOther)
}

func (s *Site) adminNutritionAchievementUpdate(w http.ResponseWriter, r *http.Request) {
	achievementID := normalizeResourceID(chi.URLParam(r, "id"))
	if achievementID == "" {
		http.Redirect(w, r, "/admin/nutrition/achievements?error=Достижение%20не%20найдено", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/nutrition/achievements?error=Некорректные%20данные%20формы", http.StatusSeeOther)
		return
	}

	item, err := adminNutritionAchievementFromForm(r)
	if err != nil {
		http.Redirect(w, r, "/admin/nutrition/achievements?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}

	err = s.DB.QueryRow(
		`select c.code, r.rule_code
		 from nutrition_achievement_catalog c
		 join nutrition_achievement_rules r on r.id = c.rule_id
		 where c.id = $1`,
		achievementID,
	).Scan(&item.Code, &item.RuleCode)
	if err != nil {
		http.Redirect(w, r, "/admin/nutrition/achievements?error=Достижение%20не%20найдено", http.StatusSeeOther)
		return
	}

	if err := s.upsertNutritionAchievement(item); err != nil {
		http.Redirect(w, r, "/admin/nutrition/achievements?error=Не%20удалось%20обновить%20достижение", http.StatusSeeOther)
		return
	}
	s.ensureNutritionAchievementProgressForEmployees()
	http.Redirect(w, r, "/admin/nutrition/achievements?success="+url.QueryEscape("Достижение обновлено"), http.StatusSeeOther)
}

func (s *Site) adminNutritionAchievementDelete(w http.ResponseWriter, r *http.Request) {
	achievementID := normalizeResourceID(chi.URLParam(r, "id"))
	if achievementID == "" {
		http.Redirect(w, r, "/admin/nutrition/achievements?error=Достижение%20не%20найдено", http.StatusSeeOther)
		return
	}

	var ruleID string
	err := s.DB.QueryRow(
		`select rule_id
		 from nutrition_achievement_catalog
		 where id = $1`,
		achievementID,
	).Scan(&ruleID)
	if err != nil {
		http.Redirect(w, r, "/admin/nutrition/achievements?error=Достижение%20не%20найдено", http.StatusSeeOther)
		return
	}

	_, err = s.DB.Exec(`delete from nutrition_achievement_catalog where id = $1`, achievementID)
	if err != nil {
		http.Redirect(w, r, "/admin/nutrition/achievements?error=Не%20удалось%20удалить%20достижение", http.StatusSeeOther)
		return
	}
	_, _ = s.DB.Exec(
		`delete from nutrition_achievement_rules
		 where id = $1
		   and not exists (
		     select 1 from nutrition_achievement_catalog where rule_id = $1
		   )`,
		ruleID,
	)

	http.Redirect(w, r, "/admin/nutrition/achievements?success="+url.QueryEscape("Достижение удалено"), http.StatusSeeOther)
}

func (s *Site) adminNutritionPointsPage(w http.ResponseWriter, r *http.Request) {
	data := s.nutritionBaseData(r, "Админка питания: баллы", "nutrition-admin")
	data["Success"] = r.URL.Query().Get("success")
	data["Error"] = r.URL.Query().Get("error")
	data["Employees"] = s.loadNutritionPointsEmployees()
	data["Ledger"] = s.loadNutritionPointsLedger(200)
	s.render(w, "admin_nutrition_points", data)
}

func (s *Site) loadNutritionPointsEmployees() []adminNutritionPointsEmployeeRow {
	rows, err := s.DB.Query(
		`select u.id,
		        u.name,
		        coalesce(u.employee_id, ''),
		        coalesce(u.corporate_email, ''),
		        coalesce(u.department, ''),
		        coalesce(u.position, ''),
		        coalesce(up.points_balance, 0)
		 from users u
		 left join user_points up on up.user_id = u.id
		 where u.role = 'employee'
		 order by u.name`,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	list := []adminNutritionPointsEmployeeRow{}
	for rows.Next() {
		var row adminNutritionPointsEmployeeRow
		if err := rows.Scan(
			&row.ID,
			&row.Name,
			&row.EmployeeID,
			&row.CorporateEmail,
			&row.Department,
			&row.Position,
			&row.Points,
		); err != nil {
			continue
		}
		list = append(list, row)
	}
	return list
}

func adminNutritionAchievementFromForm(r *http.Request) (adminNutritionAchievementCatalogRow, error) {
	pointsReward, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("points_reward")))
	sortOrder, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("sort_order")))
	targetValue, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("target_value")))
	windowDays, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("window_days")))
	item := adminNutritionAchievementCatalogRow{
		Code:            strings.TrimSpace(r.FormValue("code")),
		Title:           strings.TrimSpace(r.FormValue("title")),
		Description:     strings.TrimSpace(r.FormValue("description")),
		Icon:            strings.TrimSpace(r.FormValue("icon")),
		PointsReward:    pointsReward,
		Active:          strings.TrimSpace(r.FormValue("active")) == "on",
		SortOrder:       sortOrder,
		MetricKey:       strings.TrimSpace(r.FormValue("metric_key")),
		WindowDays:      windowDays,
		TargetValue:     targetValue,
		RuleDescription: strings.TrimSpace(r.FormValue("rule_description")),
	}
	if item.Code == "" {
		return adminNutritionAchievementCatalogRow{}, errors.New("укажите код достижения")
	}
	if item.Title == "" || item.Description == "" {
		return adminNutritionAchievementCatalogRow{}, errors.New("заполните название и описание достижения")
	}
	if item.Icon == "" {
		item.Icon = "🏅"
	}
	if item.PointsReward < 0 {
		return adminNutritionAchievementCatalogRow{}, errors.New("баллы за достижение не могут быть отрицательными")
	}
	if item.SortOrder <= 0 {
		item.SortOrder = 100
	}
	if item.TargetValue <= 0 {
		return adminNutritionAchievementCatalogRow{}, errors.New("цель достижения должна быть больше нуля")
	}
	switch item.MetricKey {
	case "completed_days_total", "best_streak", "current_streak", "hydration_days_total":
	default:
		return adminNutritionAchievementCatalogRow{}, errors.New("выберите корректную метрику")
	}
	if item.WindowDays < 0 {
		return adminNutritionAchievementCatalogRow{}, errors.New("окно дней не может быть отрицательным")
	}
	return item, nil
}

func normalizeCorporateEmail(value string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(value))
	if email == "" {
		return "", nil
	}
	if strings.Count(email, "@") != 1 {
		return "", errors.New("invalid email")
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", errors.New("invalid email")
	}
	if strings.Contains(parts[1], "..") || strings.HasPrefix(parts[1], ".") || strings.HasSuffix(parts[1], ".") {
		return "", errors.New("invalid email")
	}
	return email, nil
}

func adminNutritionMetricOptions() []adminNutritionMetricOption {
	return []adminNutritionMetricOption{
		{Key: "completed_days_total", Label: "Полностью закрытые дни"},
		{Key: "best_streak", Label: "Лучшая серия"},
		{Key: "current_streak", Label: "Текущая серия"},
		{Key: "hydration_days_total", Label: "Дни с выполненным водным балансом"},
	}
}
