export const environmentKinds = [
  { id: 'unclassified', label: '待分类' }, { id: 'development', label: '开发' },
  { id: 'testing', label: '测试' }, { id: 'staging', label: '预发布' }, { id: 'production', label: '正式' },
]
export const environmentKindLabel = kind => environmentKinds.find(item => item.id === kind)?.label || '待分类'
export const distributionAudiences = [{ id: 'unclassified', label: '待分类' }, { id: 'testing', label: '测试发行' }, { id: 'production', label: '正式发行' }]
export const audienceLabel = audience => distributionAudiences.find(item => item.id === audience)?.label || '待分类'
