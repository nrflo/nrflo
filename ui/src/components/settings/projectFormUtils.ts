export interface ProjectFormData {
  id: string
  name: string
  root_path: string
  default_branch: string
  use_git_worktrees: boolean
  push_after_merge: boolean
  safety_hook_enabled: boolean
  safety_hook_allow_git: boolean
  safety_hook_allowed_rm_paths: string
  safety_hook_dangerous_patterns: string
}

const DEFAULT_RM_PATHS = [
  'node_modules', 'target', 'build', 'dist', '.cache',
  '__pycache__', 'coverage', '.next', 'vendor', '/tmp', '/var/tmp',
].join('\n')

const DEFAULT_DANGEROUS_PATTERNS = [
  'DROP DATABASE', 'DROP TABLE', 'TRUNCATE TABLE',
  '> /dev/sda', 'mkfs', 'dd if=',
  ':(){:|:&};:', 'chmod -R 777 /',
  'sudo rm', '--hard', 'rm -rf /',
].join('\n')

export const emptyProjectForm: ProjectFormData = {
  id: '',
  name: '',
  root_path: '',
  default_branch: '',
  use_git_worktrees: false,
  push_after_merge: false,
  safety_hook_enabled: false,
  safety_hook_allow_git: true,
  safety_hook_allowed_rm_paths: DEFAULT_RM_PATHS,
  safety_hook_dangerous_patterns: DEFAULT_DANGEROUS_PATTERNS,
}

interface SafetyHookConfig {
  enabled: boolean
  allow_git: boolean
  rm_rf_allowed_paths: string[]
  dangerous_patterns: string[]
}

type SafetyHookFields = Pick<ProjectFormData, 'safety_hook_enabled' | 'safety_hook_allow_git' | 'safety_hook_allowed_rm_paths' | 'safety_hook_dangerous_patterns'>

export function parseSafetyHookConfig(json: string | null): SafetyHookFields {
  if (!json) {
    return {
      safety_hook_enabled: false,
      safety_hook_allow_git: true,
      safety_hook_allowed_rm_paths: '',
      safety_hook_dangerous_patterns: '',
    }
  }
  try {
    const config: SafetyHookConfig = JSON.parse(json)
    return {
      safety_hook_enabled: config.enabled ?? false,
      safety_hook_allow_git: config.allow_git ?? true,
      safety_hook_allowed_rm_paths: (config.rm_rf_allowed_paths || []).join('\n'),
      safety_hook_dangerous_patterns: (config.dangerous_patterns || []).join('\n'),
    }
  } catch {
    return {
      safety_hook_enabled: false,
      safety_hook_allow_git: true,
      safety_hook_allowed_rm_paths: '',
      safety_hook_dangerous_patterns: '',
    }
  }
}

export function buildSafetyHookJSON(formData: ProjectFormData): string {
  if (!formData.safety_hook_enabled) return ''
  const config: SafetyHookConfig = {
    enabled: true,
    allow_git: formData.safety_hook_allow_git,
    rm_rf_allowed_paths: formData.safety_hook_allowed_rm_paths
      .split(/\r?\n/)
      .map((s) => s.trim())
      .filter(Boolean),
    dangerous_patterns: formData.safety_hook_dangerous_patterns
      .split(/\r?\n/)
      .map((s) => s.trim())
      .filter(Boolean),
  }
  return JSON.stringify(config)
}
