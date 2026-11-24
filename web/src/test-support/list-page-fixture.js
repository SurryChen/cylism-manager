// Emulate the server page contract around the full fixtures used by existing
// interaction tests. API serialization and real paging are tested separately.
export function listPageFixture(rows = [], query = {}) {
  let filtered = rows
  if (query.source_cluster_id) filtered = filtered.filter(row => String(row.source_cluster_id) === query.source_cluster_id)
  if (query.target_cluster_id) filtered = filtered.filter(row => String(row.target_cluster_id) === query.target_cluster_id)
  if (query.cluster_id !== undefined && query.cluster_id !== '') filtered = filtered.filter(row => String(row.cluster_id || 0) === query.cluster_id)
  const offset = Number(query.offset) || 0, limit = Number(query.limit) || 10
  return { items: filtered.slice(offset, offset + limit), total: filtered.length, limit, offset }
}
