-- Uniqueness, at the source (chapter 17.7). The stream delivers at least once,
-- so a few repeated event_ids are normal and stg_clicks removes them. A lot of
-- them means a consumer is replaying, or a file was copied twice: stg_clicks
-- still hides it, which is exactly why someone should hear about it.
-- Fails (as a warning) for every day where over 2% of rows repeat an event_id.

{{ config(severity = 'warn') }}

select
    dt,
    count(*)                                   as rows_in_lake,
    count(*) - count(distinct event_id)        as repeated,
    round(100.0 * (count(*) - count(distinct event_id)) / count(*), 1) as repeated_pct
from {{ source('lake', 'clicks') }}
group by dt
having count(*) - count(distinct event_id) > 0.02 * count(*)
order by dt
