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
  system?: boolean
}

export interface UserListResponse {
  users: User[]
}

export interface CreateUserRequest {
  email: string
  display_name: string
  password: string
  role: 'admin' | 'viewer'
}

export interface UpdateUserRequest {
  display_name?: string
  role?: 'admin' | 'viewer'
  status?: 'active' | 'disabled'
}

export interface ResetPasswordRequest {
  new_password: string
}
