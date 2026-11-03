-- dhow:phase expand
-- +goose Up
alter table
    public.jobs
    drop
    column note;

-- +goose Down
alter table public.jobs add column note text;
