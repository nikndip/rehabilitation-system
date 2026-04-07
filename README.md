# Система реабилитации сотрудников

Полностью Go-приложение: сервер, шаблоны и стили. Интерфейс рендерится на сервере, данные хранятся в PostgreSQL.

## Requirements
- Go 1.22+
- PostgreSQL 14+

## Quick start
1) Запустите PostgreSQL (при желании через Docker):
   `docker-compose up -d`

2) Установите переменные окружения (см. `.env.example`):
   `export DATABASE_URL=postgres://rehab:rehab@localhost:5432/rehab_app?sslmode=disable`

3) Запустите сервер:
   `go run .`

Приложение будет доступно по адресу `http://localhost:8080`.

## Seed accounts (если `SEED_DATA=true`)
- Employee: `10001` / `password`
- Employee: `20001` / `password`
- Admin: `90000` / `password`

## Основные страницы
- `/login`, `/register`
- `/` (дашборд питания), `/nutrition/plan`, `/nutrition/meals`, `/nutrition/profile`
- `/nutrition/leaderboard`, `/nutrition/rewards`, `/nutrition/achievements`
- `/admin/nutrition`, `/admin/nutrition/achievements`, `/admin/nutrition/points`
