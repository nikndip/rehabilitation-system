package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

type seedUser struct {
	Name           string
	EmployeeID     string
	CorporateEmail string
	Role           string
	Department     string
	Password       string
}

func Seed(db *sql.DB) error {
	users := []seedUser{
		{
			Name:           "Иван Петров",
			EmployeeID:     "10001",
			CorporateEmail: "ivan.petrov@company.local",
			Role:           "employee",
			Department:     "Инженерный отдел",
			Password:       "password",
		},
		{
			Name:           "Мария Соколова",
			EmployeeID:     "20001",
			CorporateEmail: "m.sokolova@company.local",
			Role:           "employee",
			Department:     "Служба сопровождения",
			Password:       "password",
		},
		{
			Name:           "Администратор",
			EmployeeID:     "90000",
			CorporateEmail: "admin@company.local",
			Role:           "admin",
			Department:     "ИТ",
			Password:       "password",
		},
	}

	for _, user := range users {
		id, err := ensureUser(db, user)
		if err != nil {
			return err
		}
		if err := EnsureUserDefaults(db, id); err != nil {
			return err
		}
	}

	return nil
}

func ensureUser(db *sql.DB, user seedUser) (string, error) {
	var id string
	err := db.QueryRow("select id from users where employee_id = $1", user.EmployeeID).Scan(&id)
	if err == nil {
		if strings.TrimSpace(user.CorporateEmail) != "" {
			_, _ = db.Exec(
				`update users
				 set corporate_email = coalesce(nullif(corporate_email, ''), $1),
				     updated_at = now()
				 where id = $2`,
				strings.TrimSpace(user.CorporateEmail),
				id,
			)
		}
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("lookup user: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}

	err = db.QueryRow(
		`insert into users (name, employee_id, corporate_email, password_hash, role, department)
     values ($1, $2, nullif($3, ''), $4, $5, $6)
     returning id`,
		user.Name,
		user.EmployeeID,
		strings.TrimSpace(user.CorporateEmail),
		string(hash),
		user.Role,
		user.Department,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("insert user: %w", err)
	}

	return id, nil
}

func EnsureUserDefaults(db *sql.DB, userID string) error {
	_, _ = db.Exec("insert into user_profiles (user_id) values ($1) on conflict do nothing", userID)
	_, _ = db.Exec("insert into user_points (user_id) values ($1) on conflict do nothing", userID)
	return nil
}
