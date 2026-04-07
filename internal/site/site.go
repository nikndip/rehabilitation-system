package site

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"

	"rehab-app/internal/config"
	"rehab-app/internal/db"
	"rehab-app/internal/middleware"
	"rehab-app/internal/web"
)

type Site struct {
	DB       *sql.DB
	Renderer *web.Renderer
	Sessions *middleware.SessionManager
	Config   config.Config
}

type planChangeView struct {
	ChangedAt string
	Reason    string
}

type notificationHistoryEntry struct {
	When   time.Time
	Reason string
}

func New(dbConn *sql.DB, renderer *web.Renderer, sessions *middleware.SessionManager, cfg config.Config) *Site {
	return &Site{DB: dbConn, Renderer: renderer, Sessions: sessions, Config: cfg}
}

func (s *Site) Router() chi.Router {
	r := chi.NewRouter()
	r.Use(s.Sessions.Load)

	r.Get("/login", s.loginPage)
	r.Post("/login", s.loginSubmit)
	r.Get("/register", s.registerPage)
	r.Post("/register", s.registerSubmit)
	r.Post("/logout", s.logout)

	r.Group(func(pr chi.Router) {
		pr.Use(s.Sessions.RequireAuth)
		pr.Post("/notifications/clear", s.notificationsClear)
		pr.Get("/", s.nutritionDashboardPage)
		pr.Route("/nutrition", func(nr chi.Router) {
			nr.Get("/", s.nutritionDashboardPage)
			nr.Get("/plan", s.nutritionPlanPage)
			nr.Post("/plan/{day}/{slot}/complete", s.nutritionPlanMealComplete)
			nr.Post("/plan/{day}/{slot}/skip", s.nutritionPlanMealSkip)
			nr.Post("/plan/{day}/{slot}/smart-replace", s.nutritionPlanMealSmartReplace)
			nr.Post("/plan/hydration/{day}/{key}/complete", s.nutritionHydrationComplete)
			nr.Post("/plan/hydration/{day}/{key}/clear", s.nutritionHydrationClear)
			nr.Get("/leaderboard", s.nutritionLeaderboardPage)
			nr.Get("/rewards", s.nutritionRewardsPage)
			nr.Post("/rewards/{id}/redeem", s.nutritionRewardRedeem)
			nr.Get("/achievements", s.nutritionAchievementsPage)
			nr.Get("/meals", s.nutritionMealsPage)
			nr.Post("/meals/{id}/assign", s.nutritionMealAssign)
			nr.Get("/questionnaire", s.nutritionQuestionnairePage)
			nr.Post("/questionnaire", s.nutritionQuestionnaireSubmit)
			nr.Get("/profile", s.nutritionProfilePage)
			nr.Post("/profile/reminders", s.nutritionProfileReminderSettingsUpdate)
			nr.Post("/profile/redemptions/{id}/use", s.nutritionRewardUse)
			nr.Get("/support", s.nutritionSupportPage)
		})

		pr.Route("/admin", func(ar chi.Router) {
			ar.Use(s.requireRoles("admin"))
			ar.Get("/", func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, "/admin/nutrition", http.StatusSeeOther)
			})
			ar.Get("/nutrition", s.adminNutritionDashboard)
			ar.Get("/nutrition/achievements", s.adminNutritionAchievementsPage)
			ar.Post("/nutrition/achievements", s.adminNutritionAchievementCreate)
			ar.Post("/nutrition/achievements/{id}/update", s.adminNutritionAchievementUpdate)
			ar.Post("/nutrition/achievements/{id}/delete", s.adminNutritionAchievementDelete)
			ar.Get("/nutrition/points", s.adminNutritionPointsPage)
			ar.Post("/nutrition/points/adjust", s.adminNutritionPointsAdjust)
			ar.Get("/nutrition/employees/{id}", s.adminNutritionEmployeePage)
			ar.Post("/nutrition/employees/{id}/email", s.adminNutritionEmployeeEmailUpdate)
			ar.Post("/nutrition/employees/{id}/reminders", s.adminNutritionEmployeeReminderUpdate)
			ar.Post("/nutrition/employees/{id}/questionnaire", s.adminNutritionEmployeeQuestionnaireUpdate)
			ar.Post("/nutrition/employees/{id}/plan/assign", s.adminNutritionEmployeePlanAssign)
			ar.Post("/nutrition/redemptions/{id}/use", s.adminNutritionRedemptionUse)
		})
	})

	return r
}

func (s *Site) baseData(r *http.Request, title, active string) map[string]any {
	user := middleware.UserFromContext(r.Context())
	data := map[string]any{
		"Title":  title,
		"Active": active,
		"User":   user,
	}
	if user != nil {
		notifications := s.loadNotifications(user.ID)
		data["Notifications"] = notifications
		data["NotificationsCount"] = len(notifications)
	}
	return data
}

func (s *Site) notificationsClear(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	_ = db.EnsureUserDefaults(s.DB, user.ID)
	_, _ = s.DB.Exec(
		`update user_profiles
     set notifications_cleared_at = now(), updated_at = now()
     where user_id = $1`,
		user.ID,
	)

	redirectTo := "/"
	if ref := strings.TrimSpace(r.Referer()); ref != "" {
		if parsed, err := url.Parse(ref); err == nil && strings.HasPrefix(parsed.Path, "/") {
			redirectTo = parsed.Path
			if parsed.RawQuery != "" {
				redirectTo += "?" + parsed.RawQuery
			}
		}
	}
	http.Redirect(w, r, redirectTo, http.StatusSeeOther)
}

func (s *Site) loadNotifications(userID string) []planChangeView {
	entries := []notificationHistoryEntry{}
	clearedAt := time.Unix(0, 0).UTC()
	var userRole string
	_ = s.DB.QueryRow(`select role from users where id = $1`, userID).Scan(&userRole)
	_ = s.DB.QueryRow(
		`select coalesce(notifications_cleared_at, to_timestamp(0))
     from user_profiles
     where user_id = $1`,
		userID,
	).Scan(&clearedAt)

	entries = append(entries, s.loadNutritionNotificationEntries(userID, clearedAt, time.Now())...)
	if strings.EqualFold(strings.TrimSpace(userRole), "admin") {
		entries = append(entries, s.loadNutritionAdminQuestionnaireNotifications(clearedAt)...)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].When.After(entries[j].When)
	})

	notifications := make([]planChangeView, 0, len(entries))
	for _, entry := range entries {
		notifications = append(notifications, planChangeView{
			ChangedAt: entry.When.Format("02.01.2006 15:04"),
			Reason:    entry.Reason,
		})
	}
	return notifications
}

func (s *Site) render(w http.ResponseWriter, name string, data map[string]any) {
	if err := s.Renderer.Render(w, name, data); err != nil {
		log.Printf("render %s: %v", name, err)
		http.Error(w, "Ошибка шаблона", http.StatusInternalServerError)
	}
}

func (s *Site) requireRoles(roles ...string) func(http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, role := range roles {
		allowed[role] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := middleware.UserFromContext(r.Context())
			if user == nil || !allowed[user.Role] {
				http.Error(w, "Доступ запрещён", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (s *Site) loginPage(w http.ResponseWriter, r *http.Request) {
	data := s.baseData(r, "Вход", "")
	data["HideNav"] = true
	data["Error"] = r.URL.Query().Get("error")
	data["AllowRegister"] = s.Config.AllowSelfRegister
	s.render(w, "login", data)
}

func (s *Site) loginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/login?error=Некорректные%20данные", http.StatusSeeOther)
		return
	}

	employeeID := strings.TrimSpace(r.FormValue("employee_id"))
	password := r.FormValue("password")
	if employeeID == "" || password == "" {
		http.Redirect(w, r, "/login?error=Заполните%20все%20поля", http.StatusSeeOther)
		return
	}

	var userID string
	var hash string
	err := s.DB.QueryRow(
		`select id, password_hash from users where employee_id = $1`,
		employeeID,
	).Scan(&userID, &hash)
	if err != nil {
		http.Redirect(w, r, "/login?error=Неверные%20данные", http.StatusSeeOther)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		http.Redirect(w, r, "/login?error=Неверные%20данные", http.StatusSeeOther)
		return
	}

	_ = db.EnsureUserDefaults(s.DB, userID)

	if err := s.createSession(w, userID); err != nil {
		http.Redirect(w, r, "/login?error=Ошибка%20сессии", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Site) registerPage(w http.ResponseWriter, r *http.Request) {
	if !s.Config.AllowSelfRegister {
		http.Error(w, "Регистрация отключена", http.StatusForbidden)
		return
	}
	data := s.baseData(r, "Регистрация", "")
	data["HideNav"] = true
	data["Error"] = r.URL.Query().Get("error")
	s.render(w, "register", data)
}

func (s *Site) registerSubmit(w http.ResponseWriter, r *http.Request) {
	if !s.Config.AllowSelfRegister {
		http.Error(w, "Регистрация отключена", http.StatusForbidden)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/register?error=Некорректные%20данные", http.StatusSeeOther)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	employeeID := strings.TrimSpace(r.FormValue("employee_id"))
	department := strings.TrimSpace(r.FormValue("department"))
	position := strings.TrimSpace(r.FormValue("position"))
	password := r.FormValue("password")

	if name == "" || employeeID == "" || password == "" {
		http.Redirect(w, r, "/register?error=Заполните%20обязательные%20поля", http.StatusSeeOther)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		http.Redirect(w, r, "/register?error=Ошибка%20пароля", http.StatusSeeOther)
		return
	}

	var userID string
	err = s.DB.QueryRow(
		`insert into users (name, employee_id, password_hash, role, department, position)
     values ($1, $2, $3, 'employee', $4, $5)
     returning id`,
		name,
		employeeID,
		string(hash),
		nullIfEmpty(department),
		nullIfEmpty(position),
	).Scan(&userID)
	if err != nil {
		http.Redirect(w, r, "/register?error=ID-сотрудника%20уже%20занят", http.StatusSeeOther)
		return
	}

	_ = db.EnsureUserDefaults(s.DB, userID)
	if err := s.createSession(w, userID); err != nil {
		http.Redirect(w, r, "/register?error=Ошибка%20сессии", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Site) logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(s.Config.CookieName)
	if err == nil && cookie.Value != "" {
		_, _ = s.DB.Exec("delete from sessions where token = $1", cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     s.Config.CookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.Config.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Site) createSession(w http.ResponseWriter, userID string) error {
	token, err := randomToken(32)
	if err != nil {
		return err
	}

	expires := time.Now().Add(s.Config.SessionTTL)
	_, err = s.DB.Exec(
		`insert into sessions (user_id, token, expires_at)
     values ($1, $2, $3)`,
		userID,
		token,
		expires,
	)
	if err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     s.Config.CookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   s.Config.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})

	return nil
}

func randomToken(length int) (string, error) {
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func nullIfEmpty(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func normalizeResourceID(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.Trim(trimmed, "{}")
	trimmed = strings.ToLower(trimmed)
	return trimmed
}
