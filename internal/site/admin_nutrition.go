package site

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

type adminNutritionEmployeeRow struct {
	ID                   string
	Name                 string
	EmployeeID           string
	CorporateEmail       string
	Department           string
	Position             string
	Points               int
	QuestionnaireFilled  bool
	QuestionnaireUpdated string
	NutritionGoal        string
	CaloriesTarget       int
	WaterTarget          string
	RestrictionStatus    string
	ConsultationRequired bool
}

type adminNutritionRedemptionRow struct {
	ID           string
	EmployeeName string
	EmployeeID   string
	Title        string
	Status       string
	RedeemedAt   string
	UsedAt       string
	CanUse       bool
}

type adminNutritionEmployeeCard struct {
	ID             string
	Name           string
	EmployeeID     string
	CorporateEmail string
	Department     string
	Position       string
	Points         int
}

type adminNutritionSlotOption struct {
	Key   string
	Label string
}

func (s *Site) adminNutritionDashboard(w http.ResponseWriter, r *http.Request) {
	data := s.nutritionBaseData(r, "Администрирование питания", "nutrition-admin")
	data["Success"] = r.URL.Query().Get("success")
	data["Error"] = r.URL.Query().Get("error")

	var questionnaireCount int
	var plansCount int
	var issuedRewards int
	var unlockedAchievements int
	var pointsOperations int
	_ = s.DB.QueryRow(`select count(*) from nutrition_questionnaire_responses`).Scan(&questionnaireCount)
	_ = s.DB.QueryRow(`select count(distinct user_id) from nutrition_plan_meals`).Scan(&plansCount)
	_ = s.DB.QueryRow(`select count(*) from nutrition_reward_redemptions where status = 'issued'`).Scan(&issuedRewards)
	_ = s.DB.QueryRow(`select count(*) from nutrition_user_achievements where unlocked = true`).Scan(&unlockedAchievements)
	_ = s.DB.QueryRow(`select count(*) from nutrition_points_ledger where created_at >= now() - interval '30 days'`).Scan(&pointsOperations)
	data["Stats"] = map[string]int{
		"Questionnaires":      questionnaireCount,
		"Plans":               plansCount,
		"IssuedRewards":       issuedRewards,
		"UnlockedAchievements": unlockedAchievements,
		"PointsOperations":    pointsOperations,
	}

	employees := []adminNutritionEmployeeRow{}
	rows, err := s.DB.Query(
		`select u.id, u.name, coalesce(u.employee_id, ''), coalesce(u.corporate_email, ''),
		        coalesce(u.department, ''), coalesce(u.position, ''), coalesce(up.points_balance, 0),
		        nqr.answers, nqr.updated_at
		 from users u
		 left join user_points up on up.user_id = u.id
		 left join nutrition_questionnaire_responses nqr on nqr.user_id = u.id
		 where u.role = 'employee'
		 order by coalesce(nqr.updated_at, to_timestamp(0)) desc, u.name`,
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var row adminNutritionEmployeeRow
			var answers []byte
			var updatedAt sql.NullTime
			if err := rows.Scan(
				&row.ID,
				&row.Name,
				&row.EmployeeID,
				&row.CorporateEmail,
				&row.Department,
				&row.Position,
				&row.Points,
				&answers,
				&updatedAt,
			); err != nil {
				continue
			}

			questionnaire := defaultNutritionQuestionnaireData()
			if len(answers) > 0 {
				_ = json.Unmarshal(answers, &questionnaire)
				row.QuestionnaireFilled = true
			}
			if updatedAt.Valid {
				row.QuestionnaireUpdated = updatedAt.Time.Format("02.01.2006 15:04")
			}
			if row.QuestionnaireFilled {
				row.NutritionGoal = nutritionOrDefault(questionnaire.NutritionGoal, "не заполнено")
				row.CaloriesTarget = nutritionIntOrDefault(questionnaire.CaloriesTarget, 0)
				if strings.TrimSpace(questionnaire.WaterTargetLiters) != "" {
					row.WaterTarget = questionnaire.WaterTargetLiters + " л"
				}
				summary := nutritionRestrictionSummary(questionnaire, time.Time{})
				row.RestrictionStatus = summary.Status
				row.ConsultationRequired = strings.Contains(strings.ToLower(summary.Status), "требуется")
			} else {
				row.NutritionGoal = "не заполнено"
				row.RestrictionStatus = "Анкета не заполнена"
			}

			employees = append(employees, row)
		}
	}
	data["Employees"] = employees

	issued := []adminNutritionRedemptionRow{}
	rows, err = s.DB.Query(
		`select nrr.id, u.name, coalesce(u.employee_id, ''), nrr.reward_title, nrr.status, nrr.redeemed_at, nrr.used_at
		 from nutrition_reward_redemptions nrr
		 join users u on u.id = nrr.user_id
		 order by nrr.redeemed_at desc
		 limit 30`,
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var item adminNutritionRedemptionRow
			var redeemedAt time.Time
			var usedAt sql.NullTime
			if err := rows.Scan(&item.ID, &item.EmployeeName, &item.EmployeeID, &item.Title, &item.Status, &redeemedAt, &usedAt); err != nil {
				continue
			}
			item.RedeemedAt = redeemedAt.Format("02.01.2006 15:04")
			if usedAt.Valid {
				item.UsedAt = usedAt.Time.Format("02.01.2006 15:04")
			}
			item.CanUse = strings.EqualFold(strings.TrimSpace(item.Status), "issued")
			issued = append(issued, item)
		}
	}
	data["IssuedRewards"] = issued

	s.render(w, "admin_nutrition", data)
}

func (s *Site) adminNutritionEmployeePage(w http.ResponseWriter, r *http.Request) {
	employeeID := normalizeResourceID(chi.URLParam(r, "id"))
	if employeeID == "" {
		http.Redirect(w, r, "/admin/nutrition?error=Сотрудник%20не%20найден", http.StatusSeeOther)
		return
	}

	employee, ok := s.loadAdminNutritionEmployee(employeeID)
	if !ok {
		http.Redirect(w, r, "/admin/nutrition?error=Сотрудник%20не%20найден", http.StatusSeeOther)
		return
	}

	questionnaire, updatedAt, _ := s.loadNutritionQuestionnaire(employee.ID)
	summary := nutritionRestrictionSummary(questionnaire, updatedAt)
	planDays := s.buildNutritionPlan(employee.ID, time.Now())
	rewardHistory := s.loadNutritionRewardHistory(employee.ID)
	mealOptions := append([]nutritionMealCard(nil), nutritionMealLibrary()...)
	sort.SliceStable(mealOptions, func(i, j int) bool {
		if mealOptions[i].Category == mealOptions[j].Category {
			return mealOptions[i].Name < mealOptions[j].Name
		}
		return mealOptions[i].Category < mealOptions[j].Category
	})

	data := s.nutritionBaseData(r, "Сотрудник: питание", "nutrition-admin")
	data["Success"] = r.URL.Query().Get("success")
	data["Error"] = r.URL.Query().Get("error")
	data["Employee"] = employee
	data["Questionnaire"] = questionnaire
	data["RestrictionSummary"] = summary
	data["QuestionnaireUpdatedAt"] = summary.LastUpdated
	data["DayOptions"] = nutritionDayOptions()
	data["SlotOptions"] = adminNutritionSlotOptions()
	data["MealOptions"] = mealOptions
	data["PlanDays"] = planDays
	data["RewardHistory"] = rewardHistory
	data["ReminderSettings"] = s.loadNutritionReminderSettings(employee.ID)
	data["AchievementProgress"] = s.loadNutritionAchievementsView(employee.ID)
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
	data["TargetURLPrefix"] = "/admin/nutrition/employees/" + employee.ID

	s.render(w, "admin_nutrition_employee", data)
}

func (s *Site) adminNutritionEmployeeQuestionnaireUpdate(w http.ResponseWriter, r *http.Request) {
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

	questionnaire, errors := nutritionQuestionnaireFromForm(r)
	if len(errors) > 0 {
		http.Redirect(w, r, "/admin/nutrition/employees/"+employeeID+"?error="+url.QueryEscape("Проверьте заполнение анкеты питания"), http.StatusSeeOther)
		return
	}

	if err := s.saveNutritionQuestionnaire(employeeID, questionnaire); err != nil {
		http.Redirect(w, r, "/admin/nutrition/employees/"+employeeID+"?error=Не%20удалось%20сохранить%20анкету", http.StatusSeeOther)
		return
	}

	s.insertNutritionEvent(employeeID, "Анкета питания обновлена администратором.")
	http.Redirect(w, r, "/admin/nutrition/employees/"+employeeID+"?success="+url.QueryEscape("Анкета сотрудника обновлена"), http.StatusSeeOther)
}

func (s *Site) adminNutritionEmployeePlanAssign(w http.ResponseWriter, r *http.Request) {
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

	mealID := strings.TrimSpace(r.FormValue("meal_id"))
	meal, ok := nutritionMealByID(mealID)
	if !ok {
		http.Redirect(w, r, "/admin/nutrition/employees/"+employeeID+"?error=Блюдо%20не%20найдено", http.StatusSeeOther)
		return
	}

	dayKey := normalizeNutritionDayKey(r.FormValue("day"))
	slot := normalizeNutritionSlotKey(r.FormValue("slot"))
	if dayKey == "" || slot == "" {
		http.Redirect(w, r, "/admin/nutrition/employees/"+employeeID+"?error=Выберите%20день%20и%20прием%20пищи", http.StatusSeeOther)
		return
	}

	if err := s.saveNutritionMealAssignment(employeeID, dayKey, slot, meal, nutritionSlotPlannedTime(slot), ""); err != nil {
		http.Redirect(w, r, "/admin/nutrition/employees/"+employeeID+"?error=Не%20удалось%20обновить%20план", http.StatusSeeOther)
		return
	}

	s.insertNutritionEvent(employeeID, "План питания скорректирован администратором: "+nutritionDayLabel(dayKey)+" / "+nutritionSlotLabel(slot)+".")
	if dayDate, ok := nutritionDayDate(nutritionWeekStart(time.Now()), dayKey); ok {
		s.insertNutritionDayEvent(employeeID, dayKey, "admin_meal_assigned", slot, dayDate, map[string]any{
			"meal_id":   meal.ID,
			"meal_name": meal.Name,
		})
	}
	http.Redirect(w, r, "/admin/nutrition/employees/"+employeeID+"?success="+url.QueryEscape("План питания обновлен"), http.StatusSeeOther)
}

func (s *Site) adminNutritionRedemptionUse(w http.ResponseWriter, r *http.Request) {
	redemptionID := normalizeResourceID(chi.URLParam(r, "id"))
	if redemptionID == "" {
		http.Redirect(w, r, "/admin/nutrition?error=Поощрение%20не%20найдено", http.StatusSeeOther)
		return
	}

	var userID string
	var title string
	err := s.DB.QueryRow(
		`update nutrition_reward_redemptions
		 set status = 'used', used_at = now()
		 where id = $1 and status = 'issued'
		 returning user_id, reward_title`,
		redemptionID,
	).Scan(&userID, &title)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Redirect(w, r, "/admin/nutrition?error=Поощрение%20уже%20использовано%20или%20недоступно", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/admin/nutrition?error=Не%20удалось%20обновить%20статус", http.StatusSeeOther)
		return
	}

	s.insertNutritionEvent(userID, "Поощрение «"+title+"» отмечено как использованное администратором.")
	http.Redirect(w, r, "/admin/nutrition?success="+url.QueryEscape("Статус поощрения обновлен"), http.StatusSeeOther)
}

func (s *Site) loadAdminNutritionEmployee(userID string) (adminNutritionEmployeeCard, bool) {
	var employee adminNutritionEmployeeCard
	err := s.DB.QueryRow(
		`select u.id,
		        u.name,
		        coalesce(u.employee_id, ''),
		        coalesce(u.corporate_email, ''),
		        coalesce(u.department, ''),
		        coalesce(u.position, ''),
		        coalesce(up.points_balance, 0)
		 from users u
		 left join user_points up on up.user_id = u.id
		 where id = $1 and role = 'employee'`,
		userID,
	).Scan(
		&employee.ID,
		&employee.Name,
		&employee.EmployeeID,
		&employee.CorporateEmail,
		&employee.Department,
		&employee.Position,
		&employee.Points,
	)
	if err != nil {
		return adminNutritionEmployeeCard{}, false
	}
	return employee, true
}

func adminNutritionSlotOptions() []adminNutritionSlotOption {
	return []adminNutritionSlotOption{
		{Key: "breakfast", Label: "Завтрак"},
		{Key: "lunch", Label: "Обед"},
		{Key: "dinner", Label: "Ужин"},
		{Key: "snack", Label: "Перекус"},
	}
}
