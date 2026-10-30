-- Completeness, of the lake (chapter 17.7): every day between the first and the
-- newest clicks partition has a folder. A missing day usually means a late or
-- lost partition, and every total that includes that day is quietly too small.
-- A warning, not an error: the other days' numbers are still right, and the
-- missing day can be loaded when it arrives (`snipdata run --day ...`).

{{ config(severity = 'warn') }}

with present as (

    select distinct cast(dt as date) as day from {{ source('lake', 'clicks') }}

),

expected as (

    select cast(unnest(range(min(day), max(day) + interval 1 day, interval 1 day)) as date) as day
    from present

)

select expected.day as missing_day
from expected
anti join present using (day)
order by missing_day
