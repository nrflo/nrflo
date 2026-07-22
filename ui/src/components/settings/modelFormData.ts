import type { Model } from '@/api/models'

export interface ModelFormData {
  id: string
  provider: string
  display_name: string
  cli_model: string
  api_model: string
  cli_efforts: string[]
  api_efforts: string[]
  cli_context: string
  api_context: string
  fallback_models: string
  default_effort: string
}

export const emptyModelForm: ModelFormData = {
  id: '', provider: 'anthropic', display_name: '', cli_model: '', api_model: '',
  cli_efforts: [], api_efforts: [], cli_context: '200000', api_context: '200000',
  fallback_models: '', default_effort: '',
}

export function modelToFormData(model: Model): ModelFormData {
  return {
    id: model.id, provider: model.provider, display_name: model.display_name,
    cli_model: model.cli_model, api_model: model.api_model,
    cli_efforts: model.cli_efforts, api_efforts: model.api_efforts,
    cli_context: String(model.cli_context), api_context: String(model.api_context),
    fallback_models: model.fallback_models, default_effort: model.default_effort,
  }
}
