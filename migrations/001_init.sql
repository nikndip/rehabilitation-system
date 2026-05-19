-- +migrate Up
create extension if not exists "uuid-ossp";

-- Destructive cleanup of legacy/training tables is intentionally excluded.
-- Keep this migration strictly additive and safe for auto-run.

create table if not exists users (
  id uuid primary key default uuid_generate_v4(),
  name text not null,
  employee_id text not null unique,
  password_hash text not null,
  password_temp boolean not null default false,
  role text not null default 'employee',
  department text,
  position text,
  corporate_email text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists user_profiles (
  user_id uuid primary key references users(id) on delete cascade,
  notifications_cleared_at timestamptz,
  updated_at timestamptz not null default now()
);

create table if not exists user_points (
  user_id uuid primary key references users(id) on delete cascade,
  points_balance int not null default 0,
  points_total int not null default 0,
  updated_at timestamptz not null default now()
);

create table if not exists sessions (
  id uuid primary key default uuid_generate_v4(),
  user_id uuid references users(id) on delete cascade,
  token text not null unique,
  expires_at timestamptz not null,
  created_at timestamptz not null default now()
);

create index if not exists idx_sessions_token on sessions(token);
create index if not exists idx_users_password_temp
  on users(password_temp)
  where password_temp = true;
create index if not exists idx_user_profiles_notifications_cleared_at
  on user_profiles(notifications_cleared_at);
create index if not exists idx_users_corporate_email_lower
  on users (lower(corporate_email))
  where corporate_email is not null and btrim(corporate_email) <> '';

create table if not exists password_reset_requests (
  id uuid primary key default uuid_generate_v4(),
  user_id uuid not null references users(id) on delete cascade,
  status text not null default 'pending',
  requested_at timestamptz not null default now(),
  processed_at timestamptz,
  processed_by uuid references users(id) on delete set null,
  temporary_password_set boolean not null default false,
  temporary_password_set_at timestamptz,
  note text not null default '',
  check (status in ('pending', 'completed', 'rejected'))
);

create index if not exists idx_password_reset_requests_status_requested
  on password_reset_requests(status, requested_at desc);
create index if not exists idx_password_reset_requests_user_requested
  on password_reset_requests(user_id, requested_at desc);
create unique index if not exists idx_password_reset_requests_pending_user
  on password_reset_requests(user_id)
  where status = 'pending';

create table if not exists nutrition_plan_meals (
  id uuid primary key default uuid_generate_v4(),
  user_id uuid not null references users(id) on delete cascade,
  day_date date not null,
  day_key text not null,
  meal_slot text not null,
  meal_id text not null,
  meal_name text not null,
  calories int not null default 0,
  protein int not null default 0,
  carbs int not null default 0,
  fats int not null default 0,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  status text not null default 'planned',
  planned_time text not null default '',
  smart_swap_from_meal_id text,
  completed_at timestamptz,
  skipped_at timestamptz,
  unique (user_id, day_date, meal_slot)
);

create index if not exists idx_nutrition_plan_meals_user_day
  on nutrition_plan_meals(user_id, day_date);

create table if not exists nutrition_custom_meals (
  id uuid primary key default uuid_generate_v4(),
  meal_id text not null unique,
  name text not null,
  description text not null default '',
  category text not null,
  calories int not null default 0,
  protein int not null default 0,
  carbs int not null default 0,
  fats int not null default 0,
  active boolean not null default true,
  created_by uuid references users(id) on delete set null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  check (calories >= 0 and calories <= 3000),
  check (protein >= 0 and protein <= 300),
  check (carbs >= 0 and carbs <= 400),
  check (fats >= 0 and fats <= 200)
);

create index if not exists idx_nutrition_custom_meals_active_category
  on nutrition_custom_meals(active, category);

create table if not exists nutrition_day_progress (
  user_id uuid not null references users(id) on delete cascade,
  day_date date not null,
  day_key text not null,
  completed_slots int not null default 0,
  total_slots int not null default 4,
  day_completed boolean not null default false,
  points_awarded boolean not null default false,
  completed_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  primary key (user_id, day_date)
);

create index if not exists idx_nutrition_day_progress_user_date
  on nutrition_day_progress(user_id, day_date desc);

create table if not exists nutrition_user_stats (
  user_id uuid primary key references users(id) on delete cascade,
  current_streak int not null default 0,
  best_streak int not null default 0,
  total_completed_days int not null default 0,
  last_completed_day date,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists nutrition_reward_redemptions (
  id uuid primary key default uuid_generate_v4(),
  user_id uuid not null references users(id) on delete cascade,
  reward_id text not null,
  reward_title text not null,
  points_cost int not null default 0,
  status text not null default 'issued',
  redeemed_at timestamptz not null default now(),
  used_at timestamptz,
  requested_at timestamptz not null default now(),
  reviewed_at timestamptz,
  reviewed_by uuid references users(id) on delete set null,
  manager_comment text not null default ''
);

create index if not exists idx_nutrition_reward_redemptions_user
  on nutrition_reward_redemptions(user_id, redeemed_at desc);
create index if not exists idx_nutrition_reward_redemptions_status_requested
  on nutrition_reward_redemptions(status, requested_at desc);
create index if not exists idx_nutrition_reward_redemptions_user_reward_status
  on nutrition_reward_redemptions(user_id, reward_id, status);
create index if not exists idx_nutrition_reward_redemptions_reviewed_by
  on nutrition_reward_redemptions(reviewed_by, reviewed_at desc);

create table if not exists nutrition_reward_limits (
  reward_id text primary key,
  max_per_user int,
  updated_at timestamptz not null default now(),
  check (max_per_user is null or max_per_user > 0)
);

insert into nutrition_reward_limits (reward_id, max_per_user)
values
  ('nutri-1', 1),
  ('nutri-2', 3),
  ('nutri-5', 1),
  ('nutri-8', 1)
on conflict (reward_id)
do update set max_per_user = excluded.max_per_user,
              updated_at = now();

create table if not exists support_tickets (
  id uuid primary key default uuid_generate_v4(),
  user_id uuid not null references users(id) on delete cascade,
  subject text not null,
  status text not null default 'open',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  last_message_at timestamptz not null default now(),
  check (status in ('open', 'answered', 'closed'))
);

create index if not exists idx_support_tickets_user_updated
  on support_tickets(user_id, updated_at desc);
create index if not exists idx_support_tickets_status_updated
  on support_tickets(status, updated_at desc);
create index if not exists idx_support_tickets_last_message
  on support_tickets(last_message_at desc);

create table if not exists support_ticket_messages (
  id uuid primary key default uuid_generate_v4(),
  ticket_id uuid not null references support_tickets(id) on delete cascade,
  sender_id uuid references users(id) on delete set null,
  sender_role text not null default 'employee',
  message text not null,
  created_at timestamptz not null default now(),
  check (sender_role in ('employee', 'admin', 'manager', 'system'))
);

create index if not exists idx_support_ticket_messages_ticket_created
  on support_ticket_messages(ticket_id, created_at);

create table if not exists nutrition_action_audit (
  id uuid primary key default uuid_generate_v4(),
  module text not null default 'nutrition',
  action_type text not null,
  entity_type text not null,
  entity_id text not null default '',
  actor_id uuid references users(id) on delete set null,
  actor_role text not null default 'system',
  actor_name text not null default '',
  target_user_id uuid references users(id) on delete set null,
  target_department text not null default '',
  details jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now()
);

create index if not exists idx_nutrition_action_audit_created
  on nutrition_action_audit(created_at desc);
create index if not exists idx_nutrition_action_audit_action
  on nutrition_action_audit(action_type, created_at desc);
create index if not exists idx_nutrition_action_audit_actor
  on nutrition_action_audit(actor_id, created_at desc);
create index if not exists idx_nutrition_action_audit_target_user
  on nutrition_action_audit(target_user_id, created_at desc);
create index if not exists idx_nutrition_action_audit_entity
  on nutrition_action_audit(entity_type, entity_id);

create table if not exists nutrition_events (
  id uuid primary key default uuid_generate_v4(),
  user_id uuid not null references users(id) on delete cascade,
  message text not null,
  created_at timestamptz not null default now()
);

create index if not exists idx_nutrition_events_user
  on nutrition_events(user_id, created_at desc);

create table if not exists nutrition_questionnaire_responses (
  user_id uuid primary key references users(id) on delete cascade,
  answers jsonb not null default '{}',
  updated_at timestamptz not null default now()
);

create table if not exists nutrition_hydration_logs (
  user_id uuid not null references users(id) on delete cascade,
  day_date date not null,
  day_key text not null,
  reminder_key text not null,
  status text not null default 'planned',
  completed_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  primary key (user_id, day_date, reminder_key)
);

create index if not exists idx_nutrition_hydration_logs_user_date
  on nutrition_hydration_logs(user_id, day_date desc);

create table if not exists nutrition_reminder_settings (
  user_id uuid primary key references users(id) on delete cascade,
  meal_reminder_lead_minutes int not null default 20,
  meal_sla_minutes int not null default 60,
  hydration_1030_enabled boolean not null default true,
  hydration_1500_enabled boolean not null default true,
  hydration_1800_enabled boolean not null default true,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  check (meal_reminder_lead_minutes >= 0 and meal_reminder_lead_minutes <= 240),
  check (meal_sla_minutes >= 15 and meal_sla_minutes <= 360)
);

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

create table if not exists nutrition_points_ledger (
  id uuid primary key default uuid_generate_v4(),
  user_id uuid not null references users(id) on delete cascade,
  change_amount int not null,
  balance_after int,
  reason_code text not null,
  reason text not null default '',
  source_type text not null default 'system',
  source_id text,
  created_by uuid references users(id) on delete set null,
  created_at timestamptz not null default now()
);

create index if not exists idx_nutrition_points_ledger_user_created
  on nutrition_points_ledger(user_id, created_at desc);

create table if not exists nutrition_day_event_history (
  id uuid primary key default uuid_generate_v4(),
  user_id uuid not null references users(id) on delete cascade,
  day_date date not null,
  day_key text not null,
  event_type text not null,
  slot_key text,
  metadata jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now()
);

create index if not exists idx_nutrition_day_event_history_user_day
  on nutrition_day_event_history(user_id, day_date desc, created_at desc);

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
-- One-file migration mode: rollback is intentionally not supported.
