package site

import (
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"

	"rehab-app/internal/middleware"
)

type adminPasswordResetRequestRow struct {
	ID                string
	UserName          string
	EmployeeID        string
	Department        string
	Status            string
	RequestedAt       string
	ProcessedAt       string
	ProcessedBy       string
	Note              string
	TemporaryIssued   bool
	TemporaryIssuedAt string
	IsPending         bool
}

func (s *Site) forgotPasswordPage(w http.ResponseWriter, r *http.Request) {
	data := s.baseData(r, "Восстановление доступа", "")
	data["Module"] = "nutrition"
	data["HideNav"] = true
	data["Error"] = r.URL.Query().Get("error")
	data["Success"] = r.URL.Query().Get("success")
	s.render(w, "forgot_password", data)
}

func (s *Site) forgotPasswordSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/password/forgot?error=Некорректные%20данные%20формы", http.StatusSeeOther)
		return
	}

	employeeID := strings.TrimSpace(r.FormValue("employee_id"))
	if employeeID == "" {
		http.Redirect(w, r, "/password/forgot?error=Укажите%20табельный%20номер", http.StatusSeeOther)
		return
	}

	var userID string
	err := s.DB.QueryRow(
		`select id
		 from users
		 where employee_id = $1`,
		employeeID,
	).Scan(&userID)
	if err == nil {
		_, upsertErr := s.DB.Exec(
			`insert into password_reset_requests (
			    user_id, status, requested_at, processed_at, processed_by,
			    temporary_password_set, temporary_password_set_at, note
			  )
			  values ($1, 'pending', now(), null, null, false, null, '')
			  on conflict (user_id) where status = 'pending'
			  do update
			     set requested_at = excluded.requested_at,
			         processed_at = null,
			         processed_by = null,
			         temporary_password_set = false,
			         temporary_password_set_at = null,
			         note = ''`,
			userID,
		)
		if upsertErr != nil {
			http.Redirect(w, r, "/password/forgot?error=Не%20удалось%20создать%20заявку", http.StatusSeeOther)
			return
		}
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		http.Redirect(w, r, "/password/forgot?error=Не%20удалось%20создать%20заявку", http.StatusSeeOther)
		return
	}

	http.Redirect(
		w,
		r,
		"/login?success="+url.QueryEscape("Заявка на восстановление отправлена администратору. После выдачи временного пароля войдите и смените его."),
		http.StatusSeeOther,
	)
}

func (s *Site) temporaryPasswordPage(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if !user.PasswordTemp {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	data := s.baseData(r, "Смена временного пароля", "")
	data["Module"] = "nutrition"
	data["HideNav"] = true
	data["Error"] = r.URL.Query().Get("error")
	s.render(w, "change_temporary_password", data)
}

func (s *Site) temporaryPasswordSubmit(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if !user.PasswordTemp {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/password/change-temporary?error=Некорректные%20данные%20формы", http.StatusSeeOther)
		return
	}

	currentPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")
	confirmPassword := r.FormValue("confirm_password")
	if currentPassword == "" || newPassword == "" || confirmPassword == "" {
		http.Redirect(w, r, "/password/change-temporary?error=Заполните%20все%20поля", http.StatusSeeOther)
		return
	}
	if len(newPassword) < 8 {
		http.Redirect(w, r, "/password/change-temporary?error=Новый%20пароль%20должен%20содержать%20минимум%208%20символов", http.StatusSeeOther)
		return
	}
	if newPassword != confirmPassword {
		http.Redirect(w, r, "/password/change-temporary?error=Подтверждение%20пароля%20не%20совпадает", http.StatusSeeOther)
		return
	}
	if currentPassword == newPassword {
		http.Redirect(w, r, "/password/change-temporary?error=Новый%20пароль%20должен%20отличаться%20от%20временного", http.StatusSeeOther)
		return
	}

	var currentHash string
	err := s.DB.QueryRow(
		`select password_hash
		 from users
		 where id = $1`,
		user.ID,
	).Scan(&currentHash)
	if err != nil {
		http.Redirect(w, r, "/password/change-temporary?error=Не%20удалось%20проверить%20текущий%20пароль", http.StatusSeeOther)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(currentPassword)); err != nil {
		http.Redirect(w, r, "/password/change-temporary?error=Текущий%20пароль%20указан%20неверно", http.StatusSeeOther)
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		http.Redirect(w, r, "/password/change-temporary?error=Не%20удалось%20сохранить%20пароль", http.StatusSeeOther)
		return
	}

	_, err = s.DB.Exec(
		`update users
		 set password_hash = $1,
		     password_temp = false,
		     updated_at = now()
		 where id = $2`,
		string(newHash),
		user.ID,
	)
	if err != nil {
		http.Redirect(w, r, "/password/change-temporary?error=Не%20удалось%20обновить%20пароль", http.StatusSeeOther)
		return
	}

	s.insertNutritionEvent(user.ID, "Пароль обновлен после входа с временным доступом.")
	s.logNutritionAudit(
		user,
		"password_changed_after_temporary",
		"user",
		user.ID,
		user.ID,
		strings.TrimSpace(user.Department),
		map[string]any{},
	)

	http.Redirect(w, r, "/nutrition/profile?success="+url.QueryEscape("Пароль обновлен"), http.StatusSeeOther)
}

func (s *Site) adminNutritionPasswordResetIssueTemp(w http.ResponseWriter, r *http.Request) {
	admin := middleware.UserFromContext(r.Context())
	if admin == nil {
		http.Redirect(w, r, "/admin/nutrition/users?error=Сеанс%20администратора%20не%20найден", http.StatusSeeOther)
		return
	}

	requestID := normalizeResourceID(chi.URLParam(r, "id"))
	if requestID == "" {
		http.Redirect(w, r, "/admin/nutrition/users?error=Заявка%20не%20найдена", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/nutrition/users?error=Некорректные%20данные%20формы", http.StatusSeeOther)
		return
	}

	temporaryPassword := r.FormValue("temporary_password")
	note := strings.TrimSpace(r.FormValue("note"))
	if len(temporaryPassword) < 8 {
		http.Redirect(w, r, "/admin/nutrition/users?error=Временный%20пароль%20должен%20содержать%20минимум%208%20символов", http.StatusSeeOther)
		return
	}

	tx, err := s.DB.Begin()
	if err != nil {
		http.Redirect(w, r, "/admin/nutrition/users?error=Не%20удалось%20обработать%20заявку", http.StatusSeeOther)
		return
	}
	defer tx.Rollback()

	var userID string
	var employeeID string
	var employeeName string
	var department string
	err = tx.QueryRow(
		`select pr.user_id,
		        coalesce(u.employee_id, ''),
		        coalesce(u.name, ''),
		        coalesce(u.department, '')
		 from password_reset_requests pr
		 join users u on u.id = pr.user_id
		 where pr.id = $1
		   and pr.status = 'pending'
		 for update`,
		requestID,
	).Scan(&userID, &employeeID, &employeeName, &department)
	if err != nil {
		http.Redirect(w, r, "/admin/nutrition/users?error=Заявка%20не%20найдена%20или%20уже%20обработана", http.StatusSeeOther)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(temporaryPassword), bcrypt.DefaultCost)
	if err != nil {
		http.Redirect(w, r, "/admin/nutrition/users?error=Не%20удалось%20подготовить%20временный%20пароль", http.StatusSeeOther)
		return
	}

	if _, err := tx.Exec(
		`update users
		 set password_hash = $1,
		     password_temp = true,
		     updated_at = now()
		 where id = $2`,
		string(hash),
		userID,
	); err != nil {
		http.Redirect(w, r, "/admin/nutrition/users?error=Не%20удалось%20обновить%20пароль%20сотрудника", http.StatusSeeOther)
		return
	}

	if _, err := tx.Exec(`delete from sessions where user_id = $1`, userID); err != nil {
		http.Redirect(w, r, "/admin/nutrition/users?error=Не%20удалось%20обновить%20сессии%20сотрудника", http.StatusSeeOther)
		return
	}

	if _, err := tx.Exec(
		`update password_reset_requests
		 set status = 'completed',
		     processed_at = now(),
		     processed_by = $2,
		     temporary_password_set = true,
		     temporary_password_set_at = now(),
		     note = coalesce(nullif($3, ''), note)
		 where id = $1`,
		requestID,
		admin.ID,
		note,
	); err != nil {
		http.Redirect(w, r, "/admin/nutrition/users?error=Не%20удалось%20закрыть%20заявку", http.StatusSeeOther)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Redirect(w, r, "/admin/nutrition/users?error=Не%20удалось%20завершить%20операцию", http.StatusSeeOther)
		return
	}

	s.insertNutritionEvent(userID, "Администратор выдал временный пароль. Смените пароль при следующем входе.")
	s.logNutritionAudit(
		admin,
		"password_reset_temporary_issued",
		"password_reset_request",
		requestID,
		userID,
		strings.TrimSpace(department),
		map[string]any{
			"employee_name": employeeName,
			"employee_id":   employeeID,
		},
	)

	success := "Временный пароль назначен"
	if strings.TrimSpace(employeeID) != "" {
		success += " сотруднику " + strings.TrimSpace(employeeID)
	}
	http.Redirect(w, r, "/admin/nutrition/users?success="+url.QueryEscape(success), http.StatusSeeOther)
}

func (s *Site) loadAdminPasswordResetRequests(limit int) []adminPasswordResetRequestRow {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.DB.Query(
		`select pr.id,
		        pr.status,
		        pr.requested_at,
		        pr.processed_at,
		        coalesce(u.name, ''),
		        coalesce(u.employee_id, ''),
		        coalesce(u.department, ''),
		        coalesce(a.name, ''),
		        coalesce(pr.note, ''),
		        coalesce(pr.temporary_password_set, false),
		        pr.temporary_password_set_at
		 from password_reset_requests pr
		 join users u on u.id = pr.user_id
		 left join users a on a.id = pr.processed_by
		 order by case when pr.status = 'pending' then 0 else 1 end,
		          pr.requested_at desc
		 limit $1`,
		limit,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	list := []adminPasswordResetRequestRow{}
	for rows.Next() {
		var item adminPasswordResetRequestRow
		var requestedAt time.Time
		var processedAt sql.NullTime
		var temporaryIssuedAt sql.NullTime
		if scanErr := rows.Scan(
			&item.ID,
			&item.Status,
			&requestedAt,
			&processedAt,
			&item.UserName,
			&item.EmployeeID,
			&item.Department,
			&item.ProcessedBy,
			&item.Note,
			&item.TemporaryIssued,
			&temporaryIssuedAt,
		); scanErr != nil {
			continue
		}
		item.Status = strings.ToLower(strings.TrimSpace(item.Status))
		item.IsPending = item.Status == "pending"
		item.RequestedAt = requestedAt.Format("02.01.2006 15:04")
		if processedAt.Valid {
			item.ProcessedAt = processedAt.Time.Format("02.01.2006 15:04")
		}
		if temporaryIssuedAt.Valid {
			item.TemporaryIssuedAt = temporaryIssuedAt.Time.Format("02.01.2006 15:04")
		}
		list = append(list, item)
	}
	return list
}

func (s *Site) loadAdminPasswordResetNotifications(clearedAt time.Time) []notificationHistoryEntry {
	rows, err := s.DB.Query(
		`select coalesce(u.name, ''),
		        coalesce(u.employee_id, ''),
		        pr.requested_at
		 from password_reset_requests pr
		 join users u on u.id = pr.user_id
		 where pr.status = 'pending'
		   and pr.requested_at > $1
		 order by pr.requested_at desc`,
		clearedAt,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	entries := []notificationHistoryEntry{}
	for rows.Next() {
		var name string
		var employeeID string
		var requestedAt time.Time
		if scanErr := rows.Scan(&name, &employeeID, &requestedAt); scanErr != nil {
			continue
		}
		reason := "Новая заявка на восстановление пароля"
		if strings.TrimSpace(name) != "" {
			reason += ": " + strings.TrimSpace(name)
		}
		if strings.TrimSpace(employeeID) != "" {
			reason += " (табельный номер " + strings.TrimSpace(employeeID) + ")"
		}
		entries = append(entries, notificationHistoryEntry{
			When:   requestedAt,
			Reason: reason,
		})
	}
	return entries
}
