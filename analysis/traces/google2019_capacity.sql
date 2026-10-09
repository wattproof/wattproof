#standardSQL
# Cell and power-domain capacity (normalised units), from machine_events: each machine's largest
# reported capacity. Machines not in the power map count under 'none'.
WITH m AS (
  SELECT machine_id, MAX(capacity.cpus) AS cpu, MAX(capacity.memory) AS mem
  FROM `google.com:google-cluster-data.clusterdata_2019_a.machine_events`
  GROUP BY machine_id
)
SELECT IFNULL(p.pdu, 'none') AS pdu, COUNT(*) AS machines, SUM(m.cpu) AS cpu, SUM(m.mem) AS mem
FROM m LEFT JOIN `google.com:google-cluster-data.powerdata_2019.machine_to_pdu_mapping` AS p
  ON p.machine_id = m.machine_id AND p.cell = 'a'
GROUP BY ROLLUP(pdu)
ORDER BY pdu
