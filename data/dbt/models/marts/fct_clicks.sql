-- fct_clicks: one row per click. Grain: one click event (event_id).
--
-- Incremental: the first build copies every click; later builds only process
-- the last few days and replace those rows (delete+insert on event_id). The
-- three-day lookback picks up events that arrive late, and replacing instead
-- of appending means a rerun never counts a click twice.
-- `snipdata build --full-refresh` rebuilds it from scratch.
--
-- An orchestrator can instead name the days to (re)process, which is how a
-- backfill works (chapter 17.5):
--   dbt build --vars '{start_date: 2026-10-01, end_date: 2026-10-07}'
-- Those days' rows are deleted first (the pre_hook), then exactly those days'
-- events are inserted: the days are overwritten whole, so a rerun gives the
-- same rows, and an event removed from the lake leaves the table too. On the
-- first build this selects only those days, so the table can be filled one
-- day partition at a time.

{{
    config(
        materialized = 'incremental',
        unique_key = 'event_id',
        incremental_strategy = 'delete+insert',
        on_schema_change = 'fail',
        pre_hook = "{% if is_incremental() and var('start_date', none) is not none %}
            delete from {{ this }}
            where click_date between date '{{ var(\"start_date\") }}'
                and date '{{ var(\"end_date\", var(\"start_date\")) }}'
        {% endif %}"
    )
}}

select
    event_id,
    slug,                                     -- joins to dim_links.slug
    clicked_at,
    click_date,
    cast(extract(hour from clicked_at) as integer) as click_hour_utc,
    cast(isodow(clicked_at) as integer)            as click_iso_weekday   -- 1 = Monday ... 7 = Sunday
from {{ ref('stg_clicks') }}

{% if var('start_date', none) is not none %}
where click_date between date '{{ var("start_date") }}' and date '{{ var("end_date", var("start_date")) }}'
{% elif is_incremental() %}
where click_date >= (select coalesce(max(click_date), date '1970-01-01') - 2 from {{ this }})
{% endif %}
