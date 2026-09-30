import { api } from './index.js'

const base = '/cloud/connections'
const id = value => encodeURIComponent(value)
const path = (...parts) => parts.join('/')
const post = (url, body, options) => options === undefined ? api.post(url, body) : api.post(url, body, options)
const regionalPath = (url, region) => region ? `${url}?${new URLSearchParams({ region })}` : url

export function getCloudProviders(options) { return api.get('/cloud/providers', options) }
export function getCloudConnections(options) { return api.get(base, options) }
export function createCloudConnection(body, options) { return post(base, body, options) }
export function updateCloudConnection(connectionID, body, options) { return api.put(path(base, id(connectionID)), body, options) }
export function deleteCloudConnection(connectionID, options) { return api.delete(path(base, id(connectionID)), undefined, options) }
export function getCloudConnectionPermissions(connectionID, options) { return api.get(path(base, id(connectionID), 'permissions'), options) }
export function getCloudZones(connectionID, options) { return api.get(path(base, id(connectionID), 'dns/zones'), options) }
export function getCloudDNSRecords(connectionID, zone, options) { return api.get(path(base, id(connectionID), 'dns/records') + '?zone=' + id(zone), options) }
export function createCloudDNSRecord(connectionID, body, options) { return post(path(base, id(connectionID), 'dns/records'), body, options) }
export function updateCloudDNSRecord(connectionID, recordID, body, options) { return api.put(path(base, id(connectionID), 'dns/records', id(recordID)), body, options) }
export function deleteCloudDNSRecord(connectionID, recordID, body, options) { return api.delete(path(base, id(connectionID), 'dns/records', id(recordID)), body, options) }
export function getCloudContainers(connectionID, options) { return api.get(path(base, id(connectionID), 'object-storage/containers'), options) }
export function createCloudContainer(connectionID, body, options) { return post(path(base, id(connectionID), 'object-storage/containers'), body, options) }
export function updateCloudContainer(connectionID, bucket, body, options) { return api.put(path(base, id(connectionID), 'object-storage/containers', id(bucket)), body, options) }
export function deleteCloudContainer(connectionID, bucket, body, region, options) { return api.delete(regionalPath(path(base, id(connectionID), 'object-storage/containers', id(bucket)), region), body, options) }
export function getCloudObjects(connectionID, bucket, { prefix = '', token = '', region = '' } = {}, options) { const query = new URLSearchParams(); if (prefix) query.set('prefix', prefix); if (token) query.set('token', token); if (region) query.set('region', region); const suffix = query.size ? '?' + query : ''; return api.get(path(base, id(connectionID), 'object-storage/containers', id(bucket), 'objects') + suffix, options) }
export function uploadCloudObject(connectionID, bucket, form, region, options) { return api.upload(regionalPath(path(base, id(connectionID), 'object-storage/containers', id(bucket), 'objects'), region), form, options) }
export function downloadCloudObjectURL(connectionID, bucket, key, region) { const query = new URLSearchParams({ key }); if (region) query.set('region', region); return '/api' + path(base, id(connectionID), 'object-storage/containers', id(bucket), 'object') + '?' + query }
export function deleteCloudObject(connectionID, bucket, body, region, options) { return api.delete(regionalPath(path(base, id(connectionID), 'object-storage/containers', id(bucket), 'object'), region), body, options) }
