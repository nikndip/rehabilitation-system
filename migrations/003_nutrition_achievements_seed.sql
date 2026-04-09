-- +migrate Up
create extension if not exists "uuid-ossp";

create table if not exists nutrition_achievement_rules (
  id uuid primary key default uuid_generate_v4(),
  rule_code text not null unique,
  metric_key text not null,
  window_days int not null default 0,
  target_value int not null,
  description text not null default '',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  check (target_value > 0),
  check (window_days >= 0)
);

create table if not exists nutrition_achievement_catalog (
  id uuid primary key default uuid_generate_v4(),
  code text not null unique,
  title text not null,
  description text not null,
  icon text not null default '🏅',
  points_reward int not null default 0,
  rule_id uuid not null references nutrition_achievement_rules(id) on delete cascade,
  active boolean not null default true,
  sort_order int not null default 100,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  check (points_reward >= 0)
);

create table if not exists nutrition_user_achievements (
  user_id uuid not null references users(id) on delete cascade,
  achievement_id uuid not null references nutrition_achievement_catalog(id) on delete cascade,
  progress int not null default 0,
  target int not null default 0,
  unlocked boolean not null default false,
  unlocked_at timestamptz,
  last_progress_at timestamptz,
  updated_at timestamptz not null default now(),
  primary key (user_id, achievement_id)
);

create index if not exists idx_nutrition_user_achievements_user
  on nutrition_user_achievements(user_id, unlocked, updated_at desc);

insert into nutrition_achievement_rules (rule_code, metric_key, window_days, target_value, description)
values
  ('nutri-streak-7', 'best_streak', 0, 7, '7 дней подряд без пропусков основного рациона'),
  ('nutri-hydration-14', 'hydration_days_total', 0, 14, '14 дней с отмеченным водным балансом'),
  ('nutri-days-10', 'completed_days_total', 0, 10, '10 полностью закрытых дней питания'),
  ('nutri-days-12', 'completed_days_total', 0, 12, '12 дней с устойчивым вечерним режимом'),
  ('nutri-days-30', 'completed_days_total', 0, 30, '30 дней по плану питания')
on conflict (rule_code)
do update set metric_key = excluded.metric_key,
              window_days = excluded.window_days,
              target_value = excluded.target_value,
              description = excluded.description,
              updated_at = now();

insert into nutrition_achievement_catalog (code, title, description, icon, points_reward, rule_id, active, sort_order)
select 'nutri-7-days', '7 дней режима', '7 дней подряд без пропуска основного рациона.', '🥗', 40, id, true, 10
from nutrition_achievement_rules
where rule_code = 'nutri-streak-7'
on conflict (code)
do update set title = excluded.title,
              description = excluded.description,
              icon = excluded.icon,
              points_reward = excluded.points_reward,
              rule_id = excluded.rule_id,
              active = excluded.active,
              sort_order = excluded.sort_order,
              updated_at = now();

insert into nutrition_achievement_catalog (code, title, description, icon, points_reward, rule_id, active, sort_order)
select 'nutri-water-balance', 'Водный баланс', 'Отмечайте водный баланс не менее 14 дней.', '💧', 50, id, true, 20
from nutrition_achievement_rules
where rule_code = 'nutri-hydration-14'
on conflict (code)
do update set title = excluded.title,
              description = excluded.description,
              icon = excluded.icon,
              points_reward = excluded.points_reward,
              rule_id = excluded.rule_id,
              active = excluded.active,
              sort_order = excluded.sort_order,
              updated_at = now();

insert into nutrition_achievement_catalog (code, title, description, icon, points_reward, rule_id, active, sort_order)
select 'nutri-protein-focus', 'Белковый фокус', 'Закройте 10 полноценных дней питания по плану.', '🍗', 45, id, true, 30
from nutrition_achievement_rules
where rule_code = 'nutri-days-10'
on conflict (code)
do update set title = excluded.title,
              description = excluded.description,
              icon = excluded.icon,
              points_reward = excluded.points_reward,
              rule_id = excluded.rule_id,
              active = excluded.active,
              sort_order = excluded.sort_order,
              updated_at = now();

insert into nutrition_achievement_catalog (code, title, description, icon, points_reward, rule_id, active, sort_order)
select 'nutri-stable-dinner', 'Стабильный ужин', 'Соблюдайте вечерний режим 12 дней по плану питания.', '🌙', 35, id, true, 40
from nutrition_achievement_rules
where rule_code = 'nutri-days-12'
on conflict (code)
do update set title = excluded.title,
              description = excluded.description,
              icon = excluded.icon,
              points_reward = excluded.points_reward,
              rule_id = excluded.rule_id,
              active = excluded.active,
              sort_order = excluded.sort_order,
              updated_at = now();

insert into nutrition_achievement_catalog (code, title, description, icon, points_reward, rule_id, active, sort_order)
select 'nutri-month-recovery', 'Месяц восстановления', 'Закройте 30 дней питания по плану.', '🏅', 120, id, true, 50
from nutrition_achievement_rules
where rule_code = 'nutri-days-30'
on conflict (code)
do update set title = excluded.title,
              description = excluded.description,
              icon = excluded.icon,
              points_reward = excluded.points_reward,
              rule_id = excluded.rule_id,
              active = excluded.active,
              sort_order = excluded.sort_order,
              updated_at = now();

-- +migrate Down
-- Rollback is intentionally not supported.
