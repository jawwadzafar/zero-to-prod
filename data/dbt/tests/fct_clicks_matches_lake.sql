-- Reconciliation (chapter 17.7): for every day this build was meant to load, the
-- fact table holds exactly the distinct events the lake holds. With
-- start_date/end_date (a run for one day, chapter 17.5) that's those days;
-- without them, every day in the lake. It catches a late partition that an
-- incremental build's three-day lookback never reached, and a join or filter
-- that silently drops rows.

{% set start = var('start_date', none) %}

with lake as (

    select cast(clicked_at as date) as day, count(distinct event_id) as events
    from {{ source('lake', 'clicks') }}
    {% if start is not none %}
    where cast(clicked_at as date) between date '{{ start }}' and date '{{ var("end_date", start) }}'
    {% endif %}
    group by 1

),

fact as (

    select click_date as day, count(*) as events
    from {{ ref('fct_clicks') }}
    group by 1

)

select lake.day, lake.events as in_lake, coalesce(fact.events, 0) as in_fct_clicks
from lake
left join fact using (day)
where lake.events <> coalesce(fact.events, 0)
order by lake.day
