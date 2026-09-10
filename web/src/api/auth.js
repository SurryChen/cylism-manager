import { api } from './index.js'

function post(path, body, options) {
  return options === undefined ? api.post(path, body) : api.post(path, body, options)
}

export function login(username, password, options) {
  return post('/auth/login', { username, password }, options)
}

export function temporaryLogin(token, options) {
  return post('/auth/temporary-login', { token }, options)
}
