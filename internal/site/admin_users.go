package site

import (
	"database/sql"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"

	"rehab-app/internal/db"
	"rehab-app/internal/middleware"
)

type adminNutritionUserRow struct {
	ID             string
	Name           string
	EmployeeID     string
	Role           string
	Department     string
	Position       string
	CorporateEmail string
	CreatedAt      string
	IsEmployee     bool
}

type adminNutritionRoleOption struct {
	Key   string
	Label string
}

func (s *Site) adminNutritionUsersPage(w http.ResponseWriter, r *http.Request) {
	data := s.nutritionBaseData(r, "Админка питания: сотрудники", "nutrition-admin")
	data["Success"] = r.URL.Query().Get("success")
	data["Error"] = r.URL.Query().Get("error")
	data["RoleOptions"] = adminNutritionRoleOptions()
	data["PasswordResetRequests"] = s.loadAdminPasswordResetRequests(200)

	users := []adminNutritionUserRow{}
	rows, err := s.DB.Query(
		`select u.id,
		        u.name,
		        coalesce(u.employee_id, ''),
		        lower(coalesce(u.role, 'employee')),
		        coalesce(u.department, ''),
		        coalesce(u.position, ''),
		        coalesce(u.corporate_email, ''),
		        u.created_at
		 from users u
		 order by case lower(coalesce(u.role, 'employee'))
		            when 'admin' then 1
		            when 'manager' then 2
		            else 3
		          end,
		          u.name,
		          u.employee_id`,
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var row adminNutritionUserRow
			var createdAt sql.NullTime
			if scanErr := rows.Scan(
				&row.ID,
				&row.Name,
				&row.EmployeeID,
				&row.Role,
				&row.Department,
				&row.Position,
				&row.CorporateEmail,
				&createdAt,
			); scanErr != nil {
				continue
			}
			row.Role = strings.ToLower(strings.TrimSpace(row.Role))
			row.IsEmployee = row.Role == "employee"
			if createdAt.Valid {
				row.CreatedAt = createdAt.Time.Format("02.01.2006 15:04")
			} else {
				row.CreatedAt = "—"
			}
			users = append(users, row)
		}
	}

	stats := map[string]int{
		"Total":     len(users),
		"Employees": 0,
		"Managers":  0,
		"Admins":    0,
	}
	for _, item := range users {
		switch item.Role {
		case "admin":
			stats["Admins"]++
		case "manager":
			stats["Managers"]++
		default:
			stats["Employees"]++
		}
	}

	data["Stats"] = stats
	data["Users"] = users
	s.render(w, "admin_nutrition_users", data)
}

func (s *Site) adminNutritionUserCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/nutrition/users?error=Некорректные%20данные%20формы", http.StatusSeeOther)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	employeeID := strings.TrimSpace(r.FormValue("employee_id"))
	role, ok := normalizeAdminNutritionRole(r.FormValue("role"))
	if !ok {
		http.Redirect(w, r, "/admin/nutrition/users?error=Выберите%20корректную%20роль", http.StatusSeeOther)
		return
	}
	department := strings.TrimSpace(r.FormValue("department"))
	position := strings.TrimSpace(r.FormValue("position"))
	password := r.FormValue("password")
	if name == "" || employeeID == "" || password == "" {
		http.Redirect(w, r, "/admin/nutrition/users?error=Заполните%20обязательные%20поля", http.StatusSeeOther)
		return
	}
	if len(password) < 8 {
		http.Redirect(w, r, "/admin/nutrition/users?error=Пароль%20должен%20содержать%20минимум%208%20символов", http.StatusSeeOther)
		return
	}

	corporateEmail, err := normalizeCorporateEmail(r.FormValue("corporate_email"))
	if err != nil {
		http.Redirect(w, r, "/admin/nutrition/users?error="+url.QueryEscape("Проверьте формат корпоративной почты"), http.StatusSeeOther)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		http.Redirect(w, r, "/admin/nutrition/users?error=Не%20удалось%20создать%20пароль", http.StatusSeeOther)
		return
	}

	var userID string
	err = s.DB.QueryRow(
		`insert into users (name, employee_id, password_hash, role, department, position, corporate_email)
		 values ($1, $2, $3, $4, nullif($5, ''), nullif($6, ''), nullif($7, ''))
		 returning id`,
		name,
		employeeID,
		string(hash),
		role,
		department,
		position,
		corporateEmail,
	).Scan(&userID)
	if err != nil {
		lowerErr := strings.ToLower(err.Error())
		if strings.Contains(lowerErr, "employee_id") || strings.Contains(lowerErr, "duplicate") || strings.Contains(lowerErr, "unique") {
			http.Redirect(w, r, "/admin/nutrition/users?error=Табельный%20номер%20уже%20занят", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/admin/nutrition/users?error=Не%20удалось%20создать%20учётную%20запись", http.StatusSeeOther)
		return
	}

	_ = db.EnsureUserDefaults(s.DB, userID)
	http.Redirect(w, r, "/admin/nutrition/users?success="+url.QueryEscape("Учётная запись создана"), http.StatusSeeOther)
}

func (s *Site) adminNutritionUserDelete(w http.ResponseWriter, r *http.Request) {
	admin := middleware.UserFromContext(r.Context())
	if admin == nil {
		http.Redirect(w, r, "/admin/nutrition/users?error=Сеанс%20администратора%20не%20найден", http.StatusSeeOther)
		return
	}

	userID := normalizeResourceID(chi.URLParam(r, "id"))
	if userID == "" {
		http.Redirect(w, r, "/admin/nutrition/users?error=Пользователь%20не%20найден", http.StatusSeeOther)
		return
	}
	if normalizeResourceID(admin.ID) == userID {
		http.Redirect(w, r, "/admin/nutrition/users?error=Нельзя%20удалить%20собственную%20учётную%20запись", http.StatusSeeOther)
		return
	}

	var role string
	err := s.DB.QueryRow(
		`select lower(coalesce(role, 'employee'))
		 from users
		 where id = $1`,
		userID,
	).Scan(&role)
	if err != nil {
		http.Redirect(w, r, "/admin/nutrition/users?error=Пользователь%20не%20найден", http.StatusSeeOther)
		return
	}

	if role == "admin" {
		var adminCount int
		if countErr := s.DB.QueryRow(
			`select count(*)
			 from users
			 where lower(coalesce(role, 'employee')) = 'admin'`,
		).Scan(&adminCount); countErr != nil {
			http.Redirect(w, r, "/admin/nutrition/users?error=Не%20удалось%20проверить%20количество%20администраторов", http.StatusSeeOther)
			return
		}
		if adminCount <= 1 {
			http.Redirect(w, r, "/admin/nutrition/users?error=Нельзя%20удалить%20последнего%20администратора", http.StatusSeeOther)
			return
		}
	}

	var employeeNumber string
	err = s.DB.QueryRow(
		`delete from users
		 where id = $1
		 returning coalesce(employee_id, '')`,
		userID,
	).Scan(&employeeNumber)
	if err != nil {
		http.Redirect(w, r, "/admin/nutrition/users?error=Не%20удалось%20удалить%20учётную%20запись", http.StatusSeeOther)
		return
	}

	success := "Учётная запись удалена"
	if strings.TrimSpace(employeeNumber) != "" {
		success += ": табельный номер " + strings.TrimSpace(employeeNumber)
	}
	http.Redirect(w, r, "/admin/nutrition/users?success="+url.QueryEscape(success), http.StatusSeeOther)
}

func normalizeAdminNutritionRole(value string) (string, bool) {
	role := strings.ToLower(strings.TrimSpace(value))
	switch role {
	case "employee", "manager", "admin":
		return role, true
	default:
		return "", false
	}
}

func adminNutritionRoleOptions() []adminNutritionRoleOption {
	return []adminNutritionRoleOption{
		{Key: "employee", Label: "Сотрудник"},
		{Key: "manager", Label: "Руководитель"},
		{Key: "admin", Label: "Администратор"},
	}
}
