package db

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// EnsureNutritionCompatibility upgrades legacy nutrition schemas in-place.
// It is safe to run on every start: all statements are idempotent.
func EnsureNutritionCompatibility(db *sql.DB) error {
	steps := []struct {
		name string
		sql  string
	}{
		{
			name: "uuid extension",
			sql:  `create extension if not exists "uuid-ossp";`,
		},
		{
			name: "core users/sessions/profile tables",
			sql: `create table if not exists users (
			         id uuid primary key default uuid_generate_v4(),
			         name text not null default '',
			         employee_id text,
			         password_hash text not null default '',
			         password_temp boolean not null default false,
			         role text not null default 'employee',
			         department text,
			         position text,
			         corporate_email text,
			         created_at timestamptz not null default now(),
			         updated_at timestamptz not null default now()
			      );
			      alter table if exists users add column if not exists department text;
			      alter table if exists users add column if not exists position text;
			      alter table if exists users add column if not exists corporate_email text;
			      alter table if exists users add column if not exists password_temp boolean not null default false;
			      create index if not exists idx_users_employee_id
			      on users(employee_id);
			      create index if not exists idx_users_password_temp
			      on users(password_temp)
			      where password_temp = true;
			      create index if not exists idx_users_corporate_email_lower
			      on users (lower(corporate_email))
			      where corporate_email is not null and btrim(corporate_email) <> '';

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
			      alter table if exists user_points add column if not exists points_total int not null default 0;

			      create table if not exists sessions (
			         id uuid primary key default uuid_generate_v4(),
			         user_id uuid references users(id) on delete cascade,
			         token text not null unique,
			         expires_at timestamptz not null,
			         created_at timestamptz not null default now()
			      );
			      create index if not exists idx_sessions_token on sessions(token);`,
		},
		{
			name: "password reset requests",
			sql: `create table if not exists password_reset_requests (
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
			      alter table if exists password_reset_requests add column if not exists status text not null default 'pending';
			      alter table if exists password_reset_requests add column if not exists requested_at timestamptz not null default now();
			      alter table if exists password_reset_requests add column if not exists processed_at timestamptz;
			      alter table if exists password_reset_requests add column if not exists processed_by uuid references users(id) on delete set null;
			      alter table if exists password_reset_requests add column if not exists temporary_password_set boolean not null default false;
			      alter table if exists password_reset_requests add column if not exists temporary_password_set_at timestamptz;
			      alter table if exists password_reset_requests add column if not exists note text not null default '';
			      update password_reset_requests
			      set status = case lower(btrim(coalesce(status, '')))
			        when 'pending' then 'pending'
			        when 'completed' then 'completed'
			        when 'rejected' then 'rejected'
			        else 'pending'
			      end;
			      update password_reset_requests
			      set requested_at = coalesce(requested_at, now())
			      where requested_at is null;
			      delete from password_reset_requests pr
			      using (
			        select id
			        from (
			          select id,
			                 row_number() over (
			                   partition by user_id
			                   order by requested_at desc, id desc
			                 ) as rn
			          from password_reset_requests
			          where status = 'pending'
			        ) ranked
			        where ranked.rn > 1
			      ) dups
			      where pr.id = dups.id;
			      create index if not exists idx_password_reset_requests_status_requested
			      on password_reset_requests(status, requested_at desc);
			      create index if not exists idx_password_reset_requests_user_requested
			      on password_reset_requests(user_id, requested_at desc);
			      create unique index if not exists idx_password_reset_requests_pending_user
			      on password_reset_requests(user_id)
			      where status = 'pending';`,
		},
		{
			name: "nutrition_plan_meals columns",
			sql: `create table if not exists nutrition_plan_meals (
			         id uuid primary key default uuid_generate_v4(),
			         user_id uuid not null references users(id) on delete cascade,
			         day_date date,
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
			         skipped_at timestamptz
			      );

			      alter table if exists nutrition_plan_meals
			      add column if not exists day_date date;
			      alter table if exists nutrition_plan_meals
			      add column if not exists status text not null default 'planned';
			      alter table if exists nutrition_plan_meals
			      add column if not exists planned_time text not null default '';
			      alter table if exists nutrition_plan_meals
			      add column if not exists smart_swap_from_meal_id text;
			      alter table if exists nutrition_plan_meals
			      add column if not exists completed_at timestamptz;
			      alter table if exists nutrition_plan_meals
			      add column if not exists skipped_at timestamptz;`,
		},
		{
			name: "nutrition_custom_meals compatibility",
			sql: `create table if not exists nutrition_custom_meals (
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

			      alter table if exists nutrition_custom_meals
			      add column if not exists meal_id text;
			      alter table if exists nutrition_custom_meals
			      add column if not exists name text;
			      alter table if exists nutrition_custom_meals
			      add column if not exists description text not null default '';
			      alter table if exists nutrition_custom_meals
			      add column if not exists category text;
			      alter table if exists nutrition_custom_meals
			      add column if not exists calories int not null default 0;
			      alter table if exists nutrition_custom_meals
			      add column if not exists protein int not null default 0;
			      alter table if exists nutrition_custom_meals
			      add column if not exists carbs int not null default 0;
			      alter table if exists nutrition_custom_meals
			      add column if not exists fats int not null default 0;
			      alter table if exists nutrition_custom_meals
			      add column if not exists active boolean not null default true;
			      alter table if exists nutrition_custom_meals
			      add column if not exists created_by uuid references users(id) on delete set null;
			      alter table if exists nutrition_custom_meals
			      add column if not exists created_at timestamptz not null default now();
			      alter table if exists nutrition_custom_meals
			      add column if not exists updated_at timestamptz not null default now();

			      create unique index if not exists idx_nutrition_custom_meals_meal_id
			      on nutrition_custom_meals(meal_id);
			      create index if not exists idx_nutrition_custom_meals_active_category
			      on nutrition_custom_meals(active, category);`,
		},
		{
			name: "nutrition_plan_meals day_date backfill",
			sql: `with current_week as (
			         select (current_date - ((extract(isodow from current_date)::int) - 1))::date as week_start
			      )
			      update nutrition_plan_meals npm
			      set day_date = current_week.week_start + case lower(trim(coalesce(npm.day_key, '')))
			        when 'monday' then 0
			        when 'tuesday' then 1
			        when 'wednesday' then 2
			        when 'thursday' then 3
			        when 'friday' then 4
			        when 'saturday' then 5
			        when 'sunday' then 6
			        else 0
			      end
			      from current_week
			      where npm.day_date is null;

			      update nutrition_plan_meals
			      set day_date = current_date
			      where day_date is null;`,
		},
		{
			name: "nutrition_plan_meals uniqueness",
			sql: `delete from nutrition_plan_meals npm
			      using (
			        select id
			        from (
			          select id,
			                 row_number() over (
			                   partition by user_id, day_date, meal_slot
			                   order by updated_at desc, created_at desc, id desc
			                 ) as rn
			          from nutrition_plan_meals
			          where day_date is not null
			        ) ranked
			        where ranked.rn > 1
			      ) dups
			      where npm.id = dups.id;

			      alter table if exists nutrition_plan_meals
			      alter column day_date set not null;

			      create unique index if not exists idx_nutrition_plan_meals_user_day_date_slot
			      on nutrition_plan_meals (user_id, day_date, meal_slot);
			      create index if not exists idx_nutrition_plan_meals_user_day
			      on nutrition_plan_meals (user_id, day_date);`,
		},
		{
			name: "nutrition_day_progress compatibility",
			sql: `create table if not exists nutrition_day_progress (
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

			      alter table if exists nutrition_day_progress
			      add column if not exists points_awarded boolean not null default false;
			      alter table if exists nutrition_day_progress
			      add column if not exists completed_at timestamptz;
			      create index if not exists idx_nutrition_day_progress_user_date
			      on nutrition_day_progress (user_id, day_date desc);`,
		},
		{
			name: "nutrition_hydration_logs compatibility",
			sql: `create table if not exists nutrition_hydration_logs (
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

			      alter table if exists nutrition_hydration_logs
			      add column if not exists day_date date;
			      alter table if exists nutrition_hydration_logs
			      add column if not exists status text not null default 'planned';
			      alter table if exists nutrition_hydration_logs
			      add column if not exists completed_at timestamptz;
			      alter table if exists nutrition_hydration_logs
			      add column if not exists updated_at timestamptz not null default now();

			      with current_week as (
			        select (current_date - ((extract(isodow from current_date)::int) - 1))::date as week_start
			      )
			      update nutrition_hydration_logs nhl
			      set day_date = current_week.week_start + case lower(trim(coalesce(nhl.day_key, '')))
			        when 'monday' then 0
			        when 'tuesday' then 1
			        when 'wednesday' then 2
			        when 'thursday' then 3
			        when 'friday' then 4
			        when 'saturday' then 5
			        when 'sunday' then 6
			        else 0
			      end
			      from current_week
			      where nhl.day_date is null;

			      update nutrition_hydration_logs
			      set day_date = current_date
			      where day_date is null;

			      create unique index if not exists idx_nutrition_hydration_logs_user_day_reminder
			      on nutrition_hydration_logs (user_id, day_date, reminder_key);
			      create index if not exists idx_nutrition_hydration_logs_user_date
			      on nutrition_hydration_logs(user_id, day_date desc);`,
		},
		{
			name: "nutrition questionnaire and profile stats",
			sql: `create table if not exists nutrition_questionnaire_responses (
			         user_id uuid primary key references users(id) on delete cascade,
			         answers jsonb not null default '{}',
			         updated_at timestamptz not null default now()
			      );

			      create table if not exists nutrition_user_stats (
			         user_id uuid primary key references users(id) on delete cascade,
			         current_streak int not null default 0,
			         best_streak int not null default 0,
			         total_completed_days int not null default 0,
			         last_completed_day date,
			         created_at timestamptz not null default now(),
			         updated_at timestamptz not null default now()
			      );`,
		},
		{
			name: "nutrition_reminder_settings",
			sql: `create table if not exists nutrition_reminder_settings (
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
			      );`,
		},
		{
			name: "nutrition_events",
			sql: `create table if not exists nutrition_events (
			         id uuid primary key default uuid_generate_v4(),
			         user_id uuid not null references users(id) on delete cascade,
			         message text not null,
			         created_at timestamptz not null default now()
			      );
			      create index if not exists idx_nutrition_events_user
			      on nutrition_events(user_id, created_at desc);`,
		},
		{
			name: "nutrition rewards and limits",
			sql: `create table if not exists nutrition_reward_redemptions (
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
			      alter table if exists nutrition_reward_redemptions add column if not exists requested_at timestamptz;
			      alter table if exists nutrition_reward_redemptions add column if not exists reviewed_at timestamptz;
			      alter table if exists nutrition_reward_redemptions add column if not exists reviewed_by uuid references users(id) on delete set null;
			      alter table if exists nutrition_reward_redemptions add column if not exists manager_comment text not null default '';
			      update nutrition_reward_redemptions
			      set requested_at = coalesce(requested_at, redeemed_at, now())
			      where requested_at is null;
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
			                    updated_at = now();`,
		},
		{
			name: "nutrition achievements tables",
			sql: `create table if not exists nutrition_achievement_rules (
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
			      on nutrition_user_achievements(user_id, unlocked, updated_at desc);`,
		},
		{
			name: "nutrition points and events ledger",
			sql: `create table if not exists nutrition_points_ledger (
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
			      alter table if exists nutrition_points_ledger add column if not exists change_amount int;
			      alter table if exists nutrition_points_ledger add column if not exists balance_after int;
			      alter table if exists nutrition_points_ledger add column if not exists reason_code text;
			      alter table if exists nutrition_points_ledger add column if not exists reason text not null default '';
			      alter table if exists nutrition_points_ledger add column if not exists source_type text not null default 'system';
			      alter table if exists nutrition_points_ledger add column if not exists source_id text;
			      alter table if exists nutrition_points_ledger add column if not exists created_by uuid references users(id) on delete set null;
			      alter table if exists nutrition_points_ledger add column if not exists created_at timestamptz not null default now();
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
			      alter table if exists nutrition_day_event_history
			      add column if not exists metadata jsonb not null default '{}'::jsonb;
			      alter table if exists nutrition_day_event_history
			      add column if not exists created_at timestamptz not null default now();
			      create index if not exists idx_nutrition_day_event_history_user_day
			      on nutrition_day_event_history(user_id, day_date desc, created_at desc);`,
		},
		{
			name: "nutrition action audit",
			sql: `create table if not exists nutrition_action_audit (
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
			      on nutrition_action_audit(entity_type, entity_id);`,
		},
		{
			name: "support tickets and messages",
			sql: `create table if not exists support_tickets (
			         id uuid primary key default uuid_generate_v4(),
			         user_id uuid not null references users(id) on delete cascade,
			         subject text not null,
			         status text not null default 'open',
			         created_at timestamptz not null default now(),
			         updated_at timestamptz not null default now(),
			         last_message_at timestamptz not null default now(),
			         check (status in ('open', 'answered', 'closed'))
			      );
			      alter table if exists support_tickets add column if not exists user_id uuid references users(id) on delete cascade;
			      alter table if exists support_tickets add column if not exists subject text;
			      alter table if exists support_tickets add column if not exists status text not null default 'open';
			      alter table if exists support_tickets add column if not exists created_at timestamptz not null default now();
			      alter table if exists support_tickets add column if not exists updated_at timestamptz not null default now();
			      alter table if exists support_tickets add column if not exists last_message_at timestamptz;
			      update support_tickets
			      set status = case lower(btrim(coalesce(status, '')))
			        when 'open' then 'open'
			        when 'answered' then 'answered'
			        when 'closed' then 'closed'
			        when 'resolved' then 'closed'
			        else 'open'
			      end;
			      update support_tickets set created_at = coalesce(created_at, now()) where created_at is null;
			      update support_tickets set updated_at = coalesce(updated_at, created_at, now()) where updated_at is null;
			      update support_tickets set subject = coalesce(nullif(btrim(subject), ''), 'Обращение без темы')
			      where subject is null or btrim(subject) = '';
			      update support_tickets
			      set last_message_at = coalesce(last_message_at, updated_at, created_at, now())
			      where last_message_at is null;
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
			      alter table if exists support_ticket_messages add column if not exists ticket_id uuid references support_tickets(id) on delete cascade;
			      alter table if exists support_ticket_messages add column if not exists sender_id uuid references users(id) on delete set null;
			      alter table if exists support_ticket_messages add column if not exists sender_role text not null default 'employee';
			      alter table if exists support_ticket_messages add column if not exists message text;
			      alter table if exists support_ticket_messages add column if not exists created_at timestamptz not null default now();
			      update support_ticket_messages
			      set sender_role = case lower(btrim(coalesce(sender_role, '')))
			        when 'admin' then 'admin'
			        when 'manager' then 'manager'
			        when 'system' then 'system'
			        else 'employee'
			      end;
			      update support_ticket_messages set created_at = coalesce(created_at, now()) where created_at is null;
			      update support_ticket_messages
			      set message = coalesce(nullif(btrim(message), ''), 'Сообщение без текста')
			      where message is null or btrim(message) = '';
			      create index if not exists idx_support_ticket_messages_ticket_created
			      on support_ticket_messages(ticket_id, created_at);`,
		},
		{
			name: "support legacy non-null relax",
			sql: `do $$
			      declare
			        col record;
			      begin
			        if to_regclass('support_tickets') is not null then
			          for col in
			            select c.column_name
			            from information_schema.columns c
			            where c.table_schema = 'public'
			              and c.table_name = 'support_tickets'
			              and c.is_nullable = 'NO'
			              and c.column_default is null
			              and c.column_name not in ('id', 'user_id', 'subject', 'status', 'created_at', 'updated_at', 'last_message_at')
			          loop
			            execute format('alter table support_tickets alter column %I drop not null', col.column_name);
			          end loop;
			        end if;

			        if to_regclass('support_ticket_messages') is not null then
			          for col in
			            select c.column_name
			            from information_schema.columns c
			            where c.table_schema = 'public'
			              and c.table_name = 'support_ticket_messages'
			              and c.is_nullable = 'NO'
			              and c.column_default is null
			              and c.column_name not in ('id', 'ticket_id', 'sender_id', 'sender_role', 'message', 'created_at')
			          loop
			            execute format('alter table support_ticket_messages alter column %I drop not null', col.column_name);
			          end loop;
			        end if;
			      end $$;`,
		},
	}

	for _, step := range steps {
		if _, err := db.Exec(step.sql); err != nil {
			return fmt.Errorf("nutrition compatibility: %s: %w", step.name, err)
		}
	}

	if err := validateNutritionSchema(db); err != nil {
		return err
	}

	return nil
}

func validateNutritionSchema(db *sql.DB) error {
	requiredTables := []string{
		"users",
		"sessions",
		"user_profiles",
		"user_points",
		"password_reset_requests",
		"nutrition_plan_meals",
		"nutrition_day_progress",
		"nutrition_hydration_logs",
		"nutrition_reminder_settings",
		"nutrition_events",
		"nutrition_questionnaire_responses",
		"nutrition_user_stats",
		"nutrition_reward_redemptions",
		"nutrition_reward_limits",
		"nutrition_achievement_rules",
		"nutrition_achievement_catalog",
		"nutrition_user_achievements",
		"nutrition_points_ledger",
		"nutrition_day_event_history",
		"nutrition_action_audit",
		"support_tickets",
		"support_ticket_messages",
	}

	requiredColumns := map[string][]string{
		"users":                             {"id", "name", "employee_id", "password_hash", "password_temp", "role", "department", "position", "corporate_email"},
		"sessions":                          {"id", "user_id", "token", "expires_at"},
		"user_profiles":                     {"user_id", "notifications_cleared_at"},
		"user_points":                       {"user_id", "points_balance", "points_total"},
		"password_reset_requests":           {"id", "user_id", "status", "requested_at", "processed_at", "processed_by", "temporary_password_set", "temporary_password_set_at", "note"},
		"nutrition_plan_meals":              {"id", "user_id", "day_date", "day_key", "meal_slot", "meal_id", "meal_name", "status", "planned_time"},
		"nutrition_day_progress":            {"user_id", "day_date", "day_key", "completed_slots", "total_slots", "day_completed", "points_awarded"},
		"nutrition_hydration_logs":          {"user_id", "day_date", "day_key", "reminder_key", "status", "completed_at"},
		"nutrition_reminder_settings":       {"user_id", "meal_reminder_lead_minutes", "meal_sla_minutes", "hydration_1030_enabled", "hydration_1500_enabled", "hydration_1800_enabled"},
		"nutrition_events":                  {"id", "user_id", "message", "created_at"},
		"nutrition_questionnaire_responses": {"user_id", "answers", "updated_at"},
		"nutrition_user_stats":              {"user_id", "current_streak", "best_streak", "total_completed_days", "last_completed_day"},
		"nutrition_reward_redemptions":      {"id", "user_id", "reward_id", "reward_title", "points_cost", "status", "requested_at", "reviewed_at", "reviewed_by", "manager_comment"},
		"nutrition_reward_limits":           {"reward_id", "max_per_user"},
		"nutrition_achievement_rules":       {"id", "rule_code", "metric_key", "window_days", "target_value"},
		"nutrition_achievement_catalog":     {"id", "code", "title", "rule_id", "active"},
		"nutrition_user_achievements":       {"user_id", "achievement_id", "progress", "target", "unlocked"},
		"nutrition_points_ledger":           {"id", "user_id", "change_amount", "balance_after", "reason_code", "reason", "source_type", "source_id", "created_at"},
		"nutrition_day_event_history":       {"id", "user_id", "day_date", "day_key", "event_type", "slot_key", "metadata", "created_at"},
		"nutrition_action_audit":            {"id", "module", "action_type", "entity_type", "entity_id", "actor_id", "target_user_id", "details", "created_at"},
		"support_tickets":                   {"id", "user_id", "subject", "status", "created_at", "updated_at", "last_message_at"},
		"support_ticket_messages":           {"id", "ticket_id", "sender_id", "sender_role", "message", "created_at"},
	}

	missingTables := make([]string, 0)
	for _, table := range requiredTables {
		var reg sql.NullString
		if err := db.QueryRow(`select to_regclass($1)`, "public."+table).Scan(&reg); err != nil {
			return fmt.Errorf("nutrition compatibility: validate table %s: %w", table, err)
		}
		if !reg.Valid || strings.TrimSpace(reg.String) == "" {
			missingTables = append(missingTables, table)
		}
	}
	if len(missingTables) > 0 {
		sort.Strings(missingTables)
		return fmt.Errorf("nutrition compatibility: missing required tables: %s", strings.Join(missingTables, ", "))
	}

	for table, cols := range requiredColumns {
		columnSet := map[string]bool{}
		rows, err := db.Query(
			`select column_name
			 from information_schema.columns
			 where table_schema = 'public' and table_name = $1`,
			table,
		)
		if err != nil {
			return fmt.Errorf("nutrition compatibility: validate columns %s: %w", table, err)
		}
		for rows.Next() {
			var col string
			if scanErr := rows.Scan(&col); scanErr == nil {
				columnSet[strings.ToLower(strings.TrimSpace(col))] = true
			}
		}
		rows.Close()

		missingCols := make([]string, 0)
		for _, col := range cols {
			if !columnSet[strings.ToLower(col)] {
				missingCols = append(missingCols, col)
			}
		}
		if len(missingCols) > 0 {
			sort.Strings(missingCols)
			return fmt.Errorf(
				"nutrition compatibility: table %s missing required columns: %s",
				table,
				strings.Join(missingCols, ", "),
			)
		}
	}

	return nil
}
