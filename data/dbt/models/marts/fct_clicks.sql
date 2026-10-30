-- fct_clicks: one row per click. Grain: one click event (event_id).
--
-- Incremental: the first build copies every click. Later builds replace only
-- the last three days (delete+insert on click_date): every row of those days
-- is deleted and rebuilt from the lake. That picks up events that arrived
-- late, and drops rows that were removed upstream, and a rerun never counts a
-- click twice: the same habit as replacing a day's partition (chapter 17.2).
-- Anything older than three days changes only with
-- `snipdata build --full-refresh`, which rebuilds the table from scratch.

{{
    config(
        materialized = 'incremental',
        unique_key = 'click_date',
        incremental_strategy = 'delete+insert',
        on_schema_change = 'fail'
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

{% if is_incremental() %}
where click_date >= (select coalesce(max(click_date), date '1970-01-01') - 2 from {{ this }})
{% endif %}
