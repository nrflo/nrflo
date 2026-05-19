import { useState, useEffect, useRef } from 'react'
import { useArtifactStorage, useCleanup, useObserver } from '@/hooks/useProjectSettings'
import {
  type FormState as ArtifactFormState,
  toForm as toArtifactForm,
  buildPayload as buildArtifactPayload,
} from './ProjectArtifactStorageEditor'
import type { CleanupFormState } from './ProjectCleanupEditor'
import type { ObserverFormState } from './ProjectObserverEditor'
import type { ArtifactStorageConfig, CleanupSettings, ObserverSettings } from '@/api/projectSettings'

const defaultArtifactForm: ArtifactFormState = {
  mode: 'internal', account_id: '', bucket: '', prefix: '', access_key_ref: '', secret_key_ref: '',
}
const defaultCleanupForm: CleanupFormState = { enabled: false, retentionLimit: 0 }
const defaultObserverForm: ObserverFormState = { systemContext: '', provider: '', model: '' }

export function useProjectSubforms(projectId: string) {
  const { data: artifactData } = useArtifactStorage(projectId)
  const { data: cleanupData } = useCleanup(projectId)
  const { data: observerData } = useObserver(projectId)

  const [artifactValue, setArtifactValue] = useState<ArtifactFormState>(defaultArtifactForm)
  const [cleanupValue, setCleanupValue] = useState<CleanupFormState>(defaultCleanupForm)
  const [observerValue, setObserverValue] = useState<ObserverFormState>(defaultObserverForm)

  const initialArtifactRef = useRef<ArtifactStorageConfig | undefined>(undefined)
  const initialCleanupRef = useRef<CleanupSettings | undefined>(undefined)
  const initialObserverRef = useRef<ObserverSettings | undefined>(undefined)

  useEffect(() => {
    if (artifactData && !initialArtifactRef.current) {
      initialArtifactRef.current = artifactData
      setArtifactValue(toArtifactForm(artifactData))
    }
  }, [artifactData])

  useEffect(() => {
    if (cleanupData && !initialCleanupRef.current) {
      initialCleanupRef.current = cleanupData
      setCleanupValue({ enabled: cleanupData.enabled, retentionLimit: cleanupData.retention_limit })
    }
  }, [cleanupData])

  useEffect(() => {
    if (observerData && !initialObserverRef.current) {
      initialObserverRef.current = observerData
      setObserverValue({ systemContext: observerData.system_context, provider: observerData.provider, model: observerData.model })
    }
  }, [observerData])

  function buildChangedSubforms(): { artifact?: ArtifactStorageConfig; cleanup?: CleanupSettings; observer?: Partial<ObserverSettings> } {
    const initArtifact = initialArtifactRef.current
    const artifactChanged = JSON.stringify(artifactValue) !== JSON.stringify(initArtifact ? toArtifactForm(initArtifact) : defaultArtifactForm)

    const initCleanup = initialCleanupRef.current
    const initCleanupForm: CleanupFormState = initCleanup
      ? { enabled: initCleanup.enabled, retentionLimit: initCleanup.retention_limit }
      : defaultCleanupForm
    const cleanupChanged = JSON.stringify(cleanupValue) !== JSON.stringify(initCleanupForm)

    const initObserver = initialObserverRef.current
    const initObserverForm: ObserverFormState = initObserver
      ? { systemContext: initObserver.system_context, provider: initObserver.provider, model: initObserver.model }
      : defaultObserverForm
    const observerChanged = JSON.stringify(observerValue) !== JSON.stringify(initObserverForm)

    return {
      artifact: artifactChanged && artifactValue.mode !== 's3'
        ? buildArtifactPayload(artifactValue, initArtifact)
        : undefined,
      cleanup: cleanupChanged
        ? { enabled: cleanupValue.enabled, retention_limit: cleanupValue.retentionLimit }
        : undefined,
      observer: observerChanged
        ? { system_context: observerValue.systemContext, provider: observerValue.provider, model: observerValue.model }
        : undefined,
    }
  }

  return {
    artifactValue, setArtifactValue,
    cleanupValue, setCleanupValue,
    observerValue, setObserverValue,
    buildChangedSubforms,
  }
}
