package site

import (
	"database/sql"
	"encoding/json"
	"math"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"rehab-app/internal/middleware"
)

type questionnaireData struct {
	Goal           string   `json:"goal"`
	FitnessLevel   string   `json:"fitness_level"`
	DaysPerWeek    int      `json:"days_per_week"`
	SessionMinutes int      `json:"session_minutes"`
	Equipment      []string `json:"equipment"`
	Preferences    string   `json:"preferences"`
}

type planRecord struct {
	ID           string
	Goal         string
	Level        string
	Frequency    int
	Status       string
	PausedReason string
	CreatedAt    time.Time
}

type planWorkoutView struct {
	ID            string
	WorkoutID     string
	Name          string
	Description   string
	Difficulty    string
	Category      string
	Duration      int
	Week          int
	Day           int
	Intensity     int
	ScheduledDate string
	Status        string
	SkipReason    string
	SessionID     string
}

type planCalendarDay struct {
	ID            string
	WorkoutID     string
	Name          string
	ScheduledDate string
	Week          int
	Day           int
	Intensity     int
	Status        string
}

type planCalendarWeek struct {
	Week int
	Days []planCalendarDay
}

type planChangeView struct {
	ChangedAt string
	Reason    string
}

type notificationHistoryEntry struct {
	When   time.Time
	Reason string
}

type leaderboardRow struct {
	Name         string
	Department   string
	Points       int
	Workouts     int
	Minutes      int
	LastWorkout  string
	AvgTolerance int
}

type achievementView struct {
	Title        string
	Description  string
	Icon         string
	Unlocked     bool
	Progress     int
	Total        int
	PointsReward int
}

type sessionFeedbackView struct {
	PerceivedExertion int
	Tolerance         int
	PainLevel         int
	Wellbeing         int
	Comment           string
}

func (s *Site) questionnairePage(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	w.Header().Set("Cache-Control", "no-store")
	data := s.baseData(r, "Опросник", "questionnaire")
	editMode := r.URL.Query().Get("edit") == "1"
	returnTo := strings.TrimSpace(r.URL.Query().Get("from"))
	if returnTo != "" && !strings.HasPrefix(returnTo, "/") {
		returnTo = ""
	}
	q, _ := s.loadQuestionnaire(user.ID)
	if q.SessionMinutes == 0 {
		q.SessionMinutes = sessionMinutesForLevel(resolveLevel(q.FitnessLevel))
	}
	data["Questionnaire"] = q
	data["Errors"] = map[string]string{}
	data["Restrictions"] = restrictionOptions()
	data["EquipmentOptions"] = equipmentOptions()
	data["PreferenceOptions"] = preferenceOptions()
	data["SelectedPreferences"] = parseCSV(q.Preferences)
	data["QuestionnaireComplete"] = !s.needsQuestionnaire(user.ID)
	data["EditMode"] = editMode
	data["ReturnTo"] = returnTo

	data["SelectedRestrictions"] = s.loadRestrictions(user.ID)
	data["DoctorApproval"] = s.loadDoctorApproval(user.ID)

	s.render(w, "questionnaire", data)
}

func (s *Site) questionnaireSubmit(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	previous, _ := s.loadQuestionnaire(user.ID)
	prevRestrictions := s.loadRestrictions(user.ID)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Некорректный запрос", http.StatusBadRequest)
		return
	}

	days, _ := strconv.Atoi(r.FormValue("days_per_week"))
	equipment := r.Form["equipment"]
	if len(equipment) == 1 && strings.TrimSpace(equipment[0]) == "" {
		equipment = []string{}
	}
	equipment = normalizeEquipmentSelection(equipment)
	selectedPreferences := normalizePreferenceSelection(r.Form["preferences"])
	preferences := strings.Join(selectedPreferences, ", ")

	q := questionnaireData{
		Goal:           strings.TrimSpace(r.FormValue("goal")),
		FitnessLevel:   strings.TrimSpace(r.FormValue("fitness_level")),
		DaysPerWeek:    days,
		SessionMinutes: 0,
		Equipment:      equipment,
		Preferences:    preferences,
	}
	q.SessionMinutes = sessionMinutesForLevel(resolveLevel(q.FitnessLevel))

	errors := validateQuestionnaire(q)
	if len(errors) > 0 {
		data := s.baseData(r, "Опросник", "questionnaire")
		data["Questionnaire"] = q
		data["Errors"] = errors
		data["Restrictions"] = restrictionOptions()
		data["EquipmentOptions"] = equipmentOptions()
		data["PreferenceOptions"] = preferenceOptions()
		data["SelectedPreferences"] = parseCSV(q.Preferences)
		data["SelectedRestrictions"] = r.Form["restrictions"]
		data["DoctorApproval"] = s.loadDoctorApproval(user.ID)
		s.render(w, "questionnaire", data)
		return
	}

	if err := s.saveQuestionnaire(user.ID, q); err != nil {
		http.Error(w, "Ошибка сохранения", http.StatusInternalServerError)
		return
	}

	goals := []string{}
	if q.Goal != "" {
		goals = append(goals, q.Goal)
	}
	_, _ = s.DB.Exec(
		`update user_profiles
     set fitness_level = $1,
         goals = $2,
         updated_at = now()
     where user_id = $3`,
		q.FitnessLevel,
		goals,
		user.ID,
	)
	_, _ = s.DB.Exec(
		`update training_plans
     set goal = $1, updated_at = now()
     where user_id = $2 and status in ('active', 'paused')`,
		q.Goal,
		user.ID,
	)

	restrictions := normalizeRestrictionSelection(r.Form["restrictions"])
	restrictionsCSV := strings.Join(restrictions, ",")
	if _, err := s.DB.Exec(
		`insert into medical_info (user_id, restrictions, updated_at)
     values (
       $1,
       case when trim($2) = '' then '{}'::text[] else string_to_array($2, ',') end,
       now()
     )
     on conflict (user_id)
     do update set restrictions = case when trim($2) = '' then '{}'::text[] else string_to_array($2, ',') end,
                   updated_at = now()`,
		user.ID,
		restrictionsCSV,
	); err != nil {
		http.Error(w, "Ошибка сохранения ограничений", http.StatusInternalServerError)
		return
	}

	if questionnaireChanged(previous, q, prevRestrictions, restrictions) {
		if plan, err := s.getActivePlan(user.ID); err == nil && plan != nil {
			if !adminPauseLockedForRole(user.Role, plan.Status, plan.PausedReason) {
				_, _ = s.DB.Exec(`update training_plans set status = 'archived', updated_at = now() where id = $1`, plan.ID)
				_, _ = s.ensurePlan(user.ID)
			}
		} else {
			_, _ = s.ensurePlan(user.ID)
		}
	}

	returnTo := strings.TrimSpace(r.FormValue("return_to"))
	if returnTo != "" && strings.HasPrefix(returnTo, "/") {
		http.Redirect(w, r, returnTo, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/program", http.StatusSeeOther)
}

func (s *Site) planPage(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	needsQuestionnaire := s.needsQuestionnaire(user.ID)
	setupIncomplete := needsQuestionnaire

	data := s.baseData(r, "План", "program")
	data["SetupNeedsQuestionnaire"] = needsQuestionnaire
	data["SetupIncomplete"] = setupIncomplete
	data["Programs"] = s.fetchPrograms()
	if r.URL.Query().Get("doctor") == "1" {
		data["DoctorGate"] = true
	}
	if setupIncomplete {
		s.render(w, "program", data)
		return
	}

	plan, err := s.ensurePlan(user.ID)
	if err != nil || plan == nil {
		data["PlanError"] = "Не удалось сформировать план. Проверьте анкету."
		data["PlanWorkouts"] = []planWorkoutView{}
		s.render(w, "program", data)
		return
	}

	q, _ := s.loadQuestionnaire(user.ID)
	restrictions := s.loadRestrictions(user.ID)
	var doctorApproval bool
	_ = s.DB.QueryRow(`select doctor_approval from medical_info where user_id = $1`, user.ID).Scan(&doctorApproval)

	var lastChange planChangeView
	var changedAt time.Time
	var reason string
	if err := s.DB.QueryRow(
		`select changed_at, reason from training_plan_changes where plan_id = $1 order by changed_at desc limit 1`,
		plan.ID,
	).Scan(&changedAt, &reason); err == nil {
		lastChange = planChangeView{ChangedAt: changedAt.Format("02.01.2006 15:04"), Reason: reason}
		data["PlanLastChange"] = lastChange
	}

	workouts := s.fetchPlanWorkouts(plan.ID)
	nextWorkout, _ := s.fetchNextPlanWorkout(user.ID)
	data["Plan"] = plan
	data["PlanWorkouts"] = workouts
	data["PlanCalendar"] = buildPlanCalendar(workouts)
	data["NextWorkout"] = nextWorkout
	data["PlanPaused"] = plan.Status == "paused"
	data["PlanPausedReason"] = plan.PausedReason
	data["PlanAdminLocked"] = adminPauseLockedForRole(user.Role, plan.Status, plan.PausedReason)
	data["PlanLaunchBlocked"] = planLaunchBlockedForRole(user.Role, plan.Status, plan.PausedReason)
	data["Questionnaire"] = q
	data["Restrictions"] = restrictions
	data["DoctorApproval"] = doctorApproval
	s.render(w, "program", data)
}

func (s *Site) planRegenerate(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if s.ensureOnboarding(w, r, user.ID) {
		return
	}

	if plan, err := s.getActivePlan(user.ID); err == nil && plan != nil {
		if adminPauseLockedForRole(user.Role, plan.Status, plan.PausedReason) {
			http.Redirect(w, r, "/program?paused=1", http.StatusSeeOther)
			return
		}
		_, _ = s.DB.Exec(`update training_plans set status = 'archived', updated_at = now() where id = $1`, plan.ID)
	}

	_, _ = s.ensurePlan(user.ID)
	http.Redirect(w, r, "/program", http.StatusSeeOther)
}

func (s *Site) planWorkoutStart(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if !s.requireDoctorApproval(w, r, user.ID) {
		return
	}
	planWorkoutID := chi.URLParam(r, "id")
	if planWorkoutID == "" {
		http.NotFound(w, r)
		return
	}

	var workoutID string
	var planID string
	var status string
	var sessionID sql.NullString
	err := s.DB.QueryRow(
		`select workout_id, plan_id, status, session_id from training_plan_workouts where id = $1`,
		planWorkoutID,
	).Scan(&workoutID, &planID, &status, &sessionID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if !s.planOwnedByUser(planID, user.ID) {
		http.Error(w, "Доступ запрещён", http.StatusForbidden)
		return
	}

	var planStatus string
	var pausedReason string
	_ = s.DB.QueryRow(
		`select status, coalesce(paused_reason, '') from training_plans where id = $1`,
		planID,
	).Scan(&planStatus, &pausedReason)
	if planLaunchBlockedForRole(user.Role, planStatus, pausedReason) {
		http.Redirect(w, r, "/program?paused=1", http.StatusSeeOther)
		return
	}

	if status == "completed" && sessionID.Valid {
		http.Redirect(w, r, "/sessions/"+sessionID.String, http.StatusSeeOther)
		return
	}

	if sessionID.Valid {
		http.Redirect(w, r, "/sessions/"+sessionID.String, http.StatusSeeOther)
		return
	}

	sessionIDValue, err := s.createWorkoutSession(user.ID, workoutID, planWorkoutID)
	if err != nil {
		http.Redirect(w, r, "/program", http.StatusSeeOther)
		return
	}

	_, _ = s.DB.Exec(`update training_plan_workouts set status = 'in_progress' where id = $1`, planWorkoutID)
	http.Redirect(w, r, "/sessions/"+sessionIDValue, http.StatusSeeOther)
}

func (s *Site) planWorkoutSkip(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	planWorkoutID := chi.URLParam(r, "id")
	if planWorkoutID == "" {
		http.NotFound(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/program", http.StatusSeeOther)
		return
	}

	reason := strings.TrimSpace(r.FormValue("skip_reason"))
	if reason == "" {
		reason = "Пропуск без указания причины"
	}

	var planID string
	var status string
	err := s.DB.QueryRow(`select plan_id, status from training_plan_workouts where id = $1`, planWorkoutID).Scan(&planID, &status)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if !s.planOwnedByUser(planID, user.ID) {
		http.Error(w, "Доступ запрещён", http.StatusForbidden)
		return
	}
	if status == "completed" || status == "in_progress" {
		http.Redirect(w, r, "/program", http.StatusSeeOther)
		return
	}

	before := s.planSnapshot(planID)
	_, _ = s.DB.Exec(
		`update training_plan_workouts set status = 'skipped', skip_reason = $1 where id = $2`,
		reason,
		planWorkoutID,
	)
	after := s.planSnapshot(planID)
	s.logPlanChange(user.ID, planID, "skip", "Пропуск тренировки", before, after)
	s.applyAdaptation(user.ID, planID, "skip")

	http.Redirect(w, r, "/program", http.StatusSeeOther)
}

func (s *Site) sessionFeedback(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	sessionID := chi.URLParam(r, "id")
	if sessionID == "" {
		http.NotFound(w, r)
		return
	}

	var ownerID string
	var completedAt sql.NullTime
	err := s.DB.QueryRow(
		`select user_id, completed_at from workout_sessions where id = $1`,
		sessionID,
	).Scan(&ownerID, &completedAt)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if ownerID != user.ID {
		http.Error(w, "Доступ запрещён", http.StatusForbidden)
		return
	}
	if !completedAt.Valid {
		http.Redirect(w, r, "/sessions/"+sessionID, http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/sessions/"+sessionID, http.StatusSeeOther)
		return
	}

	perceived, _ := strconv.Atoi(r.FormValue("perceived_exertion"))
	tolerance, _ := strconv.Atoi(r.FormValue("tolerance"))
	pain, _ := strconv.Atoi(r.FormValue("pain_level"))
	wellbeing, _ := strconv.Atoi(r.FormValue("wellbeing"))
	comment := strings.TrimSpace(r.FormValue("comment"))

	_, _ = s.DB.Exec(
		`insert into workout_session_feedback (session_id, user_id, perceived_exertion, tolerance, pain_level, wellbeing, comment)
     values ($1, $2, $3, $4, $5, $6, $7)
     on conflict (session_id)
     do update set perceived_exertion = excluded.perceived_exertion,
                   tolerance = excluded.tolerance,
                   pain_level = excluded.pain_level,
                   wellbeing = excluded.wellbeing,
                   comment = excluded.comment`,
		sessionID,
		user.ID,
		perceived,
		tolerance,
		pain,
		wellbeing,
		comment,
	)

	planID := s.planIDBySession(sessionID)
	if planID == "" {
		planID = s.attachSessionToPlan(user.ID, sessionID)
	}
	if planID != "" {
		s.applyAdaptation(user.ID, planID, "feedback")
	}

	http.Redirect(w, r, "/sessions/"+sessionID, http.StatusSeeOther)
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

	var rows, err = s.DB.Query(
		`select changed_at, reason
     from training_plan_changes
     where user_id = $1 and changed_at > $2
     order by changed_at desc`,
		userID,
		clearedAt,
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var changedAt time.Time
			var reason string
			_ = rows.Scan(&changedAt, &reason)
			entries = append(entries, notificationHistoryEntry{When: changedAt, Reason: reason})
		}
	}

	rows, err = s.DB.Query(
		`select rr.status, r.title, coalesce(rr.handled_at, rr.redeemed_at)
     from reward_redemptions rr
     join rewards r on r.id = rr.reward_id
     where rr.user_id = $1
       and rr.status in ('approved', 'rejected')
       and coalesce(rr.handled_at, rr.redeemed_at) > $2
     order by coalesce(rr.handled_at, rr.redeemed_at) desc`,
		userID,
		clearedAt,
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var status string
			var title string
			var when time.Time
			_ = rows.Scan(&status, &title, &when)
			reason := "Поощрение «" + title + "»"
			if status == "approved" {
				reason += " одобрено"
			} else {
				reason += " отклонено"
			}
			entries = append(entries, notificationHistoryEntry{When: when, Reason: reason})
		}
	}

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

func (s *Site) leaderboard(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.Query(
		`select u.name, coalesce(u.department, ''),
            coalesce(p.points_total, 0) as points,
            coalesce(count(ws.id), 0) as workouts,
            coalesce(sum(ws.duration_minutes), 0) as minutes,
            coalesce(max(ws.completed_at), to_timestamp(0)) as last_workout,
            coalesce(f.avg_tolerance, 0) as avg_tolerance
     from users u
     left join user_points p on p.user_id = u.id
     left join workout_sessions ws on ws.user_id = u.id and ws.completed_at is not null
     left join (
       select user_id, avg(coalesce(tolerance, 0)) as avg_tolerance
       from workout_session_feedback
       group by user_id
     ) f on f.user_id = u.id
     group by u.id, p.points_total, f.avg_tolerance
     order by points desc, workouts desc, minutes desc, u.name`,
	)
	list := []leaderboardRow{}
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var row leaderboardRow
			var last time.Time
			var avgTolerance float64
			_ = rows.Scan(&row.Name, &row.Department, &row.Points, &row.Workouts, &row.Minutes, &last, &avgTolerance)
			row.AvgTolerance = int(avgTolerance + 0.5)
			if !last.IsZero() && last.Unix() > 0 {
				row.LastWorkout = last.Format("02.01.2006")
			}
			list = append(list, row)
		}
	}

	data := s.baseData(r, "Рейтинг", "leaderboard")
	data["Leaderboard"] = list
	s.render(w, "leaderboard", data)
}

func (s *Site) achievementsPage(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	s.updateAchievements(user.ID)
	rows, err := s.DB.Query(
		`select a.title, a.description, a.icon, a.points_reward,
            coalesce(ua.unlocked, false), coalesce(ua.progress, 0), coalesce(ua.total, 0)
     from achievements a
     left join user_achievements ua on ua.achievement_id = a.id and ua.user_id = $1
     order by a.title`,
		user.ID,
	)
	views := []achievementView{}
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var v achievementView
			_ = rows.Scan(&v.Title, &v.Description, &v.Icon, &v.PointsReward, &v.Unlocked, &v.Progress, &v.Total)
			views = append(views, v)
		}
	}

	data := s.baseData(r, "Достижения", "achievements")
	data["Achievements"] = views
	s.render(w, "achievements", data)
}

func (s *Site) ensureOnboarding(w http.ResponseWriter, r *http.Request, userID string) bool {
	if s.needsQuestionnaire(userID) {
		http.Redirect(w, r, "/questionnaire", http.StatusSeeOther)
		return true
	}
	return false
}

func (s *Site) needsQuestionnaire(userID string) bool {
	q, err := s.loadQuestionnaire(userID)
	if err != nil {
		return true
	}
	if q.Goal == "" || q.FitnessLevel == "" || q.DaysPerWeek == 0 {
		return true
	}
	return false
}

func validateQuestionnaire(q questionnaireData) map[string]string {
	errors := map[string]string{}
	if q.Goal == "" {
		errors["goal"] = "Выберите цель"
	}
	if q.FitnessLevel == "" {
		errors["fitness_level"] = "Укажите уровень"
	}
	if q.DaysPerWeek < 1 || q.DaysPerWeek > 7 {
		errors["days_per_week"] = "Частота 1-7"
	}
	return errors
}

func (s *Site) loadQuestionnaire(userID string) (questionnaireData, error) {
	var raw []byte
	err := s.DB.QueryRow(`select answers from questionnaire_responses where user_id = $1`, userID).Scan(&raw)
	if err != nil {
		if err == sql.ErrNoRows {
			return questionnaireData{}, nil
		}
		return questionnaireData{}, err
	}
	if len(raw) == 0 {
		return questionnaireData{}, nil
	}
	var q questionnaireData
	_ = json.Unmarshal(raw, &q)
	return q, nil
}

func (s *Site) saveQuestionnaire(userID string, q questionnaireData) error {
	payload, err := json.Marshal(q)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(
		`insert into questionnaire_responses (user_id, answers)
     values ($1, $2)
     on conflict (user_id)
     do update set answers = excluded.answers, updated_at = now()`,
		userID,
		payload,
	)
	return err
}

func (s *Site) getActivePlan(userID string) (*planRecord, error) {
	var plan planRecord
	err := s.DB.QueryRow(
		`select id, goal, level, frequency, status, coalesce(paused_reason, ''), created_at
     from training_plans
     where user_id = $1 and status in ('active', 'paused')
     order by created_at desc
     limit 1`,
		userID,
	).Scan(&plan.ID, &plan.Goal, &plan.Level, &plan.Frequency, &plan.Status, &plan.PausedReason, &plan.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &plan, nil
}

func (s *Site) ensurePlan(userID string) (*planRecord, error) {
	plan, err := s.getActivePlan(userID)
	if err != nil {
		return nil, err
	}
	if plan != nil {
		return plan, nil
	}

	created, err := s.generatePlan(userID)
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (s *Site) generatePlan(userID string) (*planRecord, error) {
	q, err := s.loadQuestionnaire(userID)
	if err != nil {
		return nil, err
	}
	restrictions := s.loadRestrictions(userID)
	doctorApproval := s.loadDoctorApproval(userID)

	level := resolveLevel(q.FitnessLevel)
	frequency := q.DaysPerWeek
	if frequency <= 0 {
		frequency = 3
	}
	if frequency > 7 {
		frequency = 7
	}
	if !doctorApproval {
		level = "Легкая"
	}

	goalCategories := categoriesForGoal(q.Goal)
	preferenceCategories := categoriesFromPreferences(q.Preferences)
	categories := mergeCategories(goalCategories, preferenceCategories)
	availableEquipment := normalizeEquipmentSelection(q.Equipment)
	if len(availableEquipment) == 0 && !prefersNoEquipment(q.Preferences) {
		availableEquipment = []string{"Коврик"}
	}

	workouts := s.fetchWorkouts(level, categories, restrictions, availableEquipment)
	if len(workouts) == 0 && len(categories) > 0 {
		workouts = s.fetchWorkouts(level, []string{}, restrictions, availableEquipment)
	}
	targetMinutes := sessionMinutesForLevel(level)
	if !doctorApproval && targetMinutes > 30 {
		targetMinutes = 30
	}
	if filtered := filterWorkoutsByDuration(workouts, targetMinutes, 10); len(filtered) > 0 {
		workouts = filtered
	}
	rand.Seed(time.Now().UnixNano())
	workouts = shuffleWorkouts(workouts)

	planID := ""
	status := "active"
	pausedReason := ""
	if len(workouts) == 0 {
		status = "paused"
		pausedReason = "Нет подходящих тренировок по ограничениям. Нужна консультация."
	}
	err = s.DB.QueryRow(
		`insert into training_plans (user_id, goal, level, frequency, status, paused_reason)
     values ($1, $2, $3, $4, $5, $6)
     returning id`,
		userID,
		q.Goal,
		level,
		frequency,
		status,
		nullIfEmpty(pausedReason),
	).Scan(&planID)
	if err != nil {
		return nil, err
	}

	if len(workouts) == 0 {
		s.logPlanChange(userID, planID, "no_workouts", "Нет подходящих тренировок по ограничениям", nil, s.planSnapshot(planID))
		return &planRecord{
			ID:           planID,
			Goal:         q.Goal,
			Level:        level,
			Frequency:    frequency,
			Status:       status,
			PausedReason: pausedReason,
			CreatedAt:    time.Now(),
		}, nil
	}

	start := nextWeekStart(time.Now())
	weeks := 4
	offsets := weeklyOffsets(frequency)
	if len(offsets) == 0 {
		offsets = []int{0}
	}

	for week := 1; week <= weeks; week++ {
		weekList := rotateWorkouts(workouts, week-1)
		for idx, offset := range offsets {
			workout := weekList[idx%len(weekList)]
			scheduled := start.AddDate(0, 0, (week-1)*7+offset)
			_, _ = s.DB.Exec(
				`insert into training_plan_workouts (plan_id, workout_id, week, day, scheduled_date, intensity)
         values ($1, $2, $3, $4, $5, 1)`,
				planID,
				workout.ID,
				week,
				idx+1,
				scheduled,
			)
		}
	}

	s.logPlanChange(userID, planID, "initial", "Первичный подбор программы", nil, s.planSnapshot(planID))

	return &planRecord{
		ID:        planID,
		Goal:      q.Goal,
		Level:     level,
		Frequency: frequency,
		Status:    status,
		CreatedAt: time.Now(),
	}, nil
}

func (s *Site) fetchPlanWorkouts(planID string) []planWorkoutView {
	rows, err := s.DB.Query(
		`select pw.id, w.id, w.name, w.description, w.difficulty, coalesce(w.category, ''), w.duration_minutes,
            pw.week, pw.day, pw.scheduled_date, pw.intensity, pw.status, coalesce(pw.skip_reason, ''), coalesce(pw.session_id::text, '')
     from training_plan_workouts pw
     join workouts w on w.id = pw.workout_id
     where pw.plan_id = $1
     order by pw.week, pw.day`,
		planID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	list := []planWorkoutView{}
	for rows.Next() {
		var v planWorkoutView
		var date sql.NullTime
		_ = rows.Scan(&v.ID, &v.WorkoutID, &v.Name, &v.Description, &v.Difficulty, &v.Category, &v.Duration,
			&v.Week, &v.Day, &date, &v.Intensity, &v.Status, &v.SkipReason, &v.SessionID)
		if date.Valid {
			v.ScheduledDate = date.Time.Format("02.01.2006")
		}
		list = append(list, v)
	}
	return list
}

func buildPlanCalendar(items []planWorkoutView) []planCalendarWeek {
	weeks := map[int][]planCalendarDay{}
	order := []int{}
	for _, item := range items {
		if _, ok := weeks[item.Week]; !ok {
			order = append(order, item.Week)
		}
		weeks[item.Week] = append(weeks[item.Week], planCalendarDay{
			ID:            item.ID,
			WorkoutID:     item.WorkoutID,
			Name:          item.Name,
			ScheduledDate: item.ScheduledDate,
			Week:          item.Week,
			Day:           item.Day,
			Intensity:     item.Intensity,
			Status:        item.Status,
		})
	}
	sort.Ints(order)
	calendar := []planCalendarWeek{}
	for _, week := range order {
		days := weeks[week]
		sort.Slice(days, func(i, j int) bool { return days[i].Day < days[j].Day })
		calendar = append(calendar, planCalendarWeek{
			Week: week,
			Days: days,
		})
	}
	return calendar
}

func (s *Site) fetchNextPlanWorkout(userID string) (*planWorkoutView, error) {
	var v planWorkoutView
	var date sql.NullTime
	err := s.DB.QueryRow(
		`select pw.id, w.id, w.name, w.description, w.difficulty, coalesce(w.category, ''), w.duration_minutes,
            pw.week, pw.day, pw.scheduled_date, pw.intensity, pw.status, coalesce(pw.skip_reason, ''), coalesce(pw.session_id::text, '')
     from training_plan_workouts pw
     join training_plans tp on tp.id = pw.plan_id
     join workouts w on w.id = pw.workout_id
     where tp.user_id = $1 and tp.status in ('active', 'paused')
       and pw.status in ('pending', 'in_progress')
     order by pw.scheduled_date nulls last, pw.week, pw.day
     limit 1`,
		userID,
	).Scan(&v.ID, &v.WorkoutID, &v.Name, &v.Description, &v.Difficulty, &v.Category, &v.Duration,
		&v.Week, &v.Day, &date, &v.Intensity, &v.Status, &v.SkipReason, &v.SessionID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if date.Valid {
		v.ScheduledDate = date.Time.Format("02.01.2006")
	}
	return &v, nil
}

func (s *Site) fetchWorkouts(level string, categories []string, restrictions []string, equipment []string) []workoutCard {
	rows, err := s.DB.Query(
		`select w.id, w.name, w.description, w.duration_minutes, w.difficulty, coalesce(w.category, ''), count(we.exercise_id)
     from workouts w
     left join workout_exercises we on we.workout_id = w.id
     group by w.id
     order by w.created_at`,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	allowedLevels := difficultyAllowed(level)
	output := []workoutCard{}
	for rows.Next() {
		var card workoutCard
		_ = rows.Scan(&card.ID, &card.Name, &card.Description, &card.Duration, &card.Difficulty, &card.Category, &card.Exercises)
		if len(allowedLevels) > 0 && !allowedLevels[card.Difficulty] {
			continue
		}
		if len(categories) > 0 && card.Category != "" {
			matched := false
			for _, c := range categories {
				if strings.EqualFold(card.Category, c) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		if !s.workoutAllowed(card.ID, restrictions, equipment) {
			continue
		}
		output = append(output, card)
	}
	return output
}

func (s *Site) workoutAllowed(workoutID string, restrictions []string, equipment []string) bool {
	restrictedCategories := restrictionCategories(restrictions)

	rows, err := s.DB.Query(
		`select coalesce(category, ''), coalesce(array_to_string(equipment, ','), '') from exercises e
     join workout_exercises we on we.exercise_id = e.id
     where we.workout_id = $1`,
		workoutID,
	)
	if err != nil {
		return true
	}
	defer rows.Close()

	for rows.Next() {
		var category string
		var equipRaw string
		_ = rows.Scan(&category, &equipRaw)
		equip := parseCSV(equipRaw)
		if category != "" {
			if restrictedCategories[category] {
				return false
			}
		}
		if len(equip) > 0 {
			if !isSubset(equip, equipment) {
				return false
			}
		}
	}

	return true
}

func (s *Site) applyAdaptation(userID, planID, trigger string) {
	plan, err := s.getActivePlan(userID)
	if err != nil || plan == nil || plan.ID != planID {
		return
	}

	var skipped int
	_ = s.DB.QueryRow(
		`select count(*) from training_plan_workouts
     where plan_id = $1 and status = 'skipped' and scheduled_date >= current_date - interval '7 days'`,
		planID,
	).Scan(&skipped)
	if trigger == "skip" {
		if skipped < 2 {
			return
		}
		before := s.planSnapshot(planID)
		result, _ := s.DB.Exec(
			`update training_plan_workouts
       set scheduled_date = scheduled_date + interval '7 days'
       where plan_id = $1 and status = 'pending' and scheduled_date is not null`,
			planID,
		)
		if affectedRows(result) == 0 {
			return
		}
		after := s.planSnapshot(planID)
		s.logPlanChange(userID, planID, "missed", "Есть пропуски: план перераспределён", before, after)
		return
	}
	if trigger != "feedback" {
		return
	}

	type feedbackSnapshot struct {
		Pain      int
		Tolerance int
		Exertion  int
		Wellbeing int
	}

	samples := 0
	sumPain := 0
	sumTolerance := 0
	sumExertion := 0
	sumWellbeing := 0
	last := feedbackSnapshot{}

	rows, err := s.DB.Query(
		`select coalesce(pain_level, 0), coalesce(tolerance, 0), coalesce(perceived_exertion, 0), coalesce(wellbeing, 0)
     from workout_session_feedback f
     join workout_sessions ws on ws.id = f.session_id
     join training_plan_workouts tpw on tpw.id = ws.plan_workout_id
     where f.user_id = $1 and tpw.plan_id = $2
     order by ws.completed_at desc nulls last, f.created_at desc
     limit 4`,
		userID,
		planID,
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var pain, tolerance, exertion, wellbeing int
			_ = rows.Scan(&pain, &tolerance, &exertion, &wellbeing)
			if samples == 0 {
				last = feedbackSnapshot{Pain: pain, Tolerance: tolerance, Exertion: exertion, Wellbeing: wellbeing}
			}
			samples++
			sumPain += pain
			sumTolerance += tolerance
			sumExertion += exertion
			sumWellbeing += wellbeing
		}
	}

	if samples == 0 {
		return
	}

	avgPain := float64(sumPain) / float64(samples)
	avgTolerance := float64(sumTolerance) / float64(samples)
	avgExertion := float64(sumExertion) / float64(samples)
	avgWellbeing := float64(sumWellbeing) / float64(samples)

	reasonCode := ""
	reason := ""
	updateQuery := ""
	lastExcellent := last.Tolerance >= 4 && last.Wellbeing >= 4 && last.Pain <= 1 && last.Exertion <= 3
	lastCritical := last.Pain >= 4 || last.Tolerance <= 2 || last.Wellbeing <= 2 || last.Exertion >= 5
	avgWarning := avgPain >= 3.5 || (avgPain >= 3 && avgWellbeing <= 2.5)
	avgRegression := avgTolerance <= 2.5 || avgWellbeing <= 2.5 || (avgExertion >= 4 && avgTolerance <= 3.2)

	if lastExcellent && skipped == 0 && avgPain <= 3 && avgExertion <= 4.5 {
		reasonCode = "progression"
		reason = "Хорошая переносимость: повышена интенсивность"
		updateQuery = `update training_plan_workouts
                   set intensity = intensity + 1
                   where plan_id = $1 and status = 'pending' and intensity < 3`
	} else if last.Pain >= 4 || (!lastExcellent && avgWarning) {
		reasonCode = "warning"
		reason = "Отмечен дискомфорт: снижена интенсивность и рекомендована консультация"
		updateQuery = `update training_plan_workouts
                   set intensity = intensity - 1
                   where plan_id = $1 and status = 'pending' and intensity > 1`
	} else if lastCritical || (!lastExcellent && avgRegression) {
		reasonCode = "regression"
		reason = "Низкая переносимость нагрузки: снижена интенсивность"
		updateQuery = `update training_plan_workouts
                   set intensity = intensity - 1
                   where plan_id = $1 and status = 'pending' and intensity > 1`
	}

	if reasonCode == "" {
		return
	}
	before := s.planSnapshot(planID)
	var pendingCount int
	var minIntensity int
	var maxIntensity int
	_ = s.DB.QueryRow(
		`select count(*), coalesce(min(intensity), 0), coalesce(max(intensity), 0)
     from training_plan_workouts
     where plan_id = $1 and status = 'pending'`,
		planID,
	).Scan(&pendingCount, &minIntensity, &maxIntensity)

	result, _ := s.DB.Exec(updateQuery, planID)
	if affectedRows(result) == 0 {
		noEffectReason := reason
		if pendingCount == 0 {
			noEffectReason = "Оценка сохранена: в плане нет будущих тренировок для корректировки"
		} else if reasonCode == "progression" && maxIntensity >= 3 {
			noEffectReason = "Хорошая переносимость: интенсивность уже максимальная"
		} else if (reasonCode == "regression" || reasonCode == "warning") && minIntensity <= 1 {
			if reasonCode == "warning" {
				noEffectReason = "Отмечен дискомфорт: интенсивность уже минимальная, рекомендована консультация"
			} else {
				noEffectReason = "Низкая переносимость нагрузки: интенсивность уже минимальная"
			}
		} else {
			noEffectReason = reason + " (изменения не требуются)"
		}
		s.logPlanChange(userID, planID, reasonCode, noEffectReason, before, before)
		return
	}
	after := s.planSnapshot(planID)
	s.logPlanChange(userID, planID, reasonCode, reason, before, after)
}

func affectedRows(result sql.Result) int64 {
	if result == nil {
		return 0
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0
	}
	return rows
}

func (s *Site) planSnapshot(planID string) json.RawMessage {
	rows, err := s.DB.Query(
		`select workout_id, week, day, intensity, status
     from training_plan_workouts
     where plan_id = $1
     order by week, day`,
		planID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	type snapshotItem struct {
		WorkoutID string `json:"workout_id"`
		Week      int    `json:"week"`
		Day       int    `json:"day"`
		Intensity int    `json:"intensity"`
		Status    string `json:"status"`
	}

	items := []snapshotItem{}
	for rows.Next() {
		var item snapshotItem
		_ = rows.Scan(&item.WorkoutID, &item.Week, &item.Day, &item.Intensity, &item.Status)
		items = append(items, item)
	}

	payload, _ := json.Marshal(items)
	return payload
}

func (s *Site) logPlanChange(userID, planID, code, reason string, before, after json.RawMessage) {
	_, _ = s.DB.Exec(
		`insert into training_plan_changes (plan_id, user_id, reason_code, reason, before_plan, after_plan)
     values ($1, $2, $3, $4, $5, $6)`,
		planID,
		userID,
		code,
		reason,
		before,
		after,
	)
}

func resolveLevel(fitness string) string {
	level := strings.TrimSpace(strings.ToLower(fitness))
	switch level {
	case "низкий", "легкая", "легкий":
		level = "Легкая"
	case "средний", "средняя":
		level = "Средняя"
	case "высокий", "продвинутая":
		level = "Продвинутая"
	default:
		level = "Легкая"
	}

	return level
}

func sessionMinutesForLevel(level string) int {
	switch level {
	case "Средняя":
		return 30
	case "Продвинутая":
		return 40
	default:
		return 20
	}
}

func categoriesForGoal(goal string) []string {
	switch strings.ToLower(goal) {
	case "восстановление":
		return []string{"Реабилитация"}
	case "подвижность", "мобилизация":
		return []string{"Мобилизация", "Растяжка"}
	case "сила":
		return []string{"Кор", "Спина", "Ноги"}
	case "выносливость":
		return []string{"Кардио"}
	default:
		return []string{}
	}
}

func categoriesFromPreferences(preferences string) []string {
	text := strings.ToLower(preferences)
	mapping := map[string]string{
		"растяж": "Растяжка",
		"мобил":  "Мобилизация",
		"кардио": "Кардио",
		"спина":  "Спина",
		"кор":    "Кор",
		"ног":    "Ноги",
		"плеч":   "Плечи",
		"баланс": "Кор",
		"осанк":  "Спина",
		"стабил": "Кор",
	}
	out := []string{}
	seen := map[string]bool{}
	for _, item := range parseCSV(preferences) {
		cleaned := strings.TrimSpace(item)
		if cleaned == "" {
			continue
		}
		key := strings.ToLower(cleaned)
		if category, ok := mapping[key]; ok {
			if !seen[category] {
				seen[category] = true
				out = append(out, category)
			}
			continue
		}
		for matchKey, category := range mapping {
			if strings.Contains(key, matchKey) && !seen[category] {
				seen[category] = true
				out = append(out, category)
			}
		}
	}
	for key, category := range mapping {
		if strings.Contains(text, key) && !seen[category] {
			seen[category] = true
			out = append(out, category)
		}
	}
	return out
}

func prefersNoEquipment(preferences string) bool {
	text := strings.ToLower(preferences)
	if strings.Contains(text, "без инвентар") || strings.Contains(text, "без оборудования") || strings.Contains(text, "без снар") {
		return true
	}
	return false
}

func mergeCategories(sets ...[]string) []string {
	seen := map[string]bool{}
	merged := []string{}
	for _, list := range sets {
		for _, item := range list {
			if item == "" {
				continue
			}
			key := strings.ToLower(item)
			if seen[key] {
				continue
			}
			seen[key] = true
			merged = append(merged, item)
		}
	}
	return merged
}

func questionnaireChanged(prev, next questionnaireData, prevRestrictions, nextRestrictions []string) bool {
	if prev.Goal != next.Goal || prev.FitnessLevel != next.FitnessLevel || prev.DaysPerWeek != next.DaysPerWeek {
		return true
	}
	if prev.Preferences != next.Preferences {
		return true
	}
	if !sameStringSet(prev.Equipment, next.Equipment) {
		return true
	}
	if !sameStringSet(prevRestrictions, nextRestrictions) {
		return true
	}
	return false
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, item := range a {
		seen[strings.ToLower(strings.TrimSpace(item))]++
	}
	for _, item := range b {
		key := strings.ToLower(strings.TrimSpace(item))
		if seen[key] == 0 {
			return false
		}
		seen[key]--
	}
	for _, v := range seen {
		if v != 0 {
			return false
		}
	}
	return true
}

func filterWorkoutsByDuration(list []workoutCard, target, tolerance int) []workoutCard {
	if target <= 0 || tolerance < 0 {
		return list
	}
	filtered := make([]workoutCard, 0, len(list))
	for _, item := range list {
		if item.Duration == 0 {
			continue
		}
		delta := item.Duration - target
		if delta < 0 {
			delta = -delta
		}
		if delta <= tolerance {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func shuffleWorkouts(list []workoutCard) []workoutCard {
	if len(list) == 0 {
		return list
	}
	out := make([]workoutCard, len(list))
	copy(out, list)
	rand.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

func rotateWorkouts(list []workoutCard, offset int) []workoutCard {
	if len(list) == 0 {
		return list
	}
	shift := offset % len(list)
	if shift == 0 {
		return list
	}
	out := append([]workoutCard{}, list[shift:]...)
	out = append(out, list[:shift]...)
	return out
}

func difficultyAllowed(level string) map[string]bool {
	allowed := map[string]bool{}
	switch level {
	case "Средняя":
		allowed["Легкая"] = true
		allowed["Средняя"] = true
	case "Продвинутая":
		allowed["Легкая"] = true
		allowed["Средняя"] = true
		allowed["Сложная"] = true
	default:
		allowed["Легкая"] = true
	}
	return allowed
}

func restrictionOptions() []string {
	return []string{"Колени", "Спина", "Плечи", "Сердце", "Растяжка"}
}

func equipmentOptions() []string {
	return []string{
		"Без инвентаря",
		"Коврик",
		"Резинка",
		"Гантели",
		"Стул",
		"Фитбол",
	}
}

func preferenceOptions() []string {
	return []string{
		"Растяжка",
		"Мобилизация",
		"Кардио",
		"Кор",
		"Спина",
		"Ноги",
		"Плечи",
	}
}

func normalizeEquipmentSelection(list []string) []string {
	allowed := map[string]string{}
	for _, item := range equipmentOptions() {
		allowed[strings.ToLower(strings.TrimSpace(item))] = item
	}

	cleaned := []string{}
	seen := map[string]bool{}
	for _, item := range list {
		key := strings.ToLower(strings.TrimSpace(item))
		if key == "" {
			continue
		}
		value, ok := allowed[key]
		if !ok {
			continue
		}
		lower := strings.ToLower(value)
		if strings.Contains(lower, "без инвентар") || strings.Contains(lower, "без оборуд") {
			return []string{}
		}
		if seen[lower] {
			continue
		}
		seen[lower] = true
		cleaned = append(cleaned, value)
	}
	return cleaned
}

func normalizeSelection(list []string, options []string) []string {
	allowed := map[string]string{}
	for _, item := range options {
		allowed[strings.ToLower(strings.TrimSpace(item))] = item
	}

	cleaned := []string{}
	seen := map[string]bool{}
	for _, item := range list {
		key := strings.ToLower(strings.TrimSpace(item))
		if key == "" {
			continue
		}
		value, ok := allowed[key]
		if !ok {
			continue
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		cleaned = append(cleaned, value)
	}
	return cleaned
}

func normalizePreferenceSelection(list []string) []string {
	return normalizeSelection(list, preferenceOptions())
}

func normalizeRestrictionSelection(list []string) []string {
	return normalizeSelection(list, restrictionOptions())
}

func isAdminPauseReason(reason string) bool {
	normalized := strings.ToLower(strings.TrimSpace(reason))
	return strings.Contains(normalized, "администратор")
}

func adminPauseLockedForRole(role, status, pausedReason string) bool {
	if status != "paused" {
		return false
	}
	if !isAdminPauseReason(pausedReason) {
		return false
	}
	return strings.ToLower(strings.TrimSpace(role)) != "admin"
}

func planLaunchBlockedForRole(role, status, pausedReason string) bool {
	if status != "paused" {
		return false
	}
	return !(strings.ToLower(strings.TrimSpace(role)) == "admin" && isAdminPauseReason(pausedReason))
}

func weeklyOffsets(frequency int) []int {
	switch frequency {
	case 1:
		return []int{2}
	case 2:
		return []int{1, 4}
	case 3:
		return []int{0, 2, 4}
	case 4:
		return []int{0, 2, 4, 6}
	case 5:
		return []int{0, 1, 3, 5, 6}
	case 6:
		return []int{0, 1, 2, 4, 5, 6}
	case 7:
		return []int{0, 1, 2, 3, 4, 5, 6}
	default:
		if frequency <= 0 {
			return []int{}
		}
		offsets := []int{}
		step := float64(7) / float64(frequency)
		used := map[int]bool{}
		for i := 0; i < frequency; i++ {
			raw := int(math.Round(float64(i) * step))
			if raw < 0 {
				raw = 0
			}
			if raw > 6 {
				raw = 6
			}
			for used[raw] && raw < 6 {
				raw++
			}
			used[raw] = true
			offsets = append(offsets, raw)
		}
		sort.Ints(offsets)
		return offsets
	}
}

func restrictionCategories(restrictions []string) map[string]bool {
	out := map[string]bool{}
	mapping := map[string][]string{
		"Колени":   {"Ноги"},
		"Спина":    {"Спина"},
		"Плечи":    {"Плечи"},
		"Сердце":   {"Кардио"},
		"Растяжка": {"Растяжка"},
	}
	for _, r := range restrictions {
		if cats, ok := mapping[r]; ok {
			for _, c := range cats {
				out[c] = true
			}
		}
	}
	return out
}

func (s *Site) loadRestrictions(userID string) []string {
	var raw string
	_ = s.DB.QueryRow(
		`select coalesce(array_to_string(restrictions, ','), '')
     from medical_info
     where user_id = $1`,
		userID,
	).Scan(&raw)
	return parseCSV(raw)
}

func (s *Site) loadDoctorApproval(userID string) bool {
	var approval bool
	_ = s.DB.QueryRow(`select doctor_approval from medical_info where user_id = $1`, userID).Scan(&approval)
	return approval
}

func isSubset(need, have []string) bool {
	allowed := map[string]bool{}
	for _, item := range have {
		allowed[strings.ToLower(item)] = true
	}
	for _, item := range need {
		if !allowed[strings.ToLower(item)] {
			return false
		}
	}
	return true
}

func nextWeekStart(now time.Time) time.Time {
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	daysUntilMonday := 8 - weekday
	if daysUntilMonday == 0 {
		daysUntilMonday = 7
	}
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, daysUntilMonday)
	return start
}

func (s *Site) planOwnedByUser(planID, userID string) bool {
	var owner string
	err := s.DB.QueryRow(`select user_id from training_plans where id = $1`, planID).Scan(&owner)
	return err == nil && owner == userID
}

func (s *Site) planIDBySession(sessionID string) string {
	var planID string
	_ = s.DB.QueryRow(
		`select tpw.plan_id
     from workout_sessions ws
     join training_plan_workouts tpw on tpw.id = ws.plan_workout_id
     where ws.id = $1`,
		sessionID,
	).Scan(&planID)
	return planID
}

func (s *Site) attachSessionToPlan(userID, sessionID string) string {
	var workoutID string
	if err := s.DB.QueryRow(
		`select workout_id
     from workout_sessions
     where id = $1 and user_id = $2`,
		sessionID,
		userID,
	).Scan(&workoutID); err != nil {
		return ""
	}

	var planWorkoutID string
	var planID string
	var linkedSessionID string
	err := s.DB.QueryRow(
		`select pw.id, pw.plan_id, coalesce(pw.session_id::text, '')
     from training_plan_workouts pw
     join training_plans tp on tp.id = pw.plan_id
     where tp.user_id = $1
       and tp.status in ('active', 'paused')
       and pw.workout_id = $2
       and pw.status in ('pending', 'in_progress')
     order by case when pw.status = 'in_progress' then 0 else 1 end,
              pw.scheduled_date nulls last, pw.week, pw.day
     limit 1`,
		userID,
		workoutID,
	).Scan(&planWorkoutID, &planID, &linkedSessionID)
	if err != nil {
		return ""
	}

	if linkedSessionID != "" && linkedSessionID != sessionID {
		return ""
	}

	_, _ = s.DB.Exec(
		`update workout_sessions
     set plan_workout_id = $1
     where id = $2`,
		planWorkoutID,
		sessionID,
	)
	_, _ = s.DB.Exec(
		`update training_plan_workouts
     set session_id = $1,
         status = 'completed'
     where id = $2`,
		sessionID,
		planWorkoutID,
	)
	return planID
}

func (s *Site) updateAchievements(userID string) {
	rows, err := s.DB.Query(`select id, title, points_reward, coalesce(metric, ''), coalesce(target, 0) from achievements`)
	if err != nil {
		return
	}
	defer rows.Close()

	type ach struct {
		ID           string
		Title        string
		PointsReward int
		Metric       string
		Target       int
	}
	list := []ach{}
	for rows.Next() {
		var a ach
		_ = rows.Scan(&a.ID, &a.Title, &a.PointsReward, &a.Metric, &a.Target)
		list = append(list, a)
	}

	existing := map[string]bool{}
	exRows, err := s.DB.Query(`select achievement_id, unlocked from user_achievements where user_id = $1`, userID)
	if err == nil {
		defer exRows.Close()
		for exRows.Next() {
			var id string
			var unlocked bool
			_ = exRows.Scan(&id, &unlocked)
			existing[id] = unlocked
		}
	}

	var total int
	_ = s.DB.QueryRow(`select count(*) from workout_sessions where user_id = $1 and completed_at is not null`, userID).Scan(&total)
	var last30 int
	_ = s.DB.QueryRow(`select count(*) from workout_sessions where user_id = $1 and completed_at >= now() - interval '30 days'`, userID).Scan(&last30)

	streak := s.computeStreak(userID)

	for _, a := range list {
		progress := 0
		target := a.Target
		metric := strings.ToLower(strings.TrimSpace(a.Metric))
		if target <= 0 {
			target = 1
		}
		switch metric {
		case "total":
			progress = total
		case "streak":
			progress = streak
		case "month":
			progress = last30
		default:
			switch a.Title {
			case "Первый шаг":
				progress = total
				target = 1
			case "Первые три":
				progress = total
				target = 3
			case "Пять тренировок":
				progress = total
				target = 5
			case "Серия":
				progress = streak
				target = 5
			case "Неделя подряд":
				progress = streak
				target = 7
			case "Железная воля":
				progress = streak
				target = 10
			case "Активные 2 недели":
				progress = streak
				target = 14
			case "Настойчивость":
				progress = last30
				target = 10
			case "12 тренировок за месяц":
				progress = last30
				target = 12
			case "Регулярность":
				progress = last30
				target = 8
			case "Месяц активности":
				progress = last30
				target = 20
			case "15 тренировок":
				progress = total
				target = 15
			case "Марафон":
				progress = total
				target = 25
			default:
				progress = total
				target = 1
			}
		}

		unlocked := progress >= target
		if progress > target {
			progress = target
		}

		_, _ = s.DB.Exec(
			`insert into user_achievements (user_id, achievement_id, unlocked, unlocked_at, progress, total)
       values ($1, $2, $3, case when $3 then now() else null end, $4, $5)
       on conflict (user_id, achievement_id)
       do update set unlocked = excluded.unlocked,
                     unlocked_at = case when excluded.unlocked then now() else user_achievements.unlocked_at end,
                     progress = excluded.progress,
                     total = excluded.total`,
			userID,
			a.ID,
			unlocked,
			progress,
			target,
		)

		if unlocked && !existing[a.ID] && a.PointsReward > 0 {
			_, _ = s.DB.Exec(
				`insert into incentive_awards (user_id, points, reason)
         values ($1, $2, $3)`,
				userID,
				a.PointsReward,
				"Достижение: "+a.Title,
			)
			_, _ = s.DB.Exec(
				`insert into user_points (user_id, points_balance, points_total)
         values ($1, $2, $2)
         on conflict (user_id)
         do update set points_balance = user_points.points_balance + $2,
                       points_total = user_points.points_total + $2`,
				userID,
				a.PointsReward,
			)
		}
	}
}

func (s *Site) computeStreak(userID string) int {
	rows, err := s.DB.Query(
		`select completed_at from workout_sessions
     where user_id = $1 and completed_at is not null
     order by completed_at desc`,
		userID,
	)
	if err != nil {
		return 0
	}
	defer rows.Close()

	streak := 0
	var last time.Time
	for rows.Next() {
		var completed time.Time
		_ = rows.Scan(&completed)
		if streak == 0 {
			streak = 1
			last = completed
			continue
		}
		if completed.After(last.AddDate(0, 0, -2)) {
			streak++
			last = completed
			continue
		}
		break
	}
	return streak
}

func (s *Site) createWorkoutSession(userID, workoutID, planWorkoutID string) (string, error) {
	_ = s.ensureWorkoutExercises(workoutID)
	var exercisesCount int
	_ = s.DB.QueryRow("select count(*) from workout_exercises where workout_id = $1", workoutID).Scan(&exercisesCount)

	var sessionID string
	err := s.DB.QueryRow(
		`insert into workout_sessions (user_id, workout_id, total_exercises, completed_exercises, plan_workout_id)
     values ($1, $2, $3, 0, $4)
     returning id`,
		userID,
		workoutID,
		exercisesCount,
		nullIfEmpty(planWorkoutID),
	).Scan(&sessionID)
	if err != nil {
		return "", err
	}

	rows, err := s.DB.Query(
		`select exercise_id, sort_order from workout_exercises where workout_id = $1 order by sort_order`,
		workoutID,
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var exerciseID string
			var order int
			_ = rows.Scan(&exerciseID, &order)
			_, _ = s.DB.Exec(
				`insert into workout_session_exercises (session_id, exercise_id, sort_order)
         values ($1, $2, $3)`,
				sessionID,
				exerciseID,
				order,
			)
		}
	}

	if planWorkoutID != "" {
		_, _ = s.DB.Exec(`update training_plan_workouts set session_id = $1 where id = $2`, sessionID, planWorkoutID)
	}

	return sessionID, nil
}
