-- +migrate Up
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

-- +migrate Down
-- Rollback is intentionally not supported.
