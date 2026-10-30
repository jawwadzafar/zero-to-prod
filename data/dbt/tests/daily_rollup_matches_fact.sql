-- A singular test: the daily rollup must add up to exactly the clicks in the fact table.
-- If a join fans out or drops rows, the totals differ and this returns a row.

with rollup as (select coalesce(sum(clicks), 0) as clicks from {{ ref('link_clicks_daily') }}),
fact as (select count(*) as clicks from {{ ref('fct_clicks') }})

select rollup.clicks as rollup_clicks, fact.clicks as fact_clicks
from rollup, fact
where rollup.clicks <> fact.clicks
