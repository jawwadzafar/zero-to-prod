-- A generic test: fails on every combination of columns that appears more than once.
-- Use it to check a model's grain, e.g. one row per (slug, click_date).

{% test unique_combination(model, columns) %}

select {{ columns | join(', ') }}, count(*) as n
from {{ model }}
group by {{ columns | join(', ') }}
having count(*) > 1

{% endtest %}
