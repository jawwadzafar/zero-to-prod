-- Volume, a simple anomaly check (chapter 17.7): each day's clicks compared with
-- the median of the seven days before it. Under 40% or over 250% is suspicious:
-- a lost file, a replay that slipped through, a bot. Weekends are quieter, so
-- the bounds are wide; days with fewer than three earlier days aren't judged.

{{ config(severity = 'warn') }}

with daily as (

    select click_date, count(*) as clicks
    from {{ ref('fct_clicks') }}
    group by click_date

),

compared as (

    select
        click_date,
        clicks,
        median(clicks) over w as usual,
        count(*) over w       as history_days
    from daily
    window w as (order by click_date range between interval 7 day preceding and interval 1 day preceding)

)

select click_date, clicks, usual, round(clicks / usual, 2) as ratio
from compared
where history_days >= 3
  and (clicks < 0.4 * usual or clicks > 2.5 * usual)
order by click_date
