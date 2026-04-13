package site_test

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"rehab-app/internal/config"
	dbpkg "rehab-app/internal/db"
	appmiddleware "rehab-app/internal/middleware"
	"rehab-app/internal/site"
	"rehab-app/internal/web"
)

func TestFunctionalScenarios(t *testing.T) {
	baseURL := buildFunctionalServer(t)

	t.Run("public login page is available", func(t *testing.T) {
		client := newFunctionalClient(t)
		resp, body := mustRequest(t, client, http.MethodGet, baseURL+"/login", nil)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
		}
		if !strings.Contains(body, "<title>Вход") {
			t.Fatalf("expected login page title in response body")
		}
	})

	t.Run("unauthorized user is redirected to login", func(t *testing.T) {
		client := newFunctionalClient(t)
		resp, _ := mustRequest(t, client, http.MethodGet, baseURL+"/", nil)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("expected status %d, got %d", http.StatusSeeOther, resp.StatusCode)
		}
		if location := resp.Header.Get("Location"); location != "/login" {
			t.Fatalf("expected redirect to /login, got %q", location)
		}
	})

	t.Run("login with valid employee credentials succeeds", func(t *testing.T) {
		client := newFunctionalClient(t)
		resp, _ := mustFormRequest(t, client, http.MethodPost, baseURL+"/login", url.Values{
			"employee_id": {"10001"},
			"password":    {"password"},
		})
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("expected status %d, got %d", http.StatusSeeOther, resp.StatusCode)
		}
		if location := resp.Header.Get("Location"); location != "/" {
			t.Fatalf("expected redirect to /, got %q", location)
		}

		resp, body := mustRequest(t, client, http.MethodGet, baseURL+"/", nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status %d after login, got %d", http.StatusOK, resp.StatusCode)
		}
		if !strings.Contains(body, "<title>Питание") {
			t.Fatalf("expected dashboard page after login")
		}
	})

	t.Run("login with invalid password is rejected", func(t *testing.T) {
		client := newFunctionalClient(t)
		resp, _ := mustFormRequest(t, client, http.MethodPost, baseURL+"/login", url.Values{
			"employee_id": {"10001"},
			"password":    {"wrong-password"},
		})
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("expected status %d, got %d", http.StatusSeeOther, resp.StatusCode)
		}
		location := resp.Header.Get("Location")
		if !strings.HasPrefix(location, "/login?error=") {
			t.Fatalf("expected redirect with error to /login, got %q", location)
		}
	})

	t.Run("employee cannot access admin section", func(t *testing.T) {
		client := newFunctionalClient(t)
		_, _ = mustFormRequest(t, client, http.MethodPost, baseURL+"/login", url.Values{
			"employee_id": {"10001"},
			"password":    {"password"},
		})

		resp, body := mustRequest(t, client, http.MethodGet, baseURL+"/admin", nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("expected status %d, got %d", http.StatusForbidden, resp.StatusCode)
		}
		if !strings.Contains(body, "Доступ запрещён") {
			t.Fatalf("expected forbidden message in response body")
		}
	})

	t.Run("manager can access manager section", func(t *testing.T) {
		client := newFunctionalClient(t)
		_, _ = mustFormRequest(t, client, http.MethodPost, baseURL+"/login", url.Values{
			"employee_id": {"30001"},
			"password":    {"password"},
		})

		resp, _ := mustRequest(t, client, http.MethodGet, baseURL+"/manager", nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("expected status %d, got %d", http.StatusSeeOther, resp.StatusCode)
		}
		if location := resp.Header.Get("Location"); location != "/manager/nutrition" {
			t.Fatalf("expected redirect to /manager/nutrition, got %q", location)
		}

		resp, body := mustRequest(t, client, http.MethodGet, baseURL+"/manager/nutrition", nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
		}
		if !strings.Contains(body, "<title>Руководитель: модуль питания") {
			t.Fatalf("expected manager page title in response body")
		}
	})

	t.Run("admin can access admin section", func(t *testing.T) {
		client := newFunctionalClient(t)
		_, _ = mustFormRequest(t, client, http.MethodPost, baseURL+"/login", url.Values{
			"employee_id": {"90000"},
			"password":    {"password"},
		})

		resp, _ := mustRequest(t, client, http.MethodGet, baseURL+"/admin", nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("expected status %d, got %d", http.StatusSeeOther, resp.StatusCode)
		}
		if location := resp.Header.Get("Location"); location != "/admin/nutrition" {
			t.Fatalf("expected redirect to /admin/nutrition, got %q", location)
		}

		resp, body := mustRequest(t, client, http.MethodGet, baseURL+"/admin/nutrition", nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
		}
		if !strings.Contains(body, "<title>Администрирование питания") {
			t.Fatalf("expected admin page title in response body")
		}
	})

	t.Run("admin can add custom meal and employee can see it", func(t *testing.T) {
		adminClient := newFunctionalClient(t)
		_, _ = mustFormRequest(t, adminClient, http.MethodPost, baseURL+"/login", url.Values{
			"employee_id": {"90000"},
			"password":    {"password"},
		})

		mealName := "Тестовое блюдо функционального теста"
		resp, _ := mustFormRequest(t, adminClient, http.MethodPost, baseURL+"/admin/nutrition/meals", url.Values{
			"name":        {mealName},
			"description": {"Создано автоматическим функциональным тестом."},
			"slot":        {"lunch"},
			"calories":    {"420"},
			"protein":     {"30"},
			"carbs":       {"40"},
			"fats":        {"12"},
		})
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("expected status %d, got %d", http.StatusSeeOther, resp.StatusCode)
		}
		if location := resp.Header.Get("Location"); !strings.HasPrefix(location, "/admin/nutrition?success=") {
			t.Fatalf("expected redirect with success to /admin/nutrition, got %q", location)
		}

		employeeClient := newFunctionalClient(t)
		_, _ = mustFormRequest(t, employeeClient, http.MethodPost, baseURL+"/login", url.Values{
			"employee_id": {"10001"},
			"password":    {"password"},
		})

		resp, body := mustRequest(t, employeeClient, http.MethodGet, baseURL+"/nutrition/meals?q=%D0%A2%D0%B5%D1%81%D1%82%D0%BE%D0%B2%D0%BE%D0%B5", nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
		}
		if !strings.Contains(body, mealName) {
			t.Fatalf("expected custom meal %q in meals page", mealName)
		}
	})

	t.Run("logout clears session", func(t *testing.T) {
		client := newFunctionalClient(t)
		_, _ = mustFormRequest(t, client, http.MethodPost, baseURL+"/login", url.Values{
			"employee_id": {"10001"},
			"password":    {"password"},
		})

		resp, _ := mustRequest(t, client, http.MethodPost, baseURL+"/logout", nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("expected status %d, got %d", http.StatusSeeOther, resp.StatusCode)
		}
		if location := resp.Header.Get("Location"); location != "/login" {
			t.Fatalf("expected redirect to /login after logout, got %q", location)
		}

		resp, _ = mustRequest(t, client, http.MethodGet, baseURL+"/", nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("expected status %d for unauthorized request, got %d", http.StatusSeeOther, resp.StatusCode)
		}
		if location := resp.Header.Get("Location"); location != "/login" {
			t.Fatalf("expected redirect to /login after logout, got %q", location)
		}
	})
}

func buildFunctionalServer(t *testing.T) string {
	t.Helper()

	cfg := config.Load()
	dbConn, err := dbpkg.Open(cfg.DatabaseURL)
	if err != nil {
		t.Skipf("functional tests are skipped: database is unavailable (%v)", err)
	}
	t.Cleanup(func() {
		_ = dbConn.Close()
	})

	rootDir := findRepoRoot(t)
	migrationsDir := filepath.Join(rootDir, "migrations")

	if err := dbpkg.RunMigrations(dbConn, migrationsDir); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	if err := dbpkg.EnsureNutritionCompatibility(dbConn); err != nil {
		t.Fatalf("ensure nutrition compatibility: %v", err)
	}
	if err := dbpkg.Seed(dbConn); err != nil {
		t.Fatalf("seed test data: %v", err)
	}

	hasPasswordTemp := false
	err = dbConn.QueryRow(
		`select exists (
       select 1
       from information_schema.columns
       where table_schema = current_schema()
         and table_name = 'users'
         and column_name = 'password_temp'
     )`,
	).Scan(&hasPasswordTemp)
	if err != nil {
		t.Fatalf("check users.password_temp column: %v", err)
	}
	if hasPasswordTemp {
		_, err = dbConn.Exec(`update users set password_temp = false where employee_id in ('10001', '20001', '90000')`)
		if err != nil {
			t.Fatalf("prepare demo users: %v", err)
		}
	}

	renderer, err := web.NewRenderer()
	if err != nil {
		t.Fatalf("create renderer: %v", err)
	}

	sessions := &appmiddleware.SessionManager{
		DB:         dbConn,
		CookieName: cfg.CookieName,
		SessionTTL: cfg.SessionTTL,
		Secure:     false,
	}

	router := chi.NewRouter()
	router.Use(middleware.RealIP)
	router.Use(middleware.StripSlashes)
	router.Use(appmiddleware.Logger)
	router.Use(appmiddleware.Recover)
	router.Handle("/assets/*", web.StaticHandler())
	router.Handle("/uploads/*", http.StripPrefix("/uploads/", http.FileServer(http.Dir(filepath.Join(rootDir, "uploads")))))
	router.Mount("/", site.New(dbConn, renderer, sessions, cfg).Router())

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	return server.URL
}

func findRepoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("cannot locate repository root from %q", dir)
		}
		dir = parent
	}
}

func newFunctionalClient(t *testing.T) *http.Client {
	t.Helper()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("create cookie jar: %v", err)
	}

	return &http.Client{
		Timeout: 15 * time.Second,
		Jar:     jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func mustFormRequest(t *testing.T, client *http.Client, method, target string, form url.Values) (*http.Response, string) {
	t.Helper()
	body := strings.NewReader(form.Encode())
	req, err := http.NewRequest(method, target, body)
	if err != nil {
		t.Fatalf("create form request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return doRequest(t, client, req)
}

func mustRequest(t *testing.T, client *http.Client, method, target string, body io.Reader) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(method, target, body)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	return doRequest(t, client, req)
}

func doRequest(t *testing.T, client *http.Client, req *http.Request) (*http.Response, string) {
	t.Helper()
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request %s %s failed: %v", req.Method, req.URL.String(), err)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		_ = resp.Body.Close()
		t.Fatalf("read response body: %v", err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("close response body: %v", err)
	}

	resp.Body = io.NopCloser(strings.NewReader(string(data)))
	return resp, string(data)
}
