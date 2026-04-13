package site

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"rehab-app/internal/models"
)

const (
	nutritionRewardRequestSLADuration = 48 * time.Hour
	nutritionSupportResponseSLA       = 24 * time.Hour
)

type nutritionAuditRow struct {
	CreatedAt        string
	ActionType       string
	ActionLabel      string
	EntityType       string
	EntityLabel      string
	EntityID         string
	ActorName        string
	ActorRole        string
	ActorRoleLabel   string
	TargetUserName   string
	TargetDepartment string
	Details          string
}

func (s *Site) logNutritionAudit(actor *models.User, actionType, entityType, entityID, targetUserID, targetDepartment string, details map[string]any) {
	actionType = strings.TrimSpace(actionType)
	entityType = strings.TrimSpace(entityType)
	if actionType == "" || entityType == "" {
		return
	}

	actorRole := "system"
	actorID := sql.NullString{}
	actorName := ""
	if actor != nil {
		actorRole = strings.ToLower(strings.TrimSpace(actor.Role))
		if actorRole == "" {
			actorRole = "system"
		}
		actorID = sql.NullString{String: strings.TrimSpace(actor.ID), Valid: strings.TrimSpace(actor.ID) != ""}
		actorName = strings.TrimSpace(actor.Name)
	}

	payload := []byte("{}")
	if len(details) > 0 {
		encoded, err := json.Marshal(details)
		if err == nil {
			payload = encoded
		}
	}

	_, err := s.DB.Exec(
		`insert into nutrition_action_audit (
		    module, action_type, entity_type, entity_id,
		    actor_id, actor_role, actor_name,
		    target_user_id, target_department, details
		  )
		  values ('nutrition', $1, $2, $3, $4, $5, $6, nullif($7, '')::uuid, $8, $9::jsonb)`,
		actionType,
		entityType,
		strings.TrimSpace(entityID),
		nullableUUIDParam(actorID),
		actorRole,
		actorName,
		strings.TrimSpace(targetUserID),
		strings.TrimSpace(targetDepartment),
		string(payload),
	)
	if err != nil {
		log.Printf("nutrition: write audit failed action=%s entity=%s id=%s: %v", actionType, entityType, entityID, err)
	}
}

func nullableUUIDParam(value sql.NullString) any {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return nil
	}
	return strings.TrimSpace(value.String)
}

func (s *Site) adminNutritionAuditPage(w http.ResponseWriter, r *http.Request) {
	data := s.nutritionBaseData(r, "Админка питания: аудит", "nutrition-admin")
	data["Rows"] = s.loadNutritionAuditRows(300)
	s.render(w, "admin_nutrition_audit", data)
}

func (s *Site) loadNutritionAuditRows(limit int) []nutritionAuditRow {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.DB.Query(
		`select a.created_at,
		        a.action_type,
		        a.entity_type,
		        coalesce(a.entity_id, ''),
		        coalesce(a.actor_role, 'system'),
		        coalesce(nullif(a.actor_name, ''), au.name, 'Система'),
		        coalesce(tu.name, ''),
		        coalesce(a.target_department, ''),
		        coalesce(a.details, '{}'::jsonb)::text
		 from nutrition_action_audit a
		 left join users au on au.id = a.actor_id
		 left join users tu on tu.id = a.target_user_id
		 where coalesce(a.module, 'nutrition') = 'nutrition'
		 order by a.created_at desc
		 limit $1`,
		limit,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	list := []nutritionAuditRow{}
	for rows.Next() {
		var item nutritionAuditRow
		var createdAt time.Time
		if scanErr := rows.Scan(
			&createdAt,
			&item.ActionType,
			&item.EntityType,
			&item.EntityID,
			&item.ActorRole,
			&item.ActorName,
			&item.TargetUserName,
			&item.TargetDepartment,
			&item.Details,
		); scanErr != nil {
			continue
		}
		item.CreatedAt = createdAt.Format("02.01.2006 15:04")
		item.ActorRole = strings.ToLower(strings.TrimSpace(item.ActorRole))
		item.ActorRoleLabel = nutritionAuditActorRoleLabel(item.ActorRole)
		item.ActionLabel = nutritionAuditActionLabel(item.ActionType)
		item.EntityLabel = nutritionAuditEntityLabel(item.EntityType)
		item.Details = nutritionAuditDetailsPreview(item.Details)
		list = append(list, item)
	}
	return list
}

func nutritionAuditActionLabel(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "reward_request_created":
		return "Создана заявка на поощрение"
	case "reward_request_approved":
		return "Заявка на поощрение одобрена"
	case "reward_request_rejected":
		return "Заявка на поощрение отклонена"
	case "manager_points_awarded":
		return "Руководитель начислил баллы"
	case "support_ticket_created":
		return "Создано обращение в поддержку"
	case "support_message_employee":
		return "Сообщение сотрудника в обращении"
	case "support_message_manager":
		return "Ответ руководителя в обращении"
	case "support_message_admin":
		return "Ответ администратора в обращении"
	case "support_status_changed":
		return "Изменен статус обращения"
	case "support_ticket_closed_by_employee":
		return "Сотрудник закрыл обращение"
	default:
		return action
	}
}

func nutritionAuditEntityLabel(entity string) string {
	switch strings.ToLower(strings.TrimSpace(entity)) {
	case "reward_request":
		return "Заявка на поощрение"
	case "points_ledger":
		return "Операция с баллами"
	case "support_ticket":
		return "Обращение в поддержку"
	case "support_message":
		return "Сообщение обращения"
	default:
		return entity
	}
}

func nutritionAuditActorRoleLabel(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "employee":
		return "Сотрудник"
	case "manager":
		return "Руководитель"
	case "admin":
		return "Администратор"
	default:
		return "Система"
	}
}

func nutritionAuditDetailsPreview(value string) string {
	text := strings.TrimSpace(value)
	if text == "" || text == "{}" || text == "null" {
		return "—"
	}
	if len(text) > 220 {
		return text[:217] + "..."
	}
	return text
}

func nutritionRewardSLADueAt(requestedAt time.Time) time.Time {
	return requestedAt.Add(nutritionRewardRequestSLADuration)
}

func nutritionSupportSLADueAt(lastMessageAt time.Time) time.Time {
	return lastMessageAt.Add(nutritionSupportResponseSLA)
}

func (s *Site) loadNutritionManagerRewardSLANotifications(managerID string, clearedAt, now time.Time) []notificationHistoryEntry {
	department, ok := s.loadManagerDepartment(managerID)
	if !ok {
		return nil
	}

	rows, err := s.DB.Query(
		`select u.name,
		        coalesce(u.employee_id, ''),
		        rr.reward_title,
		        coalesce(rr.requested_at, rr.redeemed_at, now()) + interval '48 hours' as due_at
		 from nutrition_reward_redemptions rr
		 join users u on u.id = rr.user_id
		 where u.role = 'employee'
		   and lower(btrim(coalesce(u.department, ''))) = lower(btrim($1))
		   and lower(btrim(coalesce(rr.status, ''))) = 'pending'
		   and (coalesce(rr.requested_at, rr.redeemed_at, now()) + interval '48 hours') <= $2
		   and (coalesce(rr.requested_at, rr.redeemed_at, now()) + interval '48 hours') > $3
		 order by due_at desc
		 limit 20`,
		department,
		now,
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
		var dueAt time.Time
		if scanErr := rows.Scan(&employeeName, &employeeID, &rewardTitle, &dueAt); scanErr != nil {
			continue
		}
		reason := "SLA по заявке на поощрение просрочен: " + employeeName
		if strings.TrimSpace(employeeID) != "" {
			reason += " (табельный номер " + employeeID + ")"
		}
		if strings.TrimSpace(rewardTitle) != "" {
			reason += " · " + rewardTitle
		}
		entries = append(entries, notificationHistoryEntry{When: dueAt, Reason: reason})
	}
	return entries
}

func (s *Site) loadNutritionAdminRewardSLANotifications(clearedAt, now time.Time) []notificationHistoryEntry {
	rows, err := s.DB.Query(
		`select u.name,
		        coalesce(u.employee_id, ''),
		        rr.reward_title,
		        coalesce(rr.requested_at, rr.redeemed_at, now()) + interval '48 hours' as due_at
		 from nutrition_reward_redemptions rr
		 join users u on u.id = rr.user_id
		 where u.role = 'employee'
		   and lower(btrim(coalesce(rr.status, ''))) = 'pending'
		   and (coalesce(rr.requested_at, rr.redeemed_at, now()) + interval '48 hours') <= $1
		   and (coalesce(rr.requested_at, rr.redeemed_at, now()) + interval '48 hours') > $2
		 order by due_at desc
		 limit 20`,
		now,
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
		var dueAt time.Time
		if scanErr := rows.Scan(&employeeName, &employeeID, &rewardTitle, &dueAt); scanErr != nil {
			continue
		}
		reason := "Просрочена заявка на поощрение: " + employeeName
		if strings.TrimSpace(employeeID) != "" {
			reason += " (табельный номер " + employeeID + ")"
		}
		if strings.TrimSpace(rewardTitle) != "" {
			reason += " · " + rewardTitle
		}
		entries = append(entries, notificationHistoryEntry{When: dueAt, Reason: reason})
	}
	return entries
}

func (s *Site) loadNutritionAdminSupportSLANotifications(clearedAt, now time.Time) []notificationHistoryEntry {
	rows, err := s.DB.Query(
		`select u.name,
		        coalesce(u.employee_id, ''),
		        t.subject,
		        t.last_message_at + interval '24 hours' as due_at
		 from support_tickets t
		 join users u on u.id = t.user_id
		 where lower(btrim(coalesce(t.status, ''))) = 'open'
		   and (t.last_message_at + interval '24 hours') <= $1
		   and (t.last_message_at + interval '24 hours') > $2
		 order by due_at desc
		 limit 20`,
		now,
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
		var dueAt time.Time
		if scanErr := rows.Scan(&employeeName, &employeeID, &subject, &dueAt); scanErr != nil {
			continue
		}
		reason := "Просрочено обращение поддержки: " + employeeName
		if strings.TrimSpace(employeeID) != "" {
			reason += " (табельный номер " + employeeID + ")"
		}
		if strings.TrimSpace(subject) != "" {
			reason += " · " + subject
		}
		entries = append(entries, notificationHistoryEntry{When: dueAt, Reason: reason})
	}
	return entries
}

func (s *Site) loadNutritionManagerSupportSLANotifications(managerID string, clearedAt, now time.Time) []notificationHistoryEntry {
	department, ok := s.loadManagerDepartment(managerID)
	if !ok {
		return nil
	}
	rows, err := s.DB.Query(
		`select u.name,
		        coalesce(u.employee_id, ''),
		        t.subject,
		        t.last_message_at + interval '24 hours' as due_at
		 from support_tickets t
		 join users u on u.id = t.user_id
		 where u.role = 'employee'
		   and lower(btrim(coalesce(u.department, ''))) = lower(btrim($1))
		   and lower(btrim(coalesce(t.status, ''))) = 'open'
		   and (t.last_message_at + interval '24 hours') <= $2
		   and (t.last_message_at + interval '24 hours') > $3
		 order by due_at desc
		 limit 20`,
		department,
		now,
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
		var dueAt time.Time
		if scanErr := rows.Scan(&employeeName, &employeeID, &subject, &dueAt); scanErr != nil {
			continue
		}
		reason := "SLA ответа по обращению просрочен: " + employeeName
		if strings.TrimSpace(employeeID) != "" {
			reason += " (табельный номер " + employeeID + ")"
		}
		if strings.TrimSpace(subject) != "" {
			reason += " · " + subject
		}
		entries = append(entries, notificationHistoryEntry{When: dueAt, Reason: reason})
	}
	return entries
}
