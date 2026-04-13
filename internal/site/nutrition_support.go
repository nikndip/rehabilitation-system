package site

import (
	"database/sql"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"rehab-app/internal/middleware"
)

type supportTicketListItem struct {
	ID             string
	Subject        string
	Status         string
	CreatedAt      string
	UpdatedAt      string
	LastMessageAt  string
	SLADueAt       string
	SLAOverdue     bool
	EmployeeName   string
	EmployeeID     string
	EmployeeDept   string
	UnreadForAdmin bool
}

type supportTicketThreadView struct {
	ID            string
	Subject       string
	Status        string
	CreatedAt     string
	UpdatedAt     string
	LastMessageAt string
	SLADueAt      string
	SLAOverdue    bool
	EmployeeName  string
	EmployeeID    string
	EmployeeDept  string
}

type supportTicketMessageView struct {
	Message    string
	CreatedAt  string
	SenderName string
	SenderRole string
	RoleClass  string
}

func (s *Site) nutritionInstructionEmployeePage(w http.ResponseWriter, r *http.Request) {
	data := s.nutritionBaseData(r, "Инструкция пользователя", "nutrition-profile")
	s.render(w, "nutrition_instruction_employee", data)
}

func (s *Site) nutritionInstructionAdminPage(w http.ResponseWriter, r *http.Request) {
	data := s.nutritionBaseData(r, "Инструкция администратора", "nutrition-profile")
	s.render(w, "nutrition_instruction_admin", data)
}

func (s *Site) nutritionInstructionManagerPage(w http.ResponseWriter, r *http.Request) {
	data := s.nutritionBaseData(r, "Инструкция руководителя", "nutrition-profile")
	s.render(w, "nutrition_instruction_manager", data)
}

func (s *Site) nutritionSupportTicketsPage(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	data := s.nutritionBaseData(r, "Поддержка питания", "nutrition-support")
	data["Error"] = r.URL.Query().Get("error")
	data["Success"] = r.URL.Query().Get("success")
	data["Tickets"] = s.loadSupportTicketsForUser(user.ID)
	data["Contacts"] = nutritionSupportContacts()
	data["FAQ"] = nutritionSupportFAQ()
	s.render(w, "nutrition_support", data)
}

func (s *Site) nutritionSupportCreate(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/nutrition/support?error=Некорректные%20данные%20формы", http.StatusSeeOther)
		return
	}

	subject := strings.TrimSpace(r.FormValue("subject"))
	message := strings.TrimSpace(r.FormValue("message"))
	if subject == "" || message == "" {
		http.Redirect(w, r, "/nutrition/support?error=Укажите%20тему%20и%20текст%20обращения", http.StatusSeeOther)
		return
	}
	if len(subject) > 200 {
		http.Redirect(w, r, "/nutrition/support?error=Тема%20обращения%20слишком%20длинная", http.StatusSeeOther)
		return
	}
	if len(message) > 4000 {
		http.Redirect(w, r, "/nutrition/support?error=Текст%20обращения%20слишком%20длинный", http.StatusSeeOther)
		return
	}

	tx, err := s.DB.Begin()
	if err != nil {
		http.Redirect(w, r, "/nutrition/support?error=Не%20удалось%20создать%20обращение", http.StatusSeeOther)
		return
	}
	defer tx.Rollback()

	var ticketID string
	err = tx.QueryRow(
		`insert into support_tickets (user_id, subject, status, created_at, updated_at, last_message_at)
		 values ($1, $2, 'open', now(), now(), now())
		 returning id::text`,
		user.ID,
		subject,
	).Scan(&ticketID)
	if err != nil {
		http.Redirect(w, r, "/nutrition/support?error=Не%20удалось%20сохранить%20обращение", http.StatusSeeOther)
		return
	}

	_, err = tx.Exec(
		`insert into support_ticket_messages (ticket_id, sender_id, sender_role, message)
		 values ($1, $2, 'employee', $3)`,
		ticketID,
		user.ID,
		message,
	)
	if err != nil {
		http.Redirect(w, r, "/nutrition/support?error=Не%20удалось%20сохранить%20сообщение", http.StatusSeeOther)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Redirect(w, r, "/nutrition/support?error=Не%20удалось%20завершить%20операцию", http.StatusSeeOther)
		return
	}

	s.insertNutritionEvent(user.ID, "Создано обращение в поддержку: «"+subject+"».")
	s.logNutritionAudit(
		user,
		"support_ticket_created",
		"support_ticket",
		ticketID,
		user.ID,
		strings.TrimSpace(user.Department),
		map[string]any{
			"subject": subject,
		},
	)
	http.Redirect(w, r, "/nutrition/support/"+ticketID+"?success="+url.QueryEscape("Обращение создано"), http.StatusSeeOther)
}

func (s *Site) nutritionSupportThreadPage(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	ticketID := normalizeResourceID(chi.URLParam(r, "id"))
	if ticketID == "" {
		http.Redirect(w, r, "/nutrition/support?error=Обращение%20не%20найдено", http.StatusSeeOther)
		return
	}

	ticket, ok := s.loadSupportTicketForUser(user.ID, ticketID)
	if !ok {
		http.Redirect(w, r, "/nutrition/support?error=Обращение%20не%20найдено", http.StatusSeeOther)
		return
	}

	data := s.nutritionBaseData(r, "Поддержка питания", "nutrition-support")
	data["Error"] = r.URL.Query().Get("error")
	data["Success"] = r.URL.Query().Get("success")
	data["Ticket"] = ticket
	data["Messages"] = s.loadSupportTicketMessages(ticketID)
	data["Contacts"] = nutritionSupportContacts()
	data["FAQ"] = nutritionSupportFAQ()
	s.render(w, "nutrition_support_thread", data)
}

func (s *Site) nutritionSupportMessageCreate(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	ticketID := normalizeResourceID(chi.URLParam(r, "id"))
	if ticketID == "" {
		http.Redirect(w, r, "/nutrition/support?error=Обращение%20не%20найдено", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/nutrition/support/"+ticketID+"?error=Некорректные%20данные%20сообщения", http.StatusSeeOther)
		return
	}

	message := strings.TrimSpace(r.FormValue("message"))
	if message == "" {
		http.Redirect(w, r, "/nutrition/support/"+ticketID+"?error=Сообщение%20не%20может%20быть%20пустым", http.StatusSeeOther)
		return
	}
	if len(message) > 4000 {
		http.Redirect(w, r, "/nutrition/support/"+ticketID+"?error=Сообщение%20слишком%20длинное", http.StatusSeeOther)
		return
	}

	var status string
	err := s.DB.QueryRow(
		`select status
		 from support_tickets
		 where id = $1 and user_id = $2`,
		ticketID,
		user.ID,
	).Scan(&status)
	if err != nil {
		http.Redirect(w, r, "/nutrition/support?error=Обращение%20не%20найдено", http.StatusSeeOther)
		return
	}
	if strings.EqualFold(strings.TrimSpace(status), "closed") {
		http.Redirect(w, r, "/nutrition/support/"+ticketID+"?error=Обращение%20закрыто", http.StatusSeeOther)
		return
	}

	var messageID string
	err = s.DB.QueryRow(
		`insert into support_ticket_messages (ticket_id, sender_id, sender_role, message)
		 values ($1, $2, 'employee', $3)
		 returning id::text`,
		ticketID,
		user.ID,
		message,
	).Scan(&messageID)
	if err != nil {
		http.Redirect(w, r, "/nutrition/support/"+ticketID+"?error=Не%20удалось%20сохранить%20сообщение", http.StatusSeeOther)
		return
	}

	_, _ = s.DB.Exec(
		`update support_tickets
		 set status = 'open',
		     updated_at = now(),
		     last_message_at = now()
		 where id = $1 and user_id = $2`,
		ticketID,
		user.ID,
	)
	s.logNutritionAudit(
		user,
		"support_message_employee",
		"support_message",
		messageID,
		user.ID,
		strings.TrimSpace(user.Department),
		map[string]any{
			"ticket_id": ticketID,
		},
	)

	http.Redirect(w, r, "/nutrition/support/"+ticketID+"?success=Сообщение%20отправлено", http.StatusSeeOther)
}

func (s *Site) nutritionSupportClose(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	ticketID := normalizeResourceID(chi.URLParam(r, "id"))
	if ticketID == "" {
		http.Redirect(w, r, "/nutrition/support?error=Обращение%20не%20найдено", http.StatusSeeOther)
		return
	}

	res, err := s.DB.Exec(
		`update support_tickets
		 set status = 'closed',
		     updated_at = now()
		 where id = $1 and user_id = $2 and status <> 'closed'`,
		ticketID,
		user.ID,
	)
	if err != nil {
		http.Redirect(w, r, "/nutrition/support/"+ticketID+"?error=Не%20удалось%20закрыть%20обращение", http.StatusSeeOther)
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		http.Redirect(w, r, "/nutrition/support/"+ticketID+"?error=Обращение%20уже%20закрыто", http.StatusSeeOther)
		return
	}
	s.logNutritionAudit(
		user,
		"support_ticket_closed_by_employee",
		"support_ticket",
		ticketID,
		user.ID,
		strings.TrimSpace(user.Department),
		map[string]any{
			"status": "closed",
		},
	)
	http.Redirect(w, r, "/nutrition/support/"+ticketID+"?success=Обращение%20закрыто", http.StatusSeeOther)
}

func (s *Site) adminNutritionSupportPage(w http.ResponseWriter, r *http.Request) {
	data := s.nutritionBaseData(r, "Админка питания: обращения", "nutrition-admin")
	data["Error"] = r.URL.Query().Get("error")
	data["Success"] = r.URL.Query().Get("success")
	data["Tickets"] = s.loadSupportTicketsForAdmin(300)
	s.render(w, "admin_nutrition_support", data)
}

func (s *Site) adminNutritionSupportThreadPage(w http.ResponseWriter, r *http.Request) {
	ticketID := normalizeResourceID(chi.URLParam(r, "id"))
	if ticketID == "" {
		http.Redirect(w, r, "/admin/nutrition/support?error=Обращение%20не%20найдено", http.StatusSeeOther)
		return
	}

	ticket, ok := s.loadSupportTicketForAdmin(ticketID)
	if !ok {
		http.Redirect(w, r, "/admin/nutrition/support?error=Обращение%20не%20найдено", http.StatusSeeOther)
		return
	}

	data := s.nutritionBaseData(r, "Админка питания: обращение", "nutrition-admin")
	data["Error"] = r.URL.Query().Get("error")
	data["Success"] = r.URL.Query().Get("success")
	data["Ticket"] = ticket
	data["Messages"] = s.loadSupportTicketMessages(ticketID)
	s.render(w, "admin_nutrition_support_thread", data)
}

func (s *Site) adminNutritionSupportMessageCreate(w http.ResponseWriter, r *http.Request) {
	admin := middleware.UserFromContext(r.Context())
	ticketID := normalizeResourceID(chi.URLParam(r, "id"))
	if ticketID == "" {
		http.Redirect(w, r, "/admin/nutrition/support?error=Обращение%20не%20найдено", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/nutrition/support/"+ticketID+"?error=Некорректные%20данные%20сообщения", http.StatusSeeOther)
		return
	}
	message := strings.TrimSpace(r.FormValue("message"))
	if message == "" {
		http.Redirect(w, r, "/admin/nutrition/support/"+ticketID+"?error=Сообщение%20не%20может%20быть%20пустым", http.StatusSeeOther)
		return
	}
	if len(message) > 4000 {
		http.Redirect(w, r, "/admin/nutrition/support/"+ticketID+"?error=Сообщение%20слишком%20длинное", http.StatusSeeOther)
		return
	}

	var ticketUserID string
	var status string
	err := s.DB.QueryRow(
		`select user_id, status
		 from support_tickets
		 where id = $1`,
		ticketID,
	).Scan(&ticketUserID, &status)
	if err != nil {
		http.Redirect(w, r, "/admin/nutrition/support?error=Обращение%20не%20найдено", http.StatusSeeOther)
		return
	}
	if strings.EqualFold(strings.TrimSpace(status), "closed") {
		http.Redirect(w, r, "/admin/nutrition/support/"+ticketID+"?error=Обращение%20закрыто", http.StatusSeeOther)
		return
	}

	var messageID string
	err = s.DB.QueryRow(
		`insert into support_ticket_messages (ticket_id, sender_id, sender_role, message)
		 values ($1, $2, 'admin', $3)
		 returning id::text`,
		ticketID,
		admin.ID,
		message,
	).Scan(&messageID)
	if err != nil {
		http.Redirect(w, r, "/admin/nutrition/support/"+ticketID+"?error=Не%20удалось%20сохранить%20ответ", http.StatusSeeOther)
		return
	}

	_, _ = s.DB.Exec(
		`update support_tickets
		 set status = 'answered',
		     updated_at = now(),
		     last_message_at = now()
		 where id = $1`,
		ticketID,
	)

	s.insertNutritionEvent(ticketUserID, "Поддержка ответила на обращение «"+ticketID+"».")
	s.logNutritionAudit(
		admin,
		"support_message_admin",
		"support_message",
		messageID,
		ticketUserID,
		"",
		map[string]any{
			"ticket_id": ticketID,
		},
	)
	http.Redirect(w, r, "/admin/nutrition/support/"+ticketID+"?success=Ответ%20отправлен", http.StatusSeeOther)
}

func (s *Site) adminNutritionSupportStatusUpdate(w http.ResponseWriter, r *http.Request) {
	admin := middleware.UserFromContext(r.Context())
	ticketID := normalizeResourceID(chi.URLParam(r, "id"))
	if ticketID == "" {
		http.Redirect(w, r, "/admin/nutrition/support?error=Обращение%20не%20найдено", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/nutrition/support/"+ticketID+"?error=Некорректные%20данные", http.StatusSeeOther)
		return
	}
	status := strings.ToLower(strings.TrimSpace(r.FormValue("status")))
	switch status {
	case "open", "answered", "closed":
	default:
		http.Redirect(w, r, "/admin/nutrition/support/"+ticketID+"?error=Некорректный%20статус", http.StatusSeeOther)
		return
	}

	var currentStatus string
	var ticketUserID string
	err := s.DB.QueryRow(
		`select status, user_id
		 from support_tickets
		 where id = $1`,
		ticketID,
	).Scan(&currentStatus, &ticketUserID)
	if err != nil {
		http.Redirect(w, r, "/admin/nutrition/support?error=Обращение%20не%20найдено", http.StatusSeeOther)
		return
	}

	res, err := s.DB.Exec(
		`update support_tickets
		 set status = $2,
		     updated_at = now()
		 where id = $1`,
		ticketID,
		status,
	)
	if err != nil {
		http.Redirect(w, r, "/admin/nutrition/support/"+ticketID+"?error=Не%20удалось%20обновить%20статус", http.StatusSeeOther)
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		http.Redirect(w, r, "/admin/nutrition/support?error=Обращение%20не%20найдено", http.StatusSeeOther)
		return
	}
	s.logNutritionAudit(
		admin,
		"support_status_changed",
		"support_ticket",
		ticketID,
		ticketUserID,
		"",
		map[string]any{
			"from": strings.ToLower(strings.TrimSpace(currentStatus)),
			"to":   status,
		},
	)
	http.Redirect(w, r, "/admin/nutrition/support/"+ticketID+"?success=Статус%20обращения%20обновлен", http.StatusSeeOther)
}

func (s *Site) managerNutritionSupportPage(w http.ResponseWriter, r *http.Request) {
	manager := middleware.UserFromContext(r.Context())
	department := strings.TrimSpace(manager.Department)
	if department == "" {
		http.Error(w, "Для роли руководителя не задано подразделение", http.StatusForbidden)
		return
	}

	data := s.nutritionBaseData(r, "Руководитель: обращения отдела", "nutrition-manager")
	data["Error"] = r.URL.Query().Get("error")
	data["Success"] = r.URL.Query().Get("success")
	data["Department"] = department
	data["Tickets"] = s.loadSupportTicketsForManager(department, 300)
	s.render(w, "manager_nutrition_support", data)
}

func (s *Site) managerNutritionSupportThreadPage(w http.ResponseWriter, r *http.Request) {
	manager := middleware.UserFromContext(r.Context())
	department := strings.TrimSpace(manager.Department)
	if department == "" {
		http.Error(w, "Для роли руководителя не задано подразделение", http.StatusForbidden)
		return
	}

	ticketID := normalizeResourceID(chi.URLParam(r, "id"))
	if ticketID == "" {
		http.Redirect(w, r, "/manager/nutrition/support?error=Обращение%20не%20найдено", http.StatusSeeOther)
		return
	}

	ticket, ok := s.loadSupportTicketForManager(department, ticketID)
	if !ok {
		http.Redirect(w, r, "/manager/nutrition/support?error=Обращение%20не%20найдено%20или%20нет%20доступа", http.StatusSeeOther)
		return
	}

	data := s.nutritionBaseData(r, "Руководитель: обращение", "nutrition-manager")
	data["Error"] = r.URL.Query().Get("error")
	data["Success"] = r.URL.Query().Get("success")
	data["Department"] = department
	data["Ticket"] = ticket
	data["Messages"] = s.loadSupportTicketMessages(ticketID)
	s.render(w, "manager_nutrition_support_thread", data)
}

func (s *Site) managerNutritionSupportMessageCreate(w http.ResponseWriter, r *http.Request) {
	manager := middleware.UserFromContext(r.Context())
	department := strings.TrimSpace(manager.Department)
	if department == "" {
		http.Error(w, "Для роли руководителя не задано подразделение", http.StatusForbidden)
		return
	}

	ticketID := normalizeResourceID(chi.URLParam(r, "id"))
	if ticketID == "" {
		http.Redirect(w, r, "/manager/nutrition/support?error=Обращение%20не%20найдено", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/manager/nutrition/support/"+ticketID+"?error=Некорректные%20данные%20сообщения", http.StatusSeeOther)
		return
	}

	message := strings.TrimSpace(r.FormValue("message"))
	if message == "" {
		http.Redirect(w, r, "/manager/nutrition/support/"+ticketID+"?error=Сообщение%20не%20может%20быть%20пустым", http.StatusSeeOther)
		return
	}
	if len(message) > 4000 {
		http.Redirect(w, r, "/manager/nutrition/support/"+ticketID+"?error=Сообщение%20слишком%20длинное", http.StatusSeeOther)
		return
	}

	var ticketUserID string
	var status string
	err := s.DB.QueryRow(
		`select t.user_id, t.status
		 from support_tickets t
		 join users u on u.id = t.user_id
		 where t.id = $1
		   and lower(btrim(coalesce(u.department, ''))) = lower(btrim($2))`,
		ticketID,
		department,
	).Scan(&ticketUserID, &status)
	if err != nil {
		http.Redirect(w, r, "/manager/nutrition/support?error=Обращение%20не%20найдено%20или%20нет%20доступа", http.StatusSeeOther)
		return
	}
	if strings.EqualFold(strings.TrimSpace(status), "closed") {
		http.Redirect(w, r, "/manager/nutrition/support/"+ticketID+"?error=Обращение%20закрыто", http.StatusSeeOther)
		return
	}

	var messageID string
	err = s.DB.QueryRow(
		`insert into support_ticket_messages (ticket_id, sender_id, sender_role, message)
		 values ($1, $2, 'manager', $3)
		 returning id::text`,
		ticketID,
		manager.ID,
		message,
	).Scan(&messageID)
	if err != nil {
		http.Redirect(w, r, "/manager/nutrition/support/"+ticketID+"?error=Не%20удалось%20сохранить%20ответ", http.StatusSeeOther)
		return
	}

	_, _ = s.DB.Exec(
		`update support_tickets
		 set status = 'answered',
		     updated_at = now(),
		     last_message_at = now()
		 where id = $1`,
		ticketID,
	)

	s.insertNutritionEvent(ticketUserID, "Руководитель ответил на обращение «"+ticketID+"».")
	s.logNutritionAudit(
		manager,
		"support_message_manager",
		"support_message",
		messageID,
		ticketUserID,
		department,
		map[string]any{
			"ticket_id": ticketID,
		},
	)
	http.Redirect(w, r, "/manager/nutrition/support/"+ticketID+"?success=Ответ%20отправлен", http.StatusSeeOther)
}

func (s *Site) loadSupportTicketsForUser(userID string) []supportTicketListItem {
	rows, err := s.DB.Query(
		`select id, subject, status, created_at, updated_at, last_message_at
		 from support_tickets
		 where user_id = $1
		 order by last_message_at desc, created_at desc`,
		userID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	items := []supportTicketListItem{}
	now := time.Now()
	for rows.Next() {
		var item supportTicketListItem
		var createdAt time.Time
		var updatedAt time.Time
		var lastMessageAt time.Time
		if scanErr := rows.Scan(&item.ID, &item.Subject, &item.Status, &createdAt, &updatedAt, &lastMessageAt); scanErr != nil {
			continue
		}
		item.CreatedAt = createdAt.Format("02.01.2006 15:04")
		item.UpdatedAt = updatedAt.Format("02.01.2006 15:04")
		item.LastMessageAt = lastMessageAt.Format("02.01.2006 15:04")
		dueAt := nutritionSupportSLADueAt(lastMessageAt)
		item.SLADueAt = dueAt.Format("02.01.2006 15:04")
		item.SLAOverdue = strings.EqualFold(strings.TrimSpace(item.Status), "open") && now.After(dueAt)
		items = append(items, item)
	}
	return items
}

func (s *Site) loadSupportTicketsForAdmin(limit int) []supportTicketListItem {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.DB.Query(
		`select t.id,
		        t.subject,
		        t.status,
		        t.created_at,
		        t.updated_at,
		        t.last_message_at,
		        u.name,
		        coalesce(u.employee_id, ''),
		        coalesce(u.department, '')
		 from support_tickets t
		 join users u on u.id = t.user_id
		 order by case t.status
		            when 'open' then 0
		            when 'answered' then 1
		            else 2
		          end,
		          t.last_message_at desc
		 limit $1`,
		limit,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	items := []supportTicketListItem{}
	now := time.Now()
	for rows.Next() {
		var item supportTicketListItem
		var createdAt time.Time
		var updatedAt time.Time
		var lastMessageAt time.Time
		if scanErr := rows.Scan(
			&item.ID,
			&item.Subject,
			&item.Status,
			&createdAt,
			&updatedAt,
			&lastMessageAt,
			&item.EmployeeName,
			&item.EmployeeID,
			&item.EmployeeDept,
		); scanErr != nil {
			continue
		}
		item.CreatedAt = createdAt.Format("02.01.2006 15:04")
		item.UpdatedAt = updatedAt.Format("02.01.2006 15:04")
		item.LastMessageAt = lastMessageAt.Format("02.01.2006 15:04")
		dueAt := nutritionSupportSLADueAt(lastMessageAt)
		item.SLADueAt = dueAt.Format("02.01.2006 15:04")
		item.SLAOverdue = strings.EqualFold(strings.TrimSpace(item.Status), "open") && now.After(dueAt)
		item.UnreadForAdmin = strings.EqualFold(strings.TrimSpace(item.Status), "open")
		items = append(items, item)
	}
	return items
}

func (s *Site) loadSupportTicketsForManager(department string, limit int) []supportTicketListItem {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.DB.Query(
		`select t.id::text,
		        t.subject,
		        t.status,
		        t.created_at,
		        t.updated_at,
		        t.last_message_at,
		        u.name,
		        coalesce(u.employee_id, ''),
		        coalesce(u.department, '')
		 from support_tickets t
		 join users u on u.id = t.user_id
		 where u.role = 'employee'
		   and lower(btrim(coalesce(u.department, ''))) = lower(btrim($1))
		 order by case t.status
		            when 'open' then 0
		            when 'answered' then 1
		            else 2
		          end,
		          t.last_message_at desc
		 limit $2`,
		department,
		limit,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	items := []supportTicketListItem{}
	now := time.Now()
	for rows.Next() {
		var item supportTicketListItem
		var createdAt time.Time
		var updatedAt time.Time
		var lastMessageAt time.Time
		if scanErr := rows.Scan(
			&item.ID,
			&item.Subject,
			&item.Status,
			&createdAt,
			&updatedAt,
			&lastMessageAt,
			&item.EmployeeName,
			&item.EmployeeID,
			&item.EmployeeDept,
		); scanErr != nil {
			continue
		}
		item.CreatedAt = createdAt.Format("02.01.2006 15:04")
		item.UpdatedAt = updatedAt.Format("02.01.2006 15:04")
		item.LastMessageAt = lastMessageAt.Format("02.01.2006 15:04")
		dueAt := nutritionSupportSLADueAt(lastMessageAt)
		item.SLADueAt = dueAt.Format("02.01.2006 15:04")
		item.SLAOverdue = strings.EqualFold(strings.TrimSpace(item.Status), "open") && now.After(dueAt)
		items = append(items, item)
	}
	return items
}

func (s *Site) loadSupportTicketForUser(userID, ticketID string) (supportTicketThreadView, bool) {
	return s.loadSupportTicketByAccess(ticketID, "and t.user_id = $2", userID)
}

func (s *Site) loadSupportTicketForAdmin(ticketID string) (supportTicketThreadView, bool) {
	return s.loadSupportTicketByAccess(ticketID, "", "")
}

func (s *Site) loadSupportTicketForManager(department, ticketID string) (supportTicketThreadView, bool) {
	return s.loadSupportTicketByAccess(
		ticketID,
		"and lower(btrim(coalesce(u.department, ''))) = lower(btrim($2))",
		department,
	)
}

func (s *Site) loadSupportTicketByAccess(ticketID, whereTail, userID string) (supportTicketThreadView, bool) {
	query := `select t.id::text,
	                 t.subject,
	                 t.status,
	                 t.created_at,
	                 t.updated_at,
	                 t.last_message_at,
	                 u.name,
	                 coalesce(u.employee_id, ''),
	                 coalesce(u.department, '')
	          from support_tickets t
	          join users u on u.id = t.user_id
	          where t.id = $1 ` + whereTail

	var item supportTicketThreadView
	var createdAt time.Time
	var updatedAt time.Time
	var lastMessageAt time.Time
	var err error
	if strings.TrimSpace(whereTail) == "" {
		err = s.DB.QueryRow(query, ticketID).Scan(
			&item.ID,
			&item.Subject,
			&item.Status,
			&createdAt,
			&updatedAt,
			&lastMessageAt,
			&item.EmployeeName,
			&item.EmployeeID,
			&item.EmployeeDept,
		)
	} else {
		err = s.DB.QueryRow(query, ticketID, userID).Scan(
			&item.ID,
			&item.Subject,
			&item.Status,
			&createdAt,
			&updatedAt,
			&lastMessageAt,
			&item.EmployeeName,
			&item.EmployeeID,
			&item.EmployeeDept,
		)
	}
	if err != nil {
		return supportTicketThreadView{}, false
	}
	item.CreatedAt = createdAt.Format("02.01.2006 15:04")
	item.UpdatedAt = updatedAt.Format("02.01.2006 15:04")
	item.LastMessageAt = lastMessageAt.Format("02.01.2006 15:04")
	dueAt := nutritionSupportSLADueAt(lastMessageAt)
	item.SLADueAt = dueAt.Format("02.01.2006 15:04")
	item.SLAOverdue = strings.EqualFold(strings.TrimSpace(item.Status), "open") && time.Now().After(dueAt)
	return item, true
}

func (s *Site) loadSupportTicketMessages(ticketID string) []supportTicketMessageView {
	rows, err := s.DB.Query(
		`select m.message,
		        m.created_at,
		        coalesce(u.name, ''),
		        coalesce(m.sender_role, 'employee')
		 from support_ticket_messages m
		 left join users u on u.id = m.sender_id
		 where m.ticket_id = $1
		 order by m.created_at asc`,
		ticketID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	messages := []supportTicketMessageView{}
	for rows.Next() {
		var item supportTicketMessageView
		var createdAt time.Time
		if scanErr := rows.Scan(&item.Message, &createdAt, &item.SenderName, &item.SenderRole); scanErr != nil {
			continue
		}
		item.CreatedAt = createdAt.Format("02.01.2006 15:04")
		item.SenderRole = strings.ToLower(strings.TrimSpace(item.SenderRole))
		switch item.SenderRole {
		case "admin", "manager":
			item.RoleClass = "admin"
		default:
			item.RoleClass = "employee"
		}
		if strings.TrimSpace(item.SenderName) == "" {
			switch item.SenderRole {
			case "admin":
				item.SenderName = "Администратор"
			case "manager":
				item.SenderName = "Руководитель"
			default:
				item.SenderName = "Сотрудник"
			}
		}
		messages = append(messages, item)
	}
	return messages
}

func nutritionSupportContacts() []nutritionSupportContact {
	return []nutritionSupportContact{
		{
			Title:       "Нутрициолог проекта",
			Description: "Персональные вопросы по рациону, восстановлению и корректировке плана.",
			ActionLabel: "Почта",
			ActionValue: "nutrition-support@company.local",
		},
		{
			Title:       "Координатор реабилитации",
			Description: "Организационные вопросы по модулю питания и порядку рассмотрения заявок.",
			ActionLabel: "Внутренний номер",
			ActionValue: "#4721",
		},
	}
}

func nutritionSupportFAQ() []nutritionFAQItem {
	return []nutritionFAQItem{
		{Question: "Как быстро обрабатываются обращения?", Answer: "Новые обращения обрабатываются в течение 1 рабочего дня."},
		{Question: "Куда писать по вопросам поощрений?", Answer: "Создайте обращение с темой про заявку на поощрение, укажите номер и описание ситуации."},
		{Question: "Можно ли прикрепить уточнение к уже открытому обращению?", Answer: "Да, откройте карточку обращения и отправьте дополнительное сообщение в переписке."},
	}
}

func (s *Site) loadNutritionAdminSupportNotifications(clearedAt time.Time) []notificationHistoryEntry {
	rows, err := s.DB.Query(
		`select u.name, coalesce(u.employee_id, ''), t.subject, t.created_at
		 from support_tickets t
		 join users u on u.id = t.user_id
		 where t.created_at > $1
		 order by t.created_at desc
		 limit 20`,
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
		var subject string
		var createdAt time.Time
		if scanErr := rows.Scan(&employeeName, &employeeID, &subject, &createdAt); scanErr != nil {
			continue
		}
		reason := "Новое обращение в поддержку: " + employeeName
		if strings.TrimSpace(employeeID) != "" {
			reason += " (табельный номер " + employeeID + ")"
		}
		if strings.TrimSpace(subject) != "" {
			reason += " · " + subject
		}
		entries = append(entries, notificationHistoryEntry{When: createdAt, Reason: reason})
	}
	return entries
}

func (s *Site) loadNutritionManagerSupportNotifications(managerID string, clearedAt time.Time) []notificationHistoryEntry {
	department, ok := s.loadManagerDepartment(managerID)
	if !ok {
		return nil
	}
	rows, err := s.DB.Query(
		`select u.name, coalesce(u.employee_id, ''), t.subject, t.created_at
		 from support_tickets t
		 join users u on u.id = t.user_id
		 where u.role = 'employee'
		   and lower(btrim(coalesce(u.department, ''))) = lower(btrim($1))
		   and t.created_at > $2
		 order by t.created_at desc
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
		var subject string
		var createdAt time.Time
		if scanErr := rows.Scan(&employeeName, &employeeID, &subject, &createdAt); scanErr != nil {
			continue
		}
		reason := "Новое обращение сотрудника отдела: " + employeeName
		if strings.TrimSpace(employeeID) != "" {
			reason += " (табельный номер " + employeeID + ")"
		}
		if strings.TrimSpace(subject) != "" {
			reason += " · " + subject
		}
		entries = append(entries, notificationHistoryEntry{When: createdAt, Reason: reason})
	}
	return entries
}

func normalizeSupportStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "open":
		return "open"
	case "answered":
		return "answered"
	case "closed":
		return "closed"
	default:
		return "open"
	}
}

func supportTicketExists(dbConn *sql.DB, ticketID string) bool {
	var exists bool
	_ = dbConn.QueryRow(`select exists(select 1 from support_tickets where id = $1)`, ticketID).Scan(&exists)
	return exists
}
