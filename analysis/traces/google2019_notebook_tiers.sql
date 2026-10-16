#standardSQL
# Wattproof: check 2 of docs/research/trace-analysis-plan.md. Google's own method for hourly usage
# by tier, copied from the "Cell level resource usage time series" query in
# https://github.com/google/cluster-data/blob/master/clusterdata_analysis_colab.ipynb
# with cell = 'a'. The notebook stores no outputs, so its shape is reproduced here and compared
# with the tier rows of google2019_cell.sql. Unchanged from the notebook except that the cell
# capacity is a subquery instead of a separate query. Usage is a share of the cell's capacity.
# Time: hour_index counts hours of raw trace time (it starts 600 s before 2019-05-01 00:00 PT).

WITH
cap AS (
  SELECT SUM(cpu_cap) AS cpu_capacity, SUM(memory_cap) AS memory_capacity
  FROM (
    SELECT machine_id, MAX(capacity.cpus) AS cpu_cap, MAX(capacity.memory) AS memory_cap
    FROM `google.com:google-cluster-data`.clusterdata_2019_a.machine_events
    GROUP BY 1
  )
),
inst AS (
  SELECT u.time AS time,
    CASE
      WHEN e.priority BETWEEN 0 AND 99 THEN '1_free'
      WHEN e.priority BETWEEN 100 AND 115 THEN '2_beb'
      WHEN e.priority BETWEEN 116 AND 119 THEN '3_mid'
      ELSE '4_prod'
    END AS tier,
    u.cpu_usage AS cpu_usage,
    u.memory_usage AS memory_usage
  FROM (
    SELECT start_time AS time, collection_id, instance_index, machine_id,
      average_usage.cpus AS cpu_usage,
      average_usage.memory AS memory_usage
    FROM `google.com:google-cluster-data`.clusterdata_2019_a.instance_usage
    WHERE (alloc_collection_id IS NULL OR alloc_collection_id = 0)
      AND (end_time - start_time) >= (5 * 60 * 1e6)
  ) AS u JOIN (
    SELECT collection_id, instance_index, machine_id, MAX(priority) AS priority
    FROM `google.com:google-cluster-data`.clusterdata_2019_a.instance_events
    WHERE (alloc_collection_id IS NULL OR alloc_collection_id = 0)
    GROUP BY 1, 2, 3
  ) AS e ON u.collection_id = e.collection_id
    AND u.instance_index = e.instance_index
    AND u.machine_id = e.machine_id
)
SELECT CAST(FLOOR(time / (1e6 * 60 * 60)) AS INT64) AS hour_index,
  tier,
  SUM(cpu_usage) / (12 * ANY_VALUE(cap.cpu_capacity)) AS avg_cpu_usage,
  SUM(memory_usage) / (12 * ANY_VALUE(cap.memory_capacity)) AS avg_memory_usage,
  ANY_VALUE(cap.cpu_capacity) AS cpu_capacity,
  ANY_VALUE(cap.memory_capacity) AS memory_capacity
FROM inst CROSS JOIN cap
GROUP BY 1, 2
ORDER BY 1, 2
