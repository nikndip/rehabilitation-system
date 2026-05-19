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
	r.Get("/password/forgot", s.forgotPasswordPage)
	r.Post("/password/forgot", s.forgotPasswordSubmit)
	r.Post("/logout", s.logout)

	r.Group(func(pr chi.Router) {
		pr.Use(s.Sessions.RequireAuth)
		pr.Get("/password/change-temporary", s.temporaryPasswordPage)
		pr.Post("/password/change-temporary", s.temporaryPasswordSubmit)

		pr.Group(func(ppr chi.Router) {
			ppr.Use(s.requirePermanentPassword)
			ppr.Post("/notifications/clear", s.notificationsClear)
			ppr.Get("/", s.nutritionDashboardPage)
			ppr.Route("/nutrition", func(nr chi.Router) {
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
				nr.Get("/instructions/employee", s.nutritionInstructionEmployeePage)
				nr.Get("/instructions/admin", s.nutritionInstructionAdminPage)
				nr.Get("/instructions/manager", s.nutritionInstructionManagerPage)
				nr.Post("/profile/reminders", s.nutritionProfileReminderSettingsUpdate)
				nr.Post("/profile/redemptions/{id}/use", s.nutritionRewardUse)
				nr.Get("/support", s.nutritionSupportPage)
				nr.Post("/support", s.nutritionSupportCreate)
				nr.Get("/support/{id}", s.nutritionSupportThreadPage)
				nr.Post("/support/{id}/messages", s.nutritionSupportMessageCreate)
				nr.Post("/support/{id}/close", s.nutritionSupportClose)
			})

			ppr.Route("/admin", func(ar chi.Router) {
				ar.Use(s.requireRoles("admin"))
				ar.Get("/", func(w http.ResponseWriter, r *http.Request) {
					http.Redirect(w, r, "/admin/nutrition", http.StatusSeeOther)
				})
				ar.Get("/nutrition", s.adminNutritionDashboard)
				ar.Post("/nutrition/meals", s.adminNutritionMealCreate)
				ar.Get("/nutrition/users", s.adminNutritionUsersPage)
				ar.Post("/nutrition/users", s.adminNutritionUserCreate)
				ar.Post("/nutrition/users/{id}/delete", s.adminNutritionUserDelete)
				ar.Post("/nutrition/password-resets/{id}/temporary-password", s.adminNutritionPasswordResetIssueTemp)
				ar.Get("/nutrition/audit", s.adminNutritionAuditPage)
				ar.Get("/nutrition/achievements", s.adminNutritionAchievementsPage)
				ar.Post("/nutrition/achievements", s.adminNutritionAchievementCreate)
				ar.Post("/nutrition/achievements/{id}/update", s.adminNutritionAchievementUpdate)
				ar.Post("/nutrition/achievements/{id}/delete", s.adminNutritionAchievementDelete)
				ar.Get("/nutrition/points", s.adminNutritionPointsPage)
				ar.Get("/nutrition/support", s.adminNutritionSupportPage)
				ar.Get("/nutrition/support/{id}", s.adminNutritionSupportThreadPage)
				ar.Post("/nutrition/support/{id}/messages", s.adminNutritionSupportMessageCreate)
				ar.Post("/nutrition/support/{id}/status", s.adminNutritionSupportStatusUpdate)
				ar.Get("/nutrition/employees/{id}", s.adminNutritionEmployeePage)
				ar.Post("/nutrition/employees/{id}/email", s.adminNutritionEmployeeEmailUpdate)
				ar.Post("/nutrition/employees/{id}/reminders", s.adminNutritionEmployeeReminderUpdate)
				ar.Post("/nutrition/employees/{id}/questionnaire", s.adminNutritionEmployeeQuestionnaireUpdate)
				ar.Post("/nutrition/employees/{id}/plan/assign", s.adminNutritionEmployeePlanAssign)
			})

			ppr.Route("/manager", func(mr chi.Router) {
				mr.Use(s.requireRoles("manager"))
				mr.Get("/", func(w http.ResponseWriter, r *http.Request) {
					http.Redirect(w, r, "/manager/nutrition", http.StatusSeeOther)
				})
				mr.Get("/nutrition", s.managerNutritionDashboardPage)
				mr.Get("/nutrition/points", s.managerNutritionPointsPage)
				mr.Post("/nutrition/points/award", s.managerNutritionPointsAward)
				mr.Get("/nutrition/support", s.managerNutritionSupportPage)
				mr.Get("/nutrition/support/{id}", s.managerNutritionSupportThreadPage)
				mr.Post("/nutrition/support/{id}/messages", s.managerNutritionSupportMessageCreate)
				mr.Post("/nutrition/reward-requests/{id}/approve", s.managerNutritionRewardApprove)
				mr.Post("/nutrition/reward-requests/{id}/reject", s.managerNutritionRewardReject)
			})
		})
	})

	return r
}

func (s *Site) baseData(r *http.Request, title, active string) map[string]any {
	user := middleware.UserFromContext(r.Context())
	data := map[string]any{
		"Title":     title,
		"Active":    active,
		"User":      user,
		"CSRFToken": middleware.CSRFTokenFromContext(r.Context()),
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
	now := time.Now()
	clearedAt := time.Unix(0, 0).UTC()
	var userRole string
	_ = s.DB.QueryRow(`select role from users where id = $1`, userID).Scan(&userRole)
	_ = s.DB.QueryRow(
		`select coalesce(notifications_cleared_at, to_timestamp(0))
     from user_profiles
     where user_id = $1`,
		userID,
	).Scan(&clearedAt)

	entries = append(entries, s.loadNutritionNotificationEntries(userID, clearedAt, now)...)
	if strings.EqualFold(strings.TrimSpace(userRole), "admin") {
		entries = append(entries, s.loadNutritionAdminQuestionnaireNotifications(clearedAt)...)
		entries = append(entries, s.loadNutritionAdminSupportNotifications(clearedAt)...)
		entries = append(entries, s.loadNutritionAdminRewardSLANotifications(clearedAt, now)...)
		entries = append(entries, s.loadNutritionAdminSupportSLANotifications(clearedAt, now)...)
		entries = append(entries, s.loadAdminPasswordResetNotifications(clearedAt)...)
	}
	if strings.EqualFold(strings.TrimSpace(userRole), "manager") {
		entries = append(entries, s.loadNutritionManagerRewardRequestNotifications(userID, clearedAt)...)
		entries = append(entries, s.loadNutritionManagerSupportNotifications(userID, clearedAt)...)
		entries = append(entries, s.loadNutritionManagerRewardSLANotifications(userID, clearedAt, now)...)
		entries = append(entries, s.loadNutritionManagerSupportSLANotifications(userID, clearedAt, now)...)
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
		allowed[strings.ToLower(strings.TrimSpace(role))] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := middleware.UserFromContext(r.Context())
			if user == nil || !allowed[strings.ToLower(strings.TrimSpace(user.Role))] {
				http.Error(w, "Доступ запрещён", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (s *Site) requirePermanentPassword(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		if user == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if user.PasswordTemp {
			http.Redirect(w, r, "/password/change-temporary", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Site) loginPage(w http.ResponseWriter, r *http.Request) {
	data := s.baseData(r, "Вход", "")
	data["Module"] = "nutrition"
	data["HideNav"] = true
	data["Error"] = r.URL.Query().Get("error")
	data["Success"] = r.URL.Query().Get("success")
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
	var passwordTemp bool
	err := s.DB.QueryRow(
		`select id, password_hash, coalesce(password_temp, false)
		 from users
		 where employee_id = $1`,
		employeeID,
	).Scan(&userID, &hash, &passwordTemp)
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

	if passwordTemp {
		http.Redirect(w, r, "/password/change-temporary", http.StatusSeeOther)
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
