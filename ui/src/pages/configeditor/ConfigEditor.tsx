import { useState, useEffect } from 'react'
import { useParams } from 'react-router-dom'
import Form from '@rjsf/core'
import type { RJSFSchema } from '@rjsf/utils'
import validator from '@rjsf/validator-ajv8'
import yaml from 'js-yaml'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/Card'
import { Button } from '@/components/ui/Button'
import { MarkdownEditor } from '@/components/ui/MarkdownEditor'
import { VersionHistory } from '@/components/configeditor/VersionHistory'
import { DiffPreview } from '@/components/review/DiffPreview'
import { useConfigFile, useConfigHistory, usePutConfigFile, useRollbackConfig } from '@/hooks/useConfigFiles'
import type { ConfigFile } from '@/types/config_file'

export function ConfigEditorPage() {
  const { file: fileParam = '' } = useParams<{ file: string }>()
  const filePath = decodeURIComponent(fileParam)

  const { data: file, isLoading } = useConfigFile(filePath)
  const { data: history = [] } = useConfigHistory(filePath)
  const putFile = usePutConfigFile()
  const rollback = useRollbackConfig()

  const [content, setContent] = useState('')
  const [formData, setFormData] = useState<Record<string, unknown>>({})
  const [diffBase, setDiffBase] = useState('')

  useEffect(() => {
    if (!file) return
    setContent(file.content)
    if (file.schema) {
      try {
        const parsed = yaml.load(file.content)
        setFormData((parsed as Record<string, unknown>) ?? {})
      } catch {
        setFormData({})
      }
    }
  }, [file])

  if (isLoading) return <div className="p-6 text-center text-muted-foreground">Loading…</div>

  const hasSchema = !!file?.schema
  const currentContent = content || file?.content || ''

  const handleSave = () => {
    putFile.mutate({ path: filePath, content: currentContent })
  }

  const handleRollback = (version: number) => {
    rollback.mutate({ path: filePath, version }, {
      onSuccess: (data: ConfigFile) => {
        setContent(data.content)
        setDiffBase(currentContent)
        if (data.schema ?? file?.schema) {
          try {
            const parsed = yaml.load(data.content)
            setFormData((parsed as Record<string, unknown>) ?? {})
          } catch {
            setFormData({})
          }
        }
      },
    })
  }

  const editorSection = hasSchema ? (
    <Form
      schema={file!.schema as RJSFSchema}
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      validator={validator as any}
      formData={formData}
      onChange={(e: { formData?: Record<string, unknown> }) => {
        if (e.formData !== undefined) {
          setFormData(e.formData)
          setContent(yaml.dump(e.formData))
        }
      }}
      onSubmit={() => handleSave()}
    >
      <Button type="submit" disabled={putFile.isPending}>
        Save
      </Button>
    </Form>
  ) : (
    <div className="space-y-2">
      <MarkdownEditor
        value={currentContent}
        onChange={setContent}
        minHeight="300px"
        maxHeight="600px"
      />
      <Button onClick={handleSave} disabled={putFile.isPending}>
        Save
      </Button>
    </div>
  )

  return (
    <div className="p-6 max-w-6xl mx-auto space-y-4">
      <h2 className="text-lg font-semibold truncate">{filePath}</h2>
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Editor</CardTitle>
          </CardHeader>
          <CardContent className="pt-0">{editorSection}</CardContent>
        </Card>
        <div className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="text-sm">Version History</CardTitle>
            </CardHeader>
            <CardContent className="pt-0">
              <VersionHistory
                versions={history}
                currentVersion={file?.version ?? 1}
                onRollback={handleRollback}
                isRollingBack={rollback.isPending}
              />
            </CardContent>
          </Card>
          {diffBase && (
            <Card>
              <CardHeader>
                <CardTitle className="text-sm">Diff (rollback preview)</CardTitle>
              </CardHeader>
              <CardContent className="pt-0">
                <DiffPreview before={diffBase} after={currentContent} />
              </CardContent>
            </Card>
          )}
        </div>
      </div>
    </div>
  )
}
