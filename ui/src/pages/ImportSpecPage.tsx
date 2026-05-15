import { useState, useCallback, useRef } from 'react'
import { Button } from '@/components/ui/Button'
import { Textarea } from '@/components/ui/Textarea'
import { Spinner } from '@/components/ui/Spinner'
import { GitHubSearchCombo } from '@/components/import/GitHubSearchCombo'
import { JiraSearchCombo } from '@/components/import/JiraSearchCombo'
import { ImportPreviewForm } from './ImportPreviewForm'
import {
  startImport,
  getImportPreview,
  type ImportPreviewResponse,
} from '@/api/specImport'
import type { WSEvent } from '@/hooks/useWebSocket'
import { useWebSocketEvent } from '@/hooks/useWebSocketSubscription'

// TODO(test-writer): vitest+RTL tests for ImportSpecPage — (1) markdown happy path: pick Markdown, paste body, click Normalize, simulate spec_import.ready WS event, assert getImportPreview called, preview fields populate, click Create, assert commitImport called, assert navigate to /tickets/T-1; (2) github 412: searchGitHubIssues mock throws NotConfiguredError{missing:["GITHUB_TOKEN"]}, type 3+ chars, advance 250ms, assert inline config row with /projects/{id}/edit#env-vars link; (3) spec_import.failed: simulate failed WS event, assert Try Again button returns to step 2 preserving source+body
type Source = 'github' | 'jira' | 'markdown'
type Status = 'idle' | 'normalizing' | 'ready' | 'failed' | 'committing'

function SourcePicker({
  source,
  onChange,
}: {
  source: Source
  onChange: (s: Source) => void
}) {
  const options: { value: Source; label: string; description: string }[] = [
    { value: 'github', label: 'GitHub Issue', description: 'Import from a GitHub issue URL or search' },
    { value: 'jira', label: 'Jira Issue', description: 'Import from a Jira issue key or URL' },
    { value: 'markdown', label: 'Markdown / Text', description: 'Paste raw markdown or plain text' },
  ]
  return (
    <div className="space-y-2">
      {options.map((opt) => (
        <label
          key={opt.value}
          className="flex items-start gap-3 rounded-md border border-border p-3 cursor-pointer hover:bg-muted/50 transition-colors"
        >
          <input
            type="radio"
            name="source"
            value={opt.value}
            checked={source === opt.value}
            onChange={() => onChange(opt.value)}
            className="mt-0.5"
          />
          <div>
            <div className="text-sm font-medium">{opt.label}</div>
            <div className="text-xs text-muted-foreground">{opt.description}</div>
          </div>
        </label>
      ))}
    </div>
  )
}

export function ImportSpecPage() {
  const [step, setStep] = useState<1 | 2 | 3>(1)
  const [source, setSource] = useState<Source>('github')
  const [instanceId, setInstanceId] = useState<string | null>(null)
  const [status, setStatus] = useState<Status>('idle')
  const [preview, setPreview] = useState<ImportPreviewResponse | null>(null)
  const [errorMsg, setErrorMsg] = useState<string | null>(null)
  const [markdownBody, setMarkdownBody] = useState('')
  const [githubUrl, setGithubUrl] = useState('')
  const [jiraValue, setJiraValue] = useState('')
  const instanceIdRef = useRef<string | null>(null)
  instanceIdRef.current = instanceId

  const handleWSEvent = useCallback(
    (event: WSEvent) => {
      const id = instanceIdRef.current
      if (!id) return
      if (event.data?.instance_id !== id) return

      if (event.type === 'spec_import.ready') {
        getImportPreview(id)
          .then((p) => {
            setPreview(p)
            setStatus('ready')
            setStep(3)
          })
          .catch((e) => {
            setErrorMsg(e instanceof Error ? e.message : 'Failed to load preview')
            setStatus('failed')
          })
      } else if (event.type === 'spec_import.failed') {
        const msg = typeof event.data?.error === 'string' ? event.data.error : 'Import failed'
        setErrorMsg(msg)
        setStatus('failed')
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    []
  )

  useWebSocketEvent(handleWSEvent)

  async function handleNormalize() {
    let body = ''
    if (source === 'github') body = githubUrl
    else if (source === 'jira') body = jiraValue
    else body = markdownBody

    if (!body.trim()) return

    setStatus('normalizing')
    setErrorMsg(null)
    try {
      const resp = await startImport({
        source: source === 'github' ? 'github_issue' : source === 'jira' ? 'jira' : 'markdown',
        body,
      })
      setInstanceId(resp.instance_id)
      // Fallback for the test/no-orchestrator path that completes synchronously.
      if (resp.status === 'ready') {
        try {
          const p = await getImportPreview(resp.instance_id)
          setPreview(p)
          setStatus('ready')
          setStep(3)
        } catch (e) {
          setErrorMsg(e instanceof Error ? e.message : 'Failed to load preview')
          setStatus('failed')
        }
        return
      }
      // Real path: spec-normalizer agent runs asynchronously; the
      // spec_import.ready / spec_import.failed WS event drives the next step.
    } catch (e) {
      setErrorMsg(e instanceof Error ? e.message : 'Failed to start import')
      setStatus('failed')
    }
  }

  function handleTryAgain() {
    setStatus('idle')
    setErrorMsg(null)
    setInstanceId(null)
    setPreview(null)
  }

  const isNormalizing = status === 'normalizing'
  const body = source === 'github' ? githubUrl : source === 'jira' ? jiraValue : markdownBody

  return (
    <div className="max-w-2xl mx-auto space-y-6">
      <div>
        <h1 className="text-2xl font-bold tracking-tight">Import Spec</h1>
        <p className="text-muted-foreground text-sm mt-1">
          Import a ticket from a GitHub issue, Jira issue, or pasted markdown.
        </p>
      </div>

      {/* Step breadcrumb */}
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        {['Source', 'Input', 'Preview'].map((label, i) => (
          <span key={label} className="flex items-center gap-2">
            {i > 0 && <span>›</span>}
            <span className={step === i + 1 ? 'text-foreground font-medium' : ''}>{label}</span>
          </span>
        ))}
      </div>

      {step === 1 && (
        <div className="space-y-4">
          <SourcePicker source={source} onChange={setSource} />
          <div className="flex justify-end">
            <Button onClick={() => setStep(2)}>Next</Button>
          </div>
        </div>
      )}

      {step === 2 && (
        <div className="space-y-4">
          <Button variant="ghost" size="sm" onClick={() => setStep(1)}>
            ← Back
          </Button>

          {source === 'github' && (
            <GitHubSearchCombo url={githubUrl} onUrlChange={setGithubUrl} />
          )}
          {source === 'jira' && (
            <JiraSearchCombo value={jiraValue} onChange={setJiraValue} />
          )}
          {source === 'markdown' && (
            <div>
              <label className="text-sm font-medium mb-1 block">Paste markdown or text</label>
              <Textarea
                value={markdownBody}
                onChange={(e) => setMarkdownBody(e.target.value)}
                rows={10}
                placeholder="Paste your spec content here…"
                className="resize-y"
              />
            </div>
          )}

          {status === 'failed' && errorMsg && (
            <div className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
              {errorMsg}
            </div>
          )}

          <div className="flex justify-end">
            <Button
              onClick={handleNormalize}
              disabled={isNormalizing || !body.trim()}
            >
              {isNormalizing ? (
                <>
                  <Spinner size="sm" className="mr-2" />
                  Normalizing…
                </>
              ) : (
                'Normalize'
              )}
            </Button>
          </div>

          {isNormalizing && (
            <p className="text-xs text-muted-foreground text-center">
              Processing — waiting for preview…
            </p>
          )}

          {status === 'failed' && (
            <div className="flex justify-center">
              <Button variant="outline" size="sm" onClick={handleTryAgain}>
                Try Again
              </Button>
            </div>
          )}
        </div>
      )}

      {step === 3 && preview && (
        <div className="space-y-4">
          <Button variant="ghost" size="sm" onClick={() => { setStep(2); setStatus('idle') }}>
            ← Back
          </Button>
          <h2 className="text-lg font-semibold">Review &amp; Edit</h2>
          <ImportPreviewForm
            instanceId={instanceId!}
            preview={preview}
            onCommitted={() => {}}
          />
        </div>
      )}
    </div>
  )
}
