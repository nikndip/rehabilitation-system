package middleware

import (
  "database/sql"
  "net/http"
  "time"

  "rehab-app/internal/models"
)

type SessionManager struct {
  DB         *sql.DB
  CookieName string
  SessionTTL time.Duration
  Secure     bool
}

func (s *SessionManager) Load(next http.Handler) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    cookie, err := r.Cookie(s.CookieName)
    if err != nil || cookie.Value == "" {
      next.ServeHTTP(w, r)
      return
    }

    var user models.User
    err = s.DB.QueryRow(
      `select u.id, u.name, u.employee_id, coalesce(u.corporate_email, ''), u.role, coalesce(u.department, ''), coalesce(u.position, '')
       from sessions s
       join users u on u.id = s.user_id
       where s.token = $1 and s.expires_at > now()`,
      cookie.Value,
    ).Scan(&user.ID, &user.Name, &user.EmployeeID, &user.CorporateEmail, &user.Role, &user.Department, &user.Position)
    if err != nil {
      next.ServeHTTP(w, r)
      return
    }

    ctx := WithUser(r.Context(), &user)
    next.ServeHTTP(w, r.WithContext(ctx))
  })
}

func (s *SessionManager) RequireAuth(next http.Handler) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    if UserFromContext(r.Context()) == nil {
      http.Redirect(w, r, "/login", http.StatusSeeOther)
      return
    }
    next.ServeHTTP(w, r)
  })
}

func (s *SessionManager) RequireRole(role string, next http.Handler) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    user := UserFromContext(r.Context())
    if user == nil || user.Role != role {
      http.Error(w, "forbidden", http.StatusForbidden)
      return
    }
    next.ServeHTTP(w, r)
  })
}
