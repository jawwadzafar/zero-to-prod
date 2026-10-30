-- fct_clicks: one row per click. Grain: one click event (event_id).
--
-- Incremental: the first build copies every click; later builds only process
-- the last few days and replace those rows (delete+insert on event_id). The
-- three-day lookback picks up events that arrive late, and replacing instead
-- of appending means a rerun never counts a click twice.
-- `snipdata build --full-refresh` rebuilds it from scratch.

{{
    config(
        materialized = 'incremental',
        unique_key = 'event_id',
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
