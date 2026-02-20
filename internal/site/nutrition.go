package site

import (
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"

	"rehab-app/internal/middleware"
)

type nutritionDashboardStats struct {
	DaysOnPlan      int
	HydrationDays   int
	Points          int
	ComplianceScore int
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

type nutritionTrendPoint struct {
	Label             string
	Compliance        int
	CompliancePercent int
	Hydration         int
	HydrationPercent  int
}

type nutritionPlanDay struct {
	DayLabel  string
	Breakfast string
	Lunch     string
	Dinner    string
	Calories  int
	Protein   int
	Status    string
	Hydration string
	Snack     string
	Focus     string
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

func (s *Site) moduleSelectorPage(w http.ResponseWriter, r *http.Request) {
	data := s.baseData(r, "Выбор модуля", "")
	data["HideNav"] = true
	s.render(w, "module_selector", data)
}

func (s *Site) nutritionDashboardPage(w http.ResponseWriter, r *http.Request) {
	data := s.nutritionBaseData(r, "Питание", "nutrition-dashboard")
	data["Stats"] = nutritionDashboardStats{
		DaysOnPlan:      18,
		HydrationDays:   6,
		Points:          240,
		ComplianceScore: 86,
	}
	data["NextMeal"] = nutritionMealSchedule{
		Name:        "Обед восстановления",
		Description: "Куриное филе, киноа, салат из овощей, вода 400 мл.",
		Time:        "13:00",
		Calories:    540,
		Protein:     38,
		Carbs:       52,
		Fats:        19,
	}
	data["Checklist"] = []nutritionChecklistItem{
		{Title: "Завтрак до 09:00", Completed: true},
		{Title: "Вода 1.5+ литра", Completed: true},
		{Title: "Овощи в 2 приемах пищи", Completed: true},
		{Title: "Легкий ужин до 20:00", Completed: false},
	}
	data["Trend"] = nutritionTrend()
	data["TrendBadge"] = "Последние 7 дней"
	s.render(w, "nutrition_dashboard", data)
}

func (s *Site) nutritionPlanPage(w http.ResponseWriter, r *http.Request) {
	data := s.nutritionBaseData(r, "План питания", "nutrition-plan")
	data["PlanDays"] = nutritionPlanWeek()
	data["Guidelines"] = []string{
		"Белок в каждом основном приеме пищи для поддержки восстановления мышц.",
		"Вода равномерно в течение дня, минимум 1.8 литра.",
		"Ужин без тяжелых жирных блюд для лучшего сна и восстановления.",
	}
	s.render(w, "nutrition_plan", data)
}

func (s *Site) nutritionMealsPage(w http.ResponseWriter, r *http.Request) {
	data := s.nutritionBaseData(r, "Блюда", "nutrition-meals")
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	category := strings.TrimSpace(r.URL.Query().Get("category"))
	cards := nutritionMealLibrary()

	filtered := make([]nutritionMealCard, 0, len(cards))
	q := strings.ToLower(query)
	for _, card := range cards {
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
	data["Meals"] = filtered
	s.render(w, "nutrition_meals", data)
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
	data := s.nutritionBaseData(r, "Поощрения питания", "nutrition-rewards")
	data["Rewards"] = nutritionRewardsCatalog()
	data["Points"] = 240
	data["Error"] = r.URL.Query().Get("error")
	data["Success"] = r.URL.Query().Get("success")
	s.render(w, "nutrition_rewards", data)
}

func (s *Site) nutritionRewardRedeem(w http.ResponseWriter, r *http.Request) {
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

	success := "Заявка на «" + reward.Title + "» отправлена"
	http.Redirect(w, r, "/nutrition/rewards?success="+url.QueryEscape(success), http.StatusSeeOther)
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
	data := s.nutritionBaseData(r, "Профиль питания", "nutrition-profile")
	data["Profile"] = nutritionProfileView{
		EmployeeID:      user.EmployeeID,
		Department:      user.Department,
		Position:        user.Position,
		NutritionTarget: "Поддержка восстановления и стабильная энергия",
		DailyCalories:   2100,
		WaterTarget:     "1.8 л/день",
		MealPattern:     "3 основных + 1 перекус",
		Restrictions:    []string{"Минимум жареного", "Умеренная соль", "Без поздних тяжелых ужинов"},
	}
	s.render(w, "nutrition_profile", data)
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
		{Question: "Как часто обновляется план питания?", Answer: "Базовый план формируется на неделю и уточняется по итогам ежедневных отметок."},
		{Question: "Можно ли заменить блюдо в плане?", Answer: "Да, выбирайте альтернативу из библиотеки блюд с сопоставимыми КБЖУ."},
		{Question: "Когда начисляются баллы питания?", Answer: "Баллы начисляются после закрытия дня при выполнении ключевых целей по рациону и воде."},
	}
	s.render(w, "nutrition_support", data)
}

func (s *Site) nutritionBaseData(r *http.Request, title, active string) map[string]any {
	data := s.baseData(r, title, active)
	data["Module"] = "nutrition"
	return data
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

func nutritionPlanWeek() []nutritionPlanDay {
	return []nutritionPlanDay{
		{DayLabel: "Понедельник", Breakfast: "Овсянка + ягоды", Lunch: "Курица + киноа", Dinner: "Рыба + овощи", Snack: "Йогурт и орехи", Calories: 2080, Protein: 128, Hydration: "1.9 л", Focus: "Старт недели", Status: "completed"},
		{DayLabel: "Вторник", Breakfast: "Омлет + цельнозерновой тост", Lunch: "Индейка + булгур", Dinner: "Творог + салат", Snack: "Фрукт и кефир", Calories: 2120, Protein: 132, Hydration: "2.0 л", Focus: "Стабильный белок", Status: "completed"},
		{DayLabel: "Среда", Breakfast: "Гречка + яйцо", Lunch: "Телятина + рис", Dinner: "Суп-пюре + овощи", Snack: "Протеиновый батончик", Calories: 2060, Protein: 125, Hydration: "1.8 л", Focus: "Контроль соли", Status: "in_progress"},
		{DayLabel: "Четверг", Breakfast: "Творог + банан", Lunch: "Рыба + картофель", Dinner: "Курица + салат", Snack: "Орехи", Calories: 2100, Protein: 130, Hydration: "1.9 л", Focus: "Равномерная энергия", Status: "pending"},
		{DayLabel: "Пятница", Breakfast: "Овсяноблин + творог", Lunch: "Говядина + гречка", Dinner: "Индейка + овощи", Snack: "Йогурт", Calories: 2140, Protein: 136, Hydration: "2.0 л", Focus: "Поддержка перед выходными", Status: "pending"},
	}
}

func nutritionMealLibrary() []nutritionMealCard {
	return []nutritionMealCard{
		{ID: "meal-1", Name: "Боул с курицей и киноа", Description: "Сбалансированный обед для восстановления после рабочего дня.", Category: "Обед", Calories: 540, Protein: 38, Carbs: 52, Fats: 19},
		{ID: "meal-2", Name: "Омлет с зеленью", Description: "Белковый завтрак с умеренным количеством жиров.", Category: "Завтрак", Calories: 420, Protein: 31, Carbs: 18, Fats: 24},
		{ID: "meal-3", Name: "Лосось с овощами", Description: "Легкий ужин с омега-3 и клетчаткой.", Category: "Ужин", Calories: 500, Protein: 36, Carbs: 24, Fats: 27},
		{ID: "meal-4", Name: "Творог с ягодами", Description: "Перекус для набора белка без лишних калорий.", Category: "Перекус", Calories: 260, Protein: 25, Carbs: 16, Fats: 8},
		{ID: "meal-5", Name: "Суп из чечевицы", Description: "Теплый обед с высоким содержанием растительного белка.", Category: "Обед", Calories: 390, Protein: 22, Carbs: 44, Fats: 10},
		{ID: "meal-6", Name: "Греческий салат + индейка", Description: "Быстрый ужин для легкого завершения дня.", Category: "Ужин", Calories: 430, Protein: 33, Carbs: 21, Fats: 19},
	}
}

func nutritionRewardsCatalog() []nutritionReward {
	return []nutritionReward{
		{
			ID:          "nutri-1",
			Title:       "Персональная консультация с нутрициологом 30 мин",
			Description: "Индивидуальный разбор рациона и корректировка питания под вашу динамику восстановления.",
			PointsCost:  180,
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
			PointsCost:  220,
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
