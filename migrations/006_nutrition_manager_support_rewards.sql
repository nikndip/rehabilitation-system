-- +migrate Up
create extension if not exists "uuid-ossp";

alter table if exists nutrition_reward_redemptions
  add column if not exists requested_at timestamptz;
alter table if exists nutrition_reward_redemptions
  add column if not exists reviewed_at timestamptz;
alter table if exists nutrition_reward_redemptions
  add column if not exists reviewed_by uuid references users(id) on delete set null;
alter table if exists nutrition_reward_redemptions
  add column if not exists manager_comment text not null default '';

update nutrition_reward_redemptions
set requested_at = coalesce(requested_at, redeemed_at, now())
where requested_at is null;

alter table if exists nutrition_reward_redemptions
  alter column requested_at set default now();
alter table if exists nutrition_reward_redemptions
  alter column requested_at set not null;

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

alter table if exists support_tickets
  add column if not exists user_id uuid references users(id) on delete cascade;
alter table if exists support_tickets
  add column if not exists subject text;
alter table if exists support_tickets
  add column if not exists status text not null default 'open';
alter table if exists support_tickets
  add column if not exists created_at timestamptz not null default now();
alter table if exists support_tickets
  add column if not exists updated_at timestamptz not null default now();
alter table if exists support_tickets
  add column if not exists last_message_at timestamptz;

update support_tickets
set status = case lower(btrim(coalesce(status, '')))
  when 'open' then 'open'
  when 'answered' then 'answered'
  when 'closed' then 'closed'
  when 'resolved' then 'closed'
  else 'open'
end;

update support_tickets
set created_at = coalesce(created_at, now())
where created_at is null;

update support_tickets
set updated_at = coalesce(updated_at, created_at, now())
where updated_at is null;

update support_tickets
set subject = coalesce(nullif(btrim(subject), ''), 'Обращение без темы')
where subject is null or btrim(subject) = '';

update support_tickets
set last_message_at = coalesce(last_message_at, updated_at, created_at, now())
where last_message_at is null;

alter table if exists support_tickets
  alter column status set default 'open';
alter table if exists support_tickets
  alter column status set not null;
alter table if exists support_tickets
  alter column created_at set default now();
alter table if exists support_tickets
  alter column created_at set not null;
alter table if exists support_tickets
  alter column updated_at set default now();
alter table if exists support_tickets
  alter column updated_at set not null;
alter table if exists support_tickets
  alter column last_message_at set default now();
alter table if exists support_tickets
  alter column last_message_at set not null;
alter table if exists support_tickets
  alter column subject set not null;

do $$
begin
  if to_regclass('support_tickets') is null then
    return;
  end if;
  if not exists (
    select 1
    from pg_constraint c
    where c.conrelid = 'support_tickets'::regclass
      and c.conname = 'support_tickets_status_check'
  ) then
    alter table support_tickets
      add constraint support_tickets_status_check
      check (status in ('open', 'answered', 'closed'));
  end if;
end $$;

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

alter table if exists support_ticket_messages
  add column if not exists ticket_id uuid references support_tickets(id) on delete cascade;
alter table if exists support_ticket_messages
  add column if not exists sender_id uuid references users(id) on delete set null;
alter table if exists support_ticket_messages
  add column if not exists sender_role text not null default 'employee';
alter table if exists support_ticket_messages
  add column if not exists message text;
alter table if exists support_ticket_messages
  add column if not exists created_at timestamptz not null default now();

update support_ticket_messages
set sender_role = case lower(btrim(coalesce(sender_role, '')))
  when 'admin' then 'admin'
  when 'manager' then 'manager'
  when 'system' then 'system'
  else 'employee'
end;

update support_ticket_messages
set created_at = coalesce(created_at, now())
where created_at is null;

update support_ticket_messages
set message = coalesce(nullif(btrim(message), ''), 'Сообщение без текста')
where message is null or btrim(message) = '';

alter table if exists support_ticket_messages
  alter column sender_role set default 'employee';
alter table if exists support_ticket_messages
  alter column sender_role set not null;
alter table if exists support_ticket_messages
  alter column created_at set default now();
alter table if exists support_ticket_messages
  alter column created_at set not null;
alter table if exists support_ticket_messages
  alter column message set not null;

do $$
begin
  if to_regclass('support_ticket_messages') is null then
    return;
  end if;
  if not exists (
    select 1
    from pg_constraint c
    where c.conrelid = 'support_ticket_messages'::regclass
      and c.conname = 'support_ticket_messages_sender_role_check'
  ) then
    alter table support_ticket_messages
      add constraint support_ticket_messages_sender_role_check
      check (sender_role in ('employee', 'admin', 'manager', 'system'));
  end if;
end $$;

create index if not exists idx_support_ticket_messages_ticket_created
  on support_ticket_messages(ticket_id, created_at);

-- +migrate Down
-- Rollback is intentionally not supported.
