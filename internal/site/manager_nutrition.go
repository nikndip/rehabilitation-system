package site

import (
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"rehab-app/internal/middleware"
)

type managerNutritionStats struct {
	Department           string
	Employees            int
	TotalPoints          int
	PendingRewardRequest int
	ApprovedRewards      int
	UnlockedAchievements int
}

type managerDepartmentEmployeeRow struct {
	ID             string
	Name           string
	EmployeeID     string
	Department     string
	Position       string
	CorporateEmail string
	Points         int
}

type managerRewardRequestRow struct {
	ID             string
	UserID         string
	EmployeeName   string
	EmployeeID     string
	Department     string
	RewardTitle    string
	PointsCost     int
	Status         string
	RequestedAt    string
	SLADueAt       string
	SLAOverdue     bool
	ReviewedAt     string
	ReviewedBy     string
	ManagerComment string
	CanApprove     bool
	CanReject      bool
}

type managerAchievementRow struct {
	EmployeeName string
	EmployeeID   string
	Title        string
	UnlockedAt   string
}

func (s *Site) managerNutritionDashboardPage(w http.ResponseWriter, r *http.Request) {
	manager := middleware.UserFromContext(r.Context())
	department := strings.TrimSpace(manager.Department)
	if department == "" {
		http.Error(w, "Для роли руководителя не задано подразделение", http.StatusForbidden)
		return
	}

	data := s.nutritionBaseData(r, "Руководитель: модуль питания", "nutrition-manager")
	data["Error"] = r.URL.Query().Get("error")
	data["Success"] = r.URL.Query().Get("success")
	data["Stats"] = s.loadManagerNutritionStats(department)
	data["PendingRequests"] = s.loadManagerRewardRequests(department, true, 100)
	data["RewardHistory"] = s.loadManagerRewardRequests(department, false, 200)
	data["Achievements"] = s.loadManagerDepartmentAchievements(department, 120)
	data["Employees"] = s.loadManagerDepartmentEmployees(department)
	data["SupportTickets"] = s.loadSupportTicketsForManager(department, 30)
	s.render(w, "manager_nutrition", data)
}

func (s *Site) managerNutritionPointsPage(w http.ResponseWriter, r *http.Request) {
	manager := middleware.UserFromContext(r.Context())
	department := strings.TrimSpace(manager.Department)
	if department == "" {
		http.Error(w, "Для роли руководителя не задано подразделение", http.StatusForbidden)
		return
	}

	data := s.nutritionBaseData(r, "Руководитель: баллы отдела", "nutrition-manager")
	data["Error"] = r.URL.Query().Get("error")
	data["Success"] = r.URL.Query().Get("success")
	data["Department"] = department
	data["Employees"] = s.loadManagerDepartmentEmployees(department)
	data["Ledger"] = s.loadManagerDepartmentLedger(department, 200)
	s.render(w, "manager_nutrition_points", data)
}

func (s *Site) managerNutritionPointsAward(w http.ResponseWriter, r *http.Request) {
	manager := middleware.UserFromContext(r.Context())
	department := strings.TrimSpace(manager.Department)
	if department == "" {
		http.Error(w, "Для роли руководителя не задано подразделение", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/manager/nutrition/points?error=Некорректные%20данные%20формы", http.StatusSeeOther)
		return
	}

	targetUserID := normalizeResourceID(r.FormValue("user_id"))
	if targetUserID == "" {
		http.Redirect(w, r, "/manager/nutrition/points?error=Выберите%20сотрудника", http.StatusSeeOther)
		return
	}
	if !s.managerCanAccessEmployee(manager.ID, targetUserID) {
		http.Redirect(w, r, "/manager/nutrition/points?error=Сотрудник%20не%20относится%20к%20вашему%20отделу", http.StatusSeeOther)
		return
	}

	delta, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("delta")))
	if delta <= 0 {
		http.Redirect(w, r, "/manager/nutrition/points?error=Укажите%20положительное%20количество%20баллов", http.StatusSeeOther)
		return
	}
	if delta > 10000 {
		http.Redirect(w, r, "/manager/nutrition/points?error=Слишком%20большое%20значение%20баллов", http.StatusSeeOther)
		return
	}

	reason := strings.TrimSpace(r.FormValue("reason"))
	if reason == "" {
		reason = "Дополнительные баллы от руководителя отдела"
	}

	_, err := s.applyNutritionPointsChange(
		targetUserID,
		delta,
		"manager_adjustment",
		reason,
		"manager_nutrition",
		"",
		manager.ID,
	)
	if err != nil {
		http.Redirect(w, r, "/manager/nutrition/points?error=Не%20удалось%20начислить%20баллы", http.StatusSeeOther)
		return
	}

	s.insertNutritionEvent(targetUserID, "Руководитель начислил дополнительные баллы: +"+strconv.Itoa(delta)+".")
	s.logNutritionAudit(
		manager,
		"manager_points_awarded",
		"points_ledger",
		"",
		targetUserID,
		department,
		map[string]any{
			"delta":       delta,
			"reason":      reason,
			"source_type": "manager_nutrition",
		},
	)
	http.Redirect(w, r, "/manager/nutrition/points?success="+url.QueryEscape("Баллы сотруднику начислены"), http.StatusSeeOther)
}

func (s *Site) managerNutritionRewardApprove(w http.ResponseWriter, r *http.Request) {
	manager := middleware.UserFromContext(r.Context())
	requestID := normalizeResourceID(chi.URLParam(r, "id"))
	if requestID == "" {
		http.Redirect(w, r, "/manager/nutrition?error=Заявка%20не%20найдена", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/manager/nutrition?error=Некорректные%20данные%20заявки", http.StatusSeeOther)
		return
	}
	comment := strings.TrimSpace(r.FormValue("comment"))

	tx, err := s.DB.Begin()
	if err != nil {
		http.Redirect(w, r, "/manager/nutrition?error=Не%20удалось%20обработать%20заявку", http.StatusSeeOther)
		return
	}
	defer tx.Rollback()

	var userID string
	var rewardID string
	var rewardTitle string
	var pointsCost int
	var status string
	var employeeDepartment string
	err = tx.QueryRow(
		`select rr.user_id,
		        rr.reward_id,
		        rr.reward_title,
		        rr.points_cost,
		        rr.status,
		        coalesce(u.department, '')
		 from nutrition_reward_redemptions rr
		 join users u on u.id = rr.user_id
		 where rr.id = $1
		 for update`,
		requestID,
	).Scan(&userID, &rewardID, &rewardTitle, &pointsCost, &status, &employeeDepartment)
	if err != nil {
		http.Redirect(w, r, "/manager/nutrition?error=Заявка%20не%20найдена", http.StatusSeeOther)
		return
	}
	if !strings.EqualFold(strings.TrimSpace(employeeDepartment), strings.TrimSpace(manager.Department)) {
		http.Redirect(w, r, "/manager/nutrition?error=Доступ%20к%20заявке%20запрещен", http.StatusSeeOther)
		return
	}
	if !strings.EqualFold(strings.TrimSpace(status), "pending") {
		http.Redirect(w, r, "/manager/nutrition?error=Заявка%20уже%20обработана", http.StatusSeeOther)
		return
	}

	limit, hasLimit := s.loadNutritionRewardLimit(rewardID)
	if hasLimit {
		var received int
		_ = tx.QueryRow(
			`select count(*)
			 from nutrition_reward_redemptions
			 where user_id = $1
			   and reward_id = $2
			   and id <> $3
			   and lower(status) in ('approved', 'issued', 'used', 'completed')`,
			userID,
			rewardID,
			requestID,
		).Scan(&received)
		if received >= limit {
			http.Redirect(w, r, "/manager/nutrition?error=Лимит%20поощрения%20у%20сотрудника%20исчерпан", http.StatusSeeOther)
			return
		}
	}

	_, err = s.applyNutritionPointsChangeTx(
		tx,
		userID,
		-pointsCost,
		"reward_request_approved",
		"Списание по одобренной заявке на поощрение «"+rewardTitle+"»",
		"nutrition_reward_request",
		requestID,
		manager.ID,
	)
	if err != nil {
		if errors.Is(err, errNutritionInsufficientPoints) {
			http.Redirect(w, r, "/manager/nutrition?error=У%20сотрудника%20недостаточно%20баллов%20для%20выдачи", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/manager/nutrition?error=Не%20удалось%20обновить%20баланс%20баллов", http.StatusSeeOther)
		return
	}

	_, err = tx.Exec(
		`update nutrition_reward_redemptions
		 set status = 'approved',
		     reviewed_at = now(),
		     reviewed_by = $2,
		     manager_comment = $3
		 where id = $1`,
		requestID,
		manager.ID,
		comment,
	)
	if err != nil {
		http.Redirect(w, r, "/manager/nutrition?error=Не%20удалось%20обновить%20статус%20заявки", http.StatusSeeOther)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Redirect(w, r, "/manager/nutrition?error=Не%20удалось%20завершить%20операцию", http.StatusSeeOther)
		return
	}

	s.insertNutritionEvent(userID, "Заявка на поощрение «"+rewardTitle+"» одобрена руководителем.")
	s.logNutritionAudit(
		manager,
		"reward_request_approved",
		"reward_request",
		requestID,
		userID,
		strings.TrimSpace(manager.Department),
		map[string]any{
			"reward_id":    rewardID,
			"reward_title": rewardTitle,
			"points_cost":  pointsCost,
			"comment":      comment,
		},
	)
	http.Redirect(w, r, "/manager/nutrition?success="+url.QueryEscape("Заявка одобрена"), http.StatusSeeOther)
}

func (s *Site) managerNutritionRewardReject(w http.ResponseWriter, r *http.Request) {
	manager := middleware.UserFromContext(r.Context())
	requestID := normalizeResourceID(chi.URLParam(r, "id"))
	if requestID == "" {
		http.Redirect(w, r, "/manager/nutrition?error=Заявка%20не%20найдена", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/manager/nutrition?error=Некорректные%20данные%20заявки", http.StatusSeeOther)
		return
	}
	comment := strings.TrimSpace(r.FormValue("comment"))

	var userID string
	var rewardTitle string
	err := s.DB.QueryRow(
		`update nutrition_reward_redemptions rr
		 set status = 'rejected',
		     reviewed_at = now(),
		     reviewed_by = $2,
		     manager_comment = $3
		 from users u
		 where rr.id = $1
		   and rr.user_id = u.id
		   and lower(btrim(coalesce(rr.status, ''))) = 'pending'
		   and lower(btrim(coalesce(u.department, ''))) = lower(btrim($4))
		 returning rr.user_id, rr.reward_title`,
		requestID,
		manager.ID,
		comment,
		strings.TrimSpace(manager.Department),
	).Scan(&userID, &rewardTitle)
	if err != nil {
		http.Redirect(w, r, "/manager/nutrition?error=Заявка%20не%20найдена%20или%20уже%20обработана", http.StatusSeeOther)
		return
	}

	s.insertNutritionEvent(userID, "Заявка на поощрение «"+rewardTitle+"» отклонена руководителем.")
	s.logNutritionAudit(
		manager,
		"reward_request_rejected",
		"reward_request",
		requestID,
		userID,
		strings.TrimSpace(manager.Department),
		map[string]any{
			"reward_title": rewardTitle,
			"comment":      comment,
		},
	)
	http.Redirect(w, r, "/manager/nutrition?success="+url.QueryEscape("Заявка отклонена"), http.StatusSeeOther)
}

func (s *Site) loadManagerNutritionStats(department string) managerNutritionStats {
	stats := managerNutritionStats{Department: department}
	_ = s.DB.QueryRow(
		`select count(*)
		 from users
		 where role = 'employee'
		   and lower(btrim(coalesce(department, ''))) = lower(btrim($1))`,
		department,
	).Scan(&stats.Employees)
	_ = s.DB.QueryRow(
		`select coalesce(sum(up.points_balance), 0)
		 from users u
		 left join user_points up on up.user_id = u.id
		 where u.role = 'employee'
		   and lower(btrim(coalesce(u.department, ''))) = lower(btrim($1))`,
		department,
	).Scan(&stats.TotalPoints)
	_ = s.DB.QueryRow(
		`select count(*)
		 from nutrition_reward_redemptions rr
		 join users u on u.id = rr.user_id
		 where u.role = 'employee'
		   and lower(btrim(coalesce(u.department, ''))) = lower(btrim($1))
		   and lower(btrim(coalesce(rr.status, ''))) = 'pending'`,
		department,
	).Scan(&stats.PendingRewardRequest)
	_ = s.DB.QueryRow(
		`select count(*)
		 from nutrition_reward_redemptions rr
		 join users u on u.id = rr.user_id
		 where u.role = 'employee'
		   and lower(btrim(coalesce(u.department, ''))) = lower(btrim($1))
		   and lower(btrim(coalesce(rr.status, ''))) in ('approved', 'issued', 'used', 'completed')`,
		department,
	).Scan(&stats.ApprovedRewards)
	_ = s.DB.QueryRow(
		`select count(*)
		 from nutrition_user_achievements ua
		 join users u on u.id = ua.user_id
		 where u.role = 'employee'
		   and lower(btrim(coalesce(u.department, ''))) = lower(btrim($1))
		   and ua.unlocked = true`,
		department,
	).Scan(&stats.UnlockedAchievements)
	return stats
}

func (s *Site) loadManagerDepartmentEmployees(department string) []managerDepartmentEmployeeRow {
	rows, err := s.DB.Query(
		`select u.id,
		        u.name,
		        coalesce(u.employee_id, ''),
		        coalesce(u.department, ''),
		        coalesce(u.position, ''),
		        coalesce(u.corporate_email, ''),
		        coalesce(up.points_balance, 0)
		 from users u
		 left join user_points up on up.user_id = u.id
		 where u.role = 'employee'
		   and lower(btrim(coalesce(u.department, ''))) = lower(btrim($1))
		 order by u.name`,
		department,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	list := []managerDepartmentEmployeeRow{}
	for rows.Next() {
		var row managerDepartmentEmployeeRow
		if scanErr := rows.Scan(
			&row.ID,
			&row.Name,
			&row.EmployeeID,
			&row.Department,
			&row.Position,
			&row.CorporateEmail,
			&row.Points,
		); scanErr != nil {
			continue
		}
		list = append(list, row)
	}
	return list
}

func (s *Site) loadManagerRewardRequests(department string, onlyPending bool, limit int) []managerRewardRequestRow {
	if limit <= 0 {
		limit = 100
	}
	query := `select rr.id,
	                 rr.user_id,
	                 u.name,
	                 coalesce(u.employee_id, ''),
	                 coalesce(u.department, ''),
	                 rr.reward_title,
	                 rr.points_cost,
	                 coalesce(rr.status, 'pending'),
	                 coalesce(rr.requested_at, rr.redeemed_at, now()),
	                 rr.reviewed_at,
	                 coalesce(m.name, ''),
	                 coalesce(rr.manager_comment, '')
	          from nutrition_reward_redemptions rr
	          join users u on u.id = rr.user_id
	          left join users m on m.id = rr.reviewed_by
	          where u.role = 'employee'
	            and lower(btrim(coalesce(u.department, ''))) = lower(btrim($1))`
	if onlyPending {
		query += " and lower(btrim(coalesce(rr.status, ''))) = 'pending'"
	}
	query += ` order by case when lower(btrim(coalesce(rr.status, ''))) = 'pending' then 0 else 1 end,
	                  coalesce(rr.requested_at, rr.redeemed_at, now()) desc
	           limit $2`

	rows, err := s.DB.Query(query, department, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()

	items := []managerRewardRequestRow{}
	now := time.Now()
	for rows.Next() {
		var item managerRewardRequestRow
		var requestedAt time.Time
		var reviewedAt sql.NullTime
		if scanErr := rows.Scan(
			&item.ID,
			&item.UserID,
			&item.EmployeeName,
			&item.EmployeeID,
			&item.Department,
			&item.RewardTitle,
			&item.PointsCost,
			&item.Status,
			&requestedAt,
			&reviewedAt,
			&item.ReviewedBy,
			&item.ManagerComment,
		); scanErr != nil {
			continue
		}
		item.RequestedAt = requestedAt.Format("02.01.2006 15:04")
		dueAt := nutritionRewardSLADueAt(requestedAt)
		item.SLADueAt = dueAt.Format("02.01.2006 15:04")
		if reviewedAt.Valid {
			item.ReviewedAt = reviewedAt.Time.Format("02.01.2006 15:04")
		}
		status := strings.ToLower(strings.TrimSpace(item.Status))
		item.CanApprove = status == "pending"
		item.CanReject = status == "pending"
		item.SLAOverdue = status == "pending" && now.After(dueAt)
		items = append(items, item)
	}
	return items
}

func (s *Site) loadManagerDepartmentAchievements(department string, limit int) []managerAchievementRow {
	if limit <= 0 {
		limit = 80
	}
	rows, err := s.DB.Query(
		`select u.name,
		        coalesce(u.employee_id, ''),
		        c.title,
		        ua.unlocked_at
		 from nutrition_user_achievements ua
		 join users u on u.id = ua.user_id
		 join nutrition_achievement_catalog c on c.id = ua.achievement_id
		 where u.role = 'employee'
		   and lower(btrim(coalesce(u.department, ''))) = lower(btrim($1))
		   and ua.unlocked = true
		 order by ua.unlocked_at desc nulls last
		 limit $2`,
		department,
		limit,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	result := []managerAchievementRow{}
	for rows.Next() {
		var item managerAchievementRow
		var unlockedAt sql.NullTime
		if scanErr := rows.Scan(&item.EmployeeName, &item.EmployeeID, &item.Title, &unlockedAt); scanErr != nil {
			continue
		}
		if unlockedAt.Valid {
			item.UnlockedAt = unlockedAt.Time.Format("02.01.2006 15:04")
		} else {
			item.UnlockedAt = "—"
		}
		result = append(result, item)
	}
	return result
}

func (s *Site) loadManagerDepartmentLedger(department string, limit int) []nutritionPointsLedgerView {
	if limit <= 0 {
		limit = 120
	}
	rows, err := s.DB.Query(
		`select l.user_id,
		        coalesce(u.employee_id, ''),
		        coalesce(u.name, ''),
		        l.change_amount,
		        coalesce(l.balance_after, 0),
		        coalesce(l.reason_code, ''),
		        coalesce(l.reason, ''),
		        coalesce(l.source_type, ''),
		        l.created_at,
		        coalesce(cu.name, ''),
		        coalesce(cu.id::text, '')
		 from nutrition_points_ledger l
		 join users u on u.id = l.user_id
		 left join users cu on cu.id = l.created_by
		 where u.role = 'employee'
		   and lower(btrim(coalesce(u.department, ''))) = lower(btrim($1))
		 order by l.created_at desc
		 limit $2`,
		department,
		limit,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	rowsOut := []nutritionPointsLedgerView{}
	for rows.Next() {
		var row nutritionPointsLedgerView
		var createdAt time.Time
		if scanErr := rows.Scan(
			&row.UserID,
			&row.EmployeeID,
			&row.UserName,
			&row.Change,
			&row.Balance,
			&row.ReasonCode,
			&row.Reason,
			&row.SourceType,
			&createdAt,
			&row.CreatedBy,
			&row.CreatedByID,
		); scanErr != nil {
			continue
		}
		row.CreatedAt = createdAt.Format("02.01.2006 15:04")
		rowsOut = append(rowsOut, row)
	}
	return rowsOut
}

func (s *Site) managerCanAccessEmployee(managerID, employeeID string) bool {
	managerDepartment, ok := s.loadManagerDepartment(managerID)
	if !ok {
		return false
	}
	var exists bool
	_ = s.DB.QueryRow(
		`select exists(
		   select 1
		   from users
		   where id = $1
		     and role = 'employee'
		     and lower(btrim(coalesce(department, ''))) = lower(btrim($2))
		 )`,
		employeeID,
		managerDepartment,
	).Scan(&exists)
	return exists
}

func (s *Site) loadManagerDepartment(managerID string) (string, bool) {
	var department string
	err := s.DB.QueryRow(
		`select coalesce(department, '')
		 from users
		 where id = $1 and role = 'manager'`,
		managerID,
	).Scan(&department)
	if err != nil || strings.TrimSpace(department) == "" {
		return "", false
	}
	return department, true
}

func (s *Site) loadNutritionManagerRewardRequestNotifications(managerID string, clearedAt time.Time) []notificationHistoryEntry {
	department, ok := s.loadManagerDepartment(managerID)
	if !ok {
		return nil
	}
	rows, err := s.DB.Query(
		`select u.name,
		        coalesce(u.employee_id, ''),
		        rr.reward_title,
		        coalesce(rr.requested_at, rr.redeemed_at, now())
		 from nutrition_reward_redemptions rr
		 join users u on u.id = rr.user_id
		 where u.role = 'employee'
		   and lower(btrim(coalesce(u.department, ''))) = lower(btrim($1))
		   and lower(btrim(coalesce(rr.status, ''))) = 'pending'
		   and coalesce(rr.requested_at, rr.redeemed_at, now()) > $2
		 order by coalesce(rr.requested_at, rr.redeemed_at, now()) desc
		 limit 20`,
		department,
		clearedAt,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	entries := []notificationHistoryEntry{}
	for rows.Next() {
		var employeeName string
		var employeeID string
		var rewardTitle string
		var createdAt time.Time
		if scanErr := rows.Scan(&employeeName, &employeeID, &rewardTitle, &createdAt); scanErr != nil {
			continue
		}
		reason := "Новая заявка на поощрение: " + employeeName
		if strings.TrimSpace(employeeID) != "" {
			reason += " (табельный номер " + employeeID + ")"
		}
		reason += " · " + rewardTitle
		entries = append(entries, notificationHistoryEntry{When: createdAt, Reason: reason})
	}
	return entries
}
