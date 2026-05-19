package middleware

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"
)

type csrfContextKey string

const csrfTokenKey csrfContextKey = "csrf_token"

type CSRFManager struct {
	CookieName string
	Secure     bool
}

func (m *CSRFManager) Protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookieName := strings.TrimSpace(m.CookieName)
		if cookieName == "" {
			cookieName = "csrf_token"
		}

		token := ""
		cookie, err := r.Cookie(cookieName)
		if err == nil {
			token = strings.TrimSpace(cookie.Value)
		}
		if !isCSRFTokenValid(token) {
			token, err = generateCSRFToken(32)
			if err != nil {
				http.Error(w, "csrf token error", http.StatusInternalServerError)
				return
			}
			http.SetCookie(w, &http.Cookie{
				Name:     cookieName,
				Value:    token,
				Path:     "/",
				HttpOnly: true,
				Secure:   m.Secure,
				SameSite: http.SameSiteLaxMode,
			})
		}

		if isUnsafeMethod(r.Method) {
			submitted := strings.TrimSpace(r.Header.Get("X-CSRF-Token"))
			if submitted == "" {
				_ = r.ParseForm()
				submitted = strings.TrimSpace(r.PostFormValue("_csrf"))
			}
			if !csrfTokenMatches(token, submitted) {
				http.Error(w, "invalid csrf token", http.StatusForbidden)
				return
			}
		}

		ctx := WithCSRFToken(r.Context(), token)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func WithCSRFToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, csrfTokenKey, token)
}

func CSRFTokenFromContext(ctx context.Context) string {
	token, _ := ctx.Value(csrfTokenKey).(string)
	return token
}

func isUnsafeMethod(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func isCSRFTokenValid(token string) bool {
	return strings.TrimSpace(token) != ""
}

func csrfTokenMatches(expected, submitted string) bool {
	expected = strings.TrimSpace(expected)
	submitted = strings.TrimSpace(submitted)
	if expected == "" || submitted == "" {
		return false
	}
	expectedBytes := []byte(expected)
	submittedBytes := []byte(submitted)
	if len(expectedBytes) != len(submittedBytes) {
		return false
	}
	return subtle.ConstantTimeCompare(expectedBytes, submittedBytes) == 1
}

func generateCSRFToken(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
