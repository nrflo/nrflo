export interface User {
  id: string
  email: string
  display_name: string
  role: 'admin' | 'viewer'
  status: 'active' | 'disabled'
  must_change_password: boolean
  created_at: string
  updated_at: string
  last_login_at?: string
}
