import { useState, useRef, useCallback, useEffect } from 'react'
import { Upload, X, Loader2, AlertCircle } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { cn } from '@/lib/utils'
import { uploadArtifact, cancelUpload } from '@/api/artifacts'
import type { InputArtifactRef } from '@/types/artifact'

interface StagedFile {
  id: string
  name: string
  size: number
  uploadId?: string
  uploading: boolean
  error?: string
}

interface ArtifactUploaderProps {
  onChange: (staged: InputArtifactRef[], hasPending: boolean) => void
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

export function ArtifactUploader({ onChange }: ArtifactUploaderProps) {
  const [files, setFiles] = useState<StagedFile[]>([])
  const [isDragging, setIsDragging] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    const refs: InputArtifactRef[] = files
      .filter(f => f.uploadId)
      .map(f => ({ upload_id: f.uploadId!, name: f.name }))
    const hasPending = files.some(f => f.uploading)
    onChange(refs, hasPending)
  }, [files]) // eslint-disable-line react-hooks/exhaustive-deps

  const uploadFile = useCallback(async (file: File) => {
    const id = `${Date.now()}-${Math.random()}`
    setFiles(prev => [...prev, { id, name: file.name, size: file.size, uploading: true }])
    try {
      const response = await uploadArtifact(file)
      setFiles(prev => prev.map(f =>
        f.id === id ? { ...f, uploadId: response.upload_id, uploading: false } : f
      ))
    } catch {
      setFiles(prev => prev.map(f =>
        f.id === id ? { ...f, uploading: false, error: 'Upload failed' } : f
      ))
    }
  }, [])

  const handleFiles = useCallback((fileList: FileList) => {
    for (const file of Array.from(fileList)) {
      uploadFile(file)
    }
  }, [uploadFile])

  const removeFile = async (id: string) => {
    const file = files.find(f => f.id === id)
    if (file?.uploadId) {
      cancelUpload(file.uploadId).catch(() => {})
    }
    setFiles(prev => prev.filter(f => f.id !== id))
  }

  const onDragOver = (e: React.DragEvent) => { e.preventDefault(); setIsDragging(true) }
  const onDragLeave = () => setIsDragging(false)
  const onDrop = (e: React.DragEvent) => {
    e.preventDefault()
    setIsDragging(false)
    if (e.dataTransfer.files.length) handleFiles(e.dataTransfer.files)
  }

  return (
    <div className="space-y-2">
      <div
        onDragOver={onDragOver}
        onDragLeave={onDragLeave}
        onDrop={onDrop}
        className={cn(
          'border-2 border-dashed rounded-md px-4 py-3 text-center transition-colors',
          isDragging ? 'border-primary bg-primary/5' : 'border-border'
        )}
      >
        <Upload className="h-4 w-4 mx-auto mb-1 text-muted-foreground" />
        <p className="text-xs text-muted-foreground">Drop files here or</p>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="mt-1 text-xs"
          onClick={() => inputRef.current?.click()}
        >
          Browse
        </Button>
        <input
          ref={inputRef}
          type="file"
          multiple
          className="hidden"
          onChange={(e) => e.target.files && handleFiles(e.target.files)}
        />
      </div>
      {files.length > 0 && (
        <ul className="space-y-1">
          {files.map(f => (
            <li key={f.id} className="flex items-center gap-2 text-xs border border-border rounded px-2 py-1">
              {f.uploading ? (
                <Loader2 className="h-3 w-3 shrink-0 text-muted-foreground spin-sync" />
              ) : f.error ? (
                <AlertCircle className="h-3 w-3 shrink-0 text-destructive" />
              ) : (
                <span className="h-3 w-3 shrink-0" />
              )}
              <span className="flex-1 truncate">{f.name}</span>
              <span className="text-muted-foreground shrink-0">{formatBytes(f.size)}</span>
              {f.error && <span className="text-destructive shrink-0">{f.error}</span>}
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="h-5 w-5 p-0 shrink-0"
                onClick={() => removeFile(f.id)}
                disabled={f.uploading}
              >
                <X className="h-3 w-3" />
              </Button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
